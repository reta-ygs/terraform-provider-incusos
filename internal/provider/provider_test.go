package provider

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"golang.org/x/crypto/ssh"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"incusos": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func TestAccDataSources(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
data "incusos_server" "this" {}
data "incusos_system_storage" "this" {}
data "incusos_system_security" "this" {}
data "incusos_system_network" "this" {}
data "incusos_system_resources" "this" {}
data "incusos_applications" "this" {}
data "incusos_services" "this" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incusos_server.this", "hostname", "incusos-node-1"),
					resource.TestCheckResourceAttr("data.incusos_server.this", "os_name", "IncusOS"),
					resource.TestCheckResourceAttr("data.incusos_server.this", "system_is_ready", "true"),
					resource.TestCheckResourceAttr("data.incusos_system_storage.this", "drives.#", "2"),
					resource.TestCheckResourceAttr("data.incusos_system_storage.this", "drives.0.boot", "true"),
					resource.TestCheckResourceAttr("data.incusos_system_security.this", "secure_boot_enabled", "true"),
					resource.TestCheckResourceAttr("data.incusos_system_security.this", "tpm_status", "ok"),
					resource.TestCheckResourceAttr("data.incusos_system_network.this", "interfaces.enp1s0.state", "routable"),
					resource.TestCheckResourceAttrSet("data.incusos_system_resources.this", "json"),
					resource.TestCheckResourceAttr("data.incusos_applications.this", "applications.0", "incus"),
					resource.TestCheckResourceAttr("data.incusos_services.this", "services.#", "10"),
				),
			},
		},
	})
}

func TestAccSystemResources(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  confirmation_timeout = "60s"
  auto_confirm         = true

  dns = {
    hostname    = "node-01"
    domain      = "example.internal"
    nameservers = ["1.1.1.1", "8.8.8.8"]
  }

  time = {
    timezone    = "Asia/Tokyo"
    ntp_servers = ["ntp.nict.jp"]
  }

  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
      mtu       = 1500
    }
  ]
}

resource "incusos_system_kernel" "this" {
  blacklist_modules = ["nouveau"]
  network = {
    tcp_congestion_algorithm = "bbr"
    tcp_mtu_probing          = true
  }
  zfs = {
    arc_max_size = "8GiB"
  }
}

resource "incusos_system_logging" "this" {
  syslog = {
    address  = "192.0.2.50:514"
    protocol = "tcp"
  }
}

resource "incusos_system_power" "this" {
  cpu_scaling_governor              = "performance"
  cpu_energy_performance_preference = "balance_performance"
}

resource "incusos_system_provider" "this" {
  name = "images"
}

resource "incusos_system_security" "this" {
  custom_ca_certs = ["-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"]
}

resource "incusos_system_update" "this" {
  channel         = "stable"
  check_frequency = "6h"
  auto_reboot     = false
  maintenance_windows = [
    {
      start_day_of_week = "Sunday"
      start_hour        = 2
      start_minute      = 0
      end_day_of_week   = "Sunday"
      end_hour          = 4
      end_minute        = 0
    }
  ]
}

resource "incusos_system_fallback_listener" "this" {
  listen_address = ":8444"
  persistent     = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-01"),
					resource.TestCheckResourceAttr("incusos_system_network.this", "time.timezone", "Asia/Tokyo"),
					resource.TestCheckResourceAttr("incusos_system_kernel.this", "zfs.arc_max_size", "8GiB"),
					resource.TestCheckResourceAttr("incusos_system_kernel.this", "clock_source", "tsc"),
					resource.TestCheckResourceAttr("incusos_system_logging.this", "syslog.address", "192.0.2.50:514"),
					resource.TestCheckResourceAttr("incusos_system_power.this", "cpu_scaling_governor", "performance"),
					resource.TestCheckResourceAttr("incusos_system_power.this", "cpu_scaling_driver", "amd-pstate-epp"),
					resource.TestCheckResourceAttr("incusos_system_provider.this", "registered", "true"),
					resource.TestCheckResourceAttr("incusos_system_security.this", "tpm_status", "ok"),
					resource.TestCheckResourceAttr("incusos_system_update.this", "channel", "stable"),
					resource.TestCheckResourceAttr("incusos_system_fallback_listener.this", "active", "true"),
				),
			},
			{
				ResourceName:      "incusos_system_update.this",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})

	if !mock.networkConfirmed {
		t.Fatalf("expected network changes to be auto-confirmed via POST /1.0/system/network/:confirm")
	}
}

func TestAccStoragePoolAndVolume(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_storage" "schedules" {
  scrub_schedule = "0 3 * * 0"
  trim_schedule  = "0 4 * * 0"
}

resource "incusos_system_storage_pool" "data" {
  name    = "data"
  type    = "zfs-raid0"
  devices = ["/dev/disk/by-id/nvme-test-1"]
}

resource "incusos_system_storage_volume" "incus_vol" {
  pool           = incusos_system_storage_pool.data.name
  name           = "incus-default"
  use            = "incus"
  quota_in_bytes = 107374182400
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_system_storage.schedules", "scrub_schedule", "0 3 * * 0"),
					resource.TestCheckResourceAttr("incusos_system_storage_pool.data", "state", "ONLINE"),
					resource.TestCheckResourceAttr("incusos_system_storage_pool.data", "managed", "true"),
					resource.TestCheckResourceAttr("incusos_system_storage_volume.incus_vol", "id", "data/incus-default"),
					resource.TestCheckResourceAttr("incusos_system_storage_volume.incus_vol", "use", "incus"),
				),
			},
			{
				ResourceName:      "incusos_system_storage_pool.data",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "incusos_system_storage_volume.incus_vol",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccApplication(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_application" "gpu" {
  name                 = "gpu-support"
  pin_current_versions = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_application.gpu", "name", "gpu-support"),
					resource.TestCheckResourceAttr("incusos_application.gpu", "initialized", "true"),
					resource.TestCheckResourceAttr("incusos_application.gpu", "pin_current_versions", "false"),
				),
			},
			{
				Config: mock.providerBlock() + `
resource "incusos_application" "gpu" {
  name                 = "gpu-support"
  pin_current_versions = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_application.gpu", "pin_current_versions", "true"),
				),
			},
			{
				ResourceName:      "incusos_application.gpu",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccServices(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_service_tailscale" "this" {
  enabled       = true
  accept_routes = true
  auth_key      = "tskey-auth-example"
}

resource "incusos_service_ovn" "this" {
  enabled         = true
  database        = "ssl:192.0.2.20:6642"
  tunnel_address  = "192.0.2.10"
  tunnel_protocol = "geneve"
}

resource "incusos_service_ceph" "this" {
  enabled = true
  clusters = {
    ceph = {
      fsid     = "11111111-2222-3333-4444-555555555555"
      monitors = ["192.0.2.30:6789"]
      keyrings = {
        admin = "AQBtestkey=="
      }
    }
  }
}

resource "incusos_service_iscsi" "this" {
  enabled = true
  targets = [
    {
      target  = "iqn.2026-01.example.com:lun0"
      address = "192.0.2.40"
      port    = 3260
    }
  ]
}

resource "incusos_service_linstor" "this" {
  enabled        = true
  listen_address = "192.0.2.10"
}

resource "incusos_service_lvm" "this" {
  enabled   = true
  system_id = 1
}

resource "incusos_service_multipath" "this" {
  enabled              = true
  automatic_transports = ["iscsi"]
}

resource "incusos_service_netbird" "this" {
  enabled   = true
  setup_key = "nb-setup-key"
}

resource "incusos_service_nvme" "this" {
  enabled = true
  targets = [
    {
      transport = "tcp"
      address   = "192.0.2.41"
      port      = 4420
    }
  ]
}

resource "incusos_service_usbip" "this" {
  enabled = true
  targets = [
    {
      address = "192.0.2.42"
      bus_id  = "1-1"
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_service_tailscale.this", "backend_state", "Running"),
					resource.TestCheckResourceAttr("incusos_service_tailscale.this", "tailnet_name", "example.ts.net"),
					resource.TestCheckResourceAttr("incusos_service_ovn.this", "database", "ssl:192.0.2.20:6642"),
					resource.TestCheckResourceAttr("incusos_service_ceph.this", "clusters.ceph.fsid", "11111111-2222-3333-4444-555555555555"),
					resource.TestCheckResourceAttr("incusos_service_iscsi.this", "initiator_name", "iqn.2026-01.org.incusos:node01"),
					resource.TestCheckResourceAttr("incusos_service_linstor.this", "listen_address", "192.0.2.10"),
					resource.TestCheckResourceAttr("incusos_service_lvm.this", "system_id", "1"),
					resource.TestCheckResourceAttr("incusos_service_multipath.this", "automatic_transports.0", "iscsi"),
					resource.TestCheckResourceAttr("incusos_service_netbird.this", "setup_key", "nb-setup-key"),
					resource.TestCheckResourceAttr("incusos_service_nvme.this", "host_id", "host-uuid-1"),
					resource.TestCheckResourceAttr("incusos_service_usbip.this", "targets.0.bus_id", "1-1"),
				),
			},
		},
	})
}

func TestAccProviderIncusRemoteEnvVar(t *testing.T) {
	mock := newMockIncusOSServer(t)

	confDir := t.TempDir()
	serverCertsDir := filepath.Join(confDir, "servercerts")
	if err := os.MkdirAll(serverCertsDir, 0o700); err != nil {
		t.Fatalf("creating servercerts dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(confDir, "client.crt"), []byte(mock.clientCertPEM), 0o600); err != nil {
		t.Fatalf("writing client.crt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "client.key"), []byte(mock.clientKeyPEM), 0o600); err != nil {
		t.Fatalf("writing client.key: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serverCertsDir, "node-01.crt"), []byte(mock.serverCertPEM), 0o600); err != nil {
		t.Fatalf("writing server cert: %v", err)
	}

	configYAML := "default-remote: local\nremotes:\n  node-01:\n    addr: " + mock.server.URL + "\n    protocol: incus\n    public: false\n"
	if err := os.WriteFile(filepath.Join(confDir, "config.yml"), []byte(configYAML), 0o600); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	// Inject INCUS_CONF and INCUS_REMOTE just like the Incus CLI.
	t.Setenv("INCUS_CONF", confDir)
	t.Setenv("INCUS_REMOTE", "node-01")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "incusos" {}

data "incusos_server" "this" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incusos_server.this", "hostname", "incusos-node-1"),
					resource.TestCheckResourceAttr("data.incusos_server.this", "os_name", "IncusOS"),
				),
			},
		},
	})
}

func TestAccProviderIncusRemoteKeepAliveSocket(t *testing.T) {
	confDir := t.TempDir()
	keepaliveDir := filepath.Join(confDir, "keepalive")
	if err := os.MkdirAll(keepaliveDir, 0o700); err != nil {
		t.Fatalf("creating keepalive dir: %v", err)
	}

	sockPath := filepath.Join(keepaliveDir, "node-01.socket")
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "unix", sockPath)
	if err != nil {
		t.Fatalf("listening on keepalive socket: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/os/1.0" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type":        "sync",
					"status":      "Success",
					"status_code": 200,
					"metadata": map[string]any{
						"environment": map[string]any{
							"hostname":        "keepalive-node-1",
							"os_name":         "IncusOS",
							"system_is_ready": true,
						},
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	// Write an intentionally encrypted/unparseable client.key to verify the active
	// keepalive socket is used directly without reading or prompting for client.key.
	if err := os.WriteFile(filepath.Join(confDir, "client.key"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ninvalid\n-----END OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("writing client.key: %v", err)
	}

	configYAML := "default-remote: node-01\nremotes:\n  node-01:\n    addr: https://192.0.2.10:8443\n    protocol: incus\n    keepalive: 300\n    public: false\n"
	if err := os.WriteFile(filepath.Join(confDir, "config.yml"), []byte(configYAML), 0o600); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	origPrompt := promptPassphraseFunc
	t.Cleanup(func() { promptPassphraseFunc = origPrompt })
	promptPassphraseFunc = func(filename string) (string, error) {
		t.Fatalf("unexpected passphrase prompt for %s when keepalive socket is active", filename)
		return "", nil
	}

	t.Setenv("INCUS_CONF", confDir)
	t.Setenv("INCUS_REMOTE", "node-01")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "incusos" {}

data "incusos_server" "this" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incusos_server.this", "hostname", "keepalive-node-1"),
					resource.TestCheckResourceAttr("data.incusos_server.this", "os_name", "IncusOS"),
				),
			},
		},
	})
}

func TestAccProviderIncusRemoteEncryptedOpenSSHKey(t *testing.T) {
	mock := newMockIncusOSServer(t)

	block, _ := pem.Decode([]byte(mock.clientKeyPEM))
	if block == nil {
		t.Fatal("failed to decode mock client key PEM")
	}
	ecKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parsing EC private key: %v", err)
	}

	const passphrase = "test-secret-passphrase"
	encBlock, err := ssh.MarshalPrivateKeyWithPassphrase(ecKey, "", []byte(passphrase))
	if err != nil {
		t.Fatalf("marshaling encrypted OpenSSH key: %v", err)
	}
	encKeyPEM := pem.EncodeToMemory(encBlock)

	confDir := t.TempDir()
	serverCertsDir := filepath.Join(confDir, "servercerts")
	if err := os.MkdirAll(serverCertsDir, 0o700); err != nil {
		t.Fatalf("creating servercerts dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(confDir, "client.crt"), []byte(mock.clientCertPEM), 0o600); err != nil {
		t.Fatalf("writing client.crt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "client.key"), encKeyPEM, 0o600); err != nil {
		t.Fatalf("writing encrypted client.key: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serverCertsDir, "node-01.crt"), []byte(mock.serverCertPEM), 0o600); err != nil {
		t.Fatalf("writing server cert: %v", err)
	}

	configYAML := "default-remote: node-01\nremotes:\n  node-01:\n    addr: " + mock.server.URL + "\n    protocol: incus\n    public: false\n"
	if err := os.WriteFile(filepath.Join(confDir, "config.yml"), []byte(configYAML), 0o600); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	promptCount := 0
	origPrompt := promptPassphraseFunc
	t.Cleanup(func() { promptPassphraseFunc = origPrompt })
	promptPassphraseFunc = func(_ string) (string, error) {
		promptCount++
		return passphrase, nil
	}

	t.Setenv("INCUS_CONF", confDir)
	t.Setenv("INCUS_REMOTE", "node-01")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "incusos" {}

data "incusos_server" "this" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.incusos_server.this", "hostname", "incusos-node-1"),
					resource.TestCheckResourceAttr("data.incusos_server.this", "os_name", "IncusOS"),
				),
			},
		},
	})

	if promptCount == 0 {
		t.Fatal("expected passphrase prompt to be called for encrypted OpenSSH client.key")
	}
}

func TestAccSystemNetworkDefaultProviderConfirmationTimeout(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  dns = {
    hostname = "node-01"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-01"),
					resource.TestCheckResourceAttr("incusos_system_network.this", "auto_confirm", "true"),
					resource.TestCheckNoResourceAttr("incusos_system_network.this", "confirmation_timeout"),
				),
			},
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  confirmation_timeout = "45s"

  dns = {
    hostname = "node-02"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-02"),
					resource.TestCheckResourceAttr("incusos_system_network.this", "confirmation_timeout", "45s"),
				),
			},
		},
	})

	if mock.lastNetworkConfirmationTimeout != "45s" {
		t.Fatalf("expected last confirmation_timeout to be 45s, got %q", mock.lastNetworkConfirmationTimeout)
	}
	if mock.networkConfirmCount != 2 {
		t.Fatalf("expected 2 network confirmations, got %d", mock.networkConfirmCount)
	}
	if mock.networkPending {
		t.Fatalf("expected network configuration not to remain pending")
	}
}

func TestAccSystemNetworkConfirmationRetry(t *testing.T) {
	mock := newMockIncusOSServer(t)
	mock.networkConfirmFailCount = 2

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  dns = {
    hostname = "node-retry"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-retry"),
			},
		},
	})

	if mock.networkConfirmCount != 1 {
		t.Fatalf("expected 1 successful confirmation after retries, got %d", mock.networkConfirmCount)
	}
	if mock.networkPending {
		t.Fatalf("expected network configuration not to remain pending")
	}
}

func TestAccSystemNetworkDisableConfirmationAndDefaultConfirmationTimeout(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// First apply sets default_confirmation_timeout while provider confirmation is disabled (0).
				// IncusOS does NOT enter pending state on this first PUT because the prior running config had none.
				Config: mock.providerBlockWithExtra("  network_confirmation_timeout = \"0\"\n") + `
resource "incusos_system_network" "this" {
  default_confirmation_timeout = "30s"

  dns = {
    hostname = "node-01"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.TestCheckResourceAttr("incusos_system_network.this", "default_confirmation_timeout", "30s"),
			},
			{
				// Second apply: prior running config now has default_confirmation_timeout = "30s",
				// so IncusOS enters pending state even though confirmation_timeout is omitted,
				// and the provider must still confirm it.
				Config: mock.providerBlockWithExtra("  network_confirmation_timeout = \"0\"\n") + `
resource "incusos_system_network" "this" {
  default_confirmation_timeout = "30s"

  dns = {
    hostname = "node-02"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-02"),
			},
		},
	})

	if mock.networkConfirmCount != 1 {
		t.Fatalf("expected exactly 1 confirmation (on second apply only), got %d", mock.networkConfirmCount)
	}
	if mock.networkPending {
		t.Fatalf("expected network configuration not to remain pending")
	}
}

func TestAccSystemNetworkAutoConfirmFalse(t *testing.T) {
	mock := newMockIncusOSServer(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  auto_confirm = false

  dns = {
    hostname = "node-manual"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.TestCheckResourceAttr("incusos_system_network.this", "auto_confirm", "false"),
			},
		},
	})

	if mock.networkConfirmCount != 0 {
		t.Fatalf("expected 0 confirmations when auto_confirm=false, got %d", mock.networkConfirmCount)
	}
	if !mock.networkPending {
		t.Fatalf("expected network configuration to remain pending when auto_confirm=false")
	}
}

func TestAccSystemNetworkConfirmationTimeoutEnvVar(t *testing.T) {
	mock := newMockIncusOSServer(t)
	t.Setenv("INCUSOS_NETWORK_CONFIRMATION_TIMEOUT", "90s")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: mock.providerBlock() + `
resource "incusos_system_network" "this" {
  dns = {
    hostname = "node-env"
  }
  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management"]
    }
  ]
}
`,
				Check: resource.TestCheckResourceAttr("incusos_system_network.this", "dns.hostname", "node-env"),
			},
		},
	})

	if mock.lastNetworkConfirmationTimeout != "1m30s" {
		t.Fatalf("expected confirmation_timeout 1m30s from env var, got %q", mock.lastNetworkConfirmationTimeout)
	}
	if mock.networkConfirmCount != 1 {
		t.Fatalf("expected 1 confirmation, got %d", mock.networkConfirmCount)
	}
}
