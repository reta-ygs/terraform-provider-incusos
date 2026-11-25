package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	osapi "github.com/lxc/incus-os/incus-osd/api"
)

type mockIncusOSServer struct {
	server        *httptest.Server
	serverCertPEM string
	clientCertPEM string
	clientKeyPEM  string

	mu                             sync.Mutex
	network                        osapi.SystemNetwork
	networkPending                 bool
	networkConfirmed               bool
	networkConfirmCount            int
	networkConfirmFailCount        int
	lastNetworkConfirmationTimeout string
	storage                        osapi.SystemStorage
	kernel                         osapi.SystemKernel
	logging                        osapi.SystemLogging
	power                          osapi.SystemPower
	provider                       osapi.SystemProvider
	security                       osapi.SystemSecurity
	update                         osapi.SystemUpdate
	fallback                       osapi.SystemFallbackListener
	applications                   map[string]osapi.Application
	services                       map[string]json.RawMessage
}

func newMockIncusOSServer(t *testing.T) *mockIncusOSServer {
	t.Helper()

	m := &mockIncusOSServer{
		network: osapi.SystemNetwork{
			Config: &osapi.SystemNetworkConfig{},
			State: osapi.SystemNetworkState{
				Interfaces: map[string]osapi.SystemNetworkInterfaceState{
					"enp1s0": {
						Addresses: []string{"192.0.2.10"},
						Hwaddr:    "00:11:22:33:44:55",
						LLDP: []osapi.SystemNetworkLLDPState{
							{
								Capabilities: []string{"bridge", "router"},
								ChassisID:    "aa:bb:cc:dd:ee:ff",
								Description:  "Core Switch OS 1.0",
								Name:         "sw-core-01",
								PortID:       "Ethernet1/1",
								Port:         "uplink-node-01",
								VLAN:         100,
							},
						},
						MTU:   1500,
						Roles: []string{"management"},
						Speed: "1000Mb/s",
						State: "routable",
						Type:  "interface",
					},
					"bond0": {
						Hwaddr: "00:11:22:33:44:66",
						Members: map[string]osapi.SystemNetworkInterfaceState{
							"enp2s0": {
								Hwaddr: "00:11:22:33:44:66",
								LLDP: []osapi.SystemNetworkLLDPState{
									{
										Capabilities: []string{"bridge"},
										ChassisID:    "aa:bb:cc:dd:ee:02",
										Description:  "Leaf Switch OS 1.0",
										Name:         "sw-leaf-01",
										PortID:       "Ethernet1/2",
										Port:         "bond0-member0",
										VLAN:         200,
									},
								},
								MTU:   9000,
								Speed: "10000Mb/s",
								State: "enslaved",
								Type:  "bond_member",
							},
						},
						MTU:   9000,
						Roles: []string{"instances"},
						Speed: "10000Mb/s",
						State: "routable",
						Type:  "bond",
					},
				},
			},
		},
		storage: osapi.SystemStorage{
			Config: osapi.SystemStorageConfig{
				ScrubSchedule: "0 0 * * 0",
				TrimSchedule:  "0 2 * * 0",
			},
			State: osapi.SystemStorageState{
				RootPartition: osapi.SystemStorageRootPartition{
					SizeInBytes:      107374182400,
					AvailableInBytes: 85899345920,
				},
				Drives: []osapi.SystemStorageDrive{
					{
						ID:              "/dev/disk/by-id/nvme-test-0",
						ModelFamily:     "NVMe",
						ModelName:       "Test NVMe 1TB",
						SerialNumber:    "SN0001",
						Bus:             "nvme",
						CapacityInBytes: 1000204886016,
						Boot:            true,
					},
					{
						ID:              "/dev/disk/by-id/nvme-test-1",
						ModelFamily:     "NVMe",
						ModelName:       "Test NVMe 2TB",
						SerialNumber:    "SN0002",
						Bus:             "nvme",
						CapacityInBytes: 2000398934016,
					},
				},
			},
		},
		kernel: osapi.SystemKernel{
			State: osapi.SystemKernelState{
				Clock: &osapi.SystemKernelStateClock{
					Source:           "tsc",
					AvailableSources: []string{"tsc", "hpet"},
				},
			},
		},
		power: osapi.SystemPower{
			State: osapi.SystemPowerState{
				CPU: osapi.SystemPowerStateCPU{
					ScalingDriver:                         "amd-pstate-epp",
					ScalingGovernor:                       "powersave",
					ScalingAvailableGovernors:             []string{"performance", "powersave"},
					EnergyPerformancePreference:           "balance_performance",
					EnergyPerformanceAvailablePreferences: []string{"performance", "balance_performance", "power"},
				},
			},
		},
		provider: osapi.SystemProvider{
			Config: osapi.SystemProviderConfig{Name: "images"},
			State:  osapi.SystemProviderState{Registered: true},
		},
		security: osapi.SystemSecurity{
			State: osapi.SystemSecurityState{
				SecureBootEnabled:               true,
				SystemStateIsTrusted:            true,
				SystemStateStatus:               "trusted",
				TPMStatus:                       osapi.TPMStatusOK,
				TPMPublicKey:                    "ssh-rsa AAAAB3...",
				EncryptionRecoveryKeysRetrieved: true,
				EncryptedVolumes: []osapi.SystemSecurityEncryptedVolume{
					{Volume: "root", State: "unlocked"},
				},
				SecureBootCertificates: []osapi.SystemSecuritySecureBootCertificate{
					{Type: "PK", Fingerprint: "aa:bb:cc", Subject: "CN=IncusOS", Issuer: "CN=IncusOS"},
				},
			},
		},
		update: osapi.SystemUpdate{
			Config: osapi.SystemUpdateConfig{
				Channel:        "stable",
				CheckFrequency: "6h",
			},
			State: osapi.SystemUpdateState{
				Status:      "idle",
				NeedsReboot: false,
			},
		},
		applications: map[string]osapi.Application{
			"incus": {
				State: osapi.ApplicationState{
					IsPrimary:         true,
					Initialized:       true,
					FriendlyVersion:   "7.6.0 [202611200000]",
					Version:           "202611200000",
					AvailableVersions: []string{"202611200000"},
				},
			},
		},
		services: map[string]json.RawMessage{},
	}

	// Pre-populate empty service states
	mustMarshal := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	m.services["ceph"] = mustMarshal(osapi.ServiceCeph{})
	m.services["iscsi"] = mustMarshal(osapi.ServiceISCSI{State: osapi.ServiceISCSIState{InitiatorName: "iqn.2026-01.org.incusos:node01"}})
	m.services["linstor"] = mustMarshal(osapi.ServiceLinstor{})
	m.services["lvm"] = mustMarshal(osapi.ServiceLVM{})
	m.services["multipath"] = mustMarshal(osapi.ServiceMultipath{})
	m.services["netbird"] = mustMarshal(osapi.ServiceNetbird{})
	m.services["nvme"] = mustMarshal(osapi.ServiceNVME{State: osapi.ServiceNVMEState{HostID: "host-uuid-1", HostNQN: "nqn.2014-08.org.nvmexpress:uuid:host-uuid-1"}})
	m.services["ovn"] = mustMarshal(osapi.ServiceOVN{})
	m.services["tailscale"] = mustMarshal(osapi.ServiceTailscale{State: osapi.ServiceTailscaleState{BackendState: osapi.ServiceTailscaleBackendStateRunning, TailnetName: "example.ts.net", Version: "1.102.5"}})
	m.services["usbip"] = mustMarshal(osapi.ServiceUSBIP{})

	serverCertPEM, _, serverTLSCert := generateSelfSignedCert(t, "incusos-mock", true)
	clientCertPEM, clientKeyPEM, _ := generateSelfSignedCert(t, "terraform-client", false)
	m.serverCertPEM = string(serverCertPEM)
	m.clientCertPEM = string(clientCertPEM)
	m.clientKeyPEM = string(clientKeyPEM)

	clientPool := x509.NewCertPool()
	clientPool.AppendCertsFromPEM(clientCertPEM)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(m.handle))
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientPool,
	}
	srv.StartTLS()
	m.server = srv
	t.Cleanup(srv.Close)

	return m
}

func (m *mockIncusOSServer) providerBlock() string {
	return m.providerBlockWithExtra("")
}

func (m *mockIncusOSServer) providerBlockWithExtra(extra string) string {
	return `
provider "incusos" {
  address     = "` + m.server.URL + `"
  path_prefix = "/os"
  client_cert = <<EOT
` + m.clientCertPEM + `EOT
  client_key  = <<EOT
` + m.clientKeyPEM + `EOT
  server_cert = <<EOT
` + m.serverCertPEM + `EOT
` + extra + `}
`
}

func writeSync(w http.ResponseWriter, metadata any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":        "sync",
		"status":      "Success",
		"status_code": 200,
		"metadata":    metadata,
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "error",
		"error":      msg,
		"error_code": status,
	})
}

func writeNotFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, msg)
}

func (m *mockIncusOSServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := strings.TrimPrefix(r.URL.Path, "/os")

	switch p {
	case "/1.0":
		writeSync(w, map[string]any{
			"environment": map[string]any{
				"hostname":             "incusos-node-1",
				"machine_id":           "1111222233334444",
				"os_name":              "IncusOS",
				"os_version":           "202611200000",
				"os_version_alternate": "202611100000",
				"os_version_next":      "202611200000",
				"system_is_ready":      true,
				"uptime":               12345,
			},
		})
	case "/1.0/system/network":
		if r.Method == http.MethodGet {
			writeSync(w, m.network)
			return
		}
		if m.networkPending {
			writeError(w, http.StatusBadRequest, "a pending network configuration must first be confirmed before a new configuration can be applied")
			return
		}
		var req osapi.SystemNetwork
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Config == nil {
			req.Config = &osapi.SystemNetworkConfig{}
		}

		// Emulate incus-osd api_system_network.go confirmation timeout semantics.
		confirmTimeout := req.Config.ConfirmationTimeout
		m.lastNetworkConfirmationTimeout = confirmTimeout
		if confirmTimeout != "" {
			d, err := time.ParseDuration(confirmTimeout)
			if err != nil || d <= 0 {
				writeError(w, http.StatusBadRequest, "invalid confirmation timeout provided")
				return
			}
			// IncusOS clears ConfirmationTimeout after parsing so GET never returns it.
			req.Config.ConfirmationTimeout = ""
		}

		if confirmTimeout == "" && m.network.Config != nil && m.network.Config.DefaultConfirmationTimeout != "" {
			confirmTimeout = m.network.Config.DefaultConfirmationTimeout
		}

		if confirmTimeout != "" {
			m.networkPending = true
		}

		m.network.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/network/:confirm":
		if m.networkConfirmFailCount > 0 {
			m.networkConfirmFailCount--
			writeError(w, http.StatusServiceUnavailable, "temporary network disruption")
			return
		}
		if !m.networkPending {
			writeError(w, http.StatusBadRequest, "no network configuration is pending a confirmation")
			return
		}
		m.networkPending = false
		m.networkConfirmed = true
		m.networkConfirmCount++
		writeSync(w, map[string]any{})
	case "/1.0/system/storage":
		if r.Method == http.MethodGet {
			writeSync(w, m.storage)
			return
		}
		var req osapi.SystemStorage
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.storage.Config.ScrubSchedule = req.Config.ScrubSchedule
		m.storage.Config.TrimSchedule = req.Config.TrimSchedule
		for i := range req.Config.Pools {
			req.Config.Pools[i].Managed = true
			req.Config.Pools[i].State = "ONLINE"
			req.Config.Pools[i].EncryptionKeyStatus = "available"
			req.Config.Pools[i].RawPoolSizeInBytes = 2000398934016
			req.Config.Pools[i].UsablePoolSizeInBytes = 1900000000000
			// Preserve existing volumes if updating pool
			for _, old := range m.storage.Config.Pools {
				if old.Name == req.Config.Pools[i].Name {
					req.Config.Pools[i].Volumes = old.Volumes
				}
			}
		}
		m.storage.Config.Pools = req.Config.Pools
		writeSync(w, map[string]any{})
	case "/1.0/system/storage/:delete-pool":
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		pools := make([]osapi.SystemStoragePool, 0, len(m.storage.Config.Pools))
		for _, pool := range m.storage.Config.Pools {
			if pool.Name != req.Name {
				pools = append(pools, pool)
			}
		}
		m.storage.Config.Pools = pools
		writeSync(w, map[string]any{})
	case "/1.0/system/storage/:create-volume":
		var req struct {
			Pool  string `json:"pool"`
			Name  string `json:"name"`
			Quota int    `json:"quota"`
			Use   string `json:"use"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for i := range m.storage.Config.Pools {
			if m.storage.Config.Pools[i].Name == req.Pool {
				m.storage.Config.Pools[i].Volumes = append(m.storage.Config.Pools[i].Volumes, osapi.SystemStoragePoolVolume{
					Name:         req.Name,
					QuotaInBytes: req.Quota,
					Use:          req.Use,
					UsageInBytes: 1048576,
				})
				writeSync(w, map[string]any{})
				return
			}
		}
		writeNotFound(w, "pool not found")
	case "/1.0/system/storage/:delete-volume":
		var req struct {
			Pool string `json:"pool"`
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for i := range m.storage.Config.Pools {
			if m.storage.Config.Pools[i].Name == req.Pool {
				vols := make([]osapi.SystemStoragePoolVolume, 0)
				for _, v := range m.storage.Config.Pools[i].Volumes {
					if v.Name != req.Name {
						vols = append(vols, v)
					}
				}
				m.storage.Config.Pools[i].Volumes = vols
				writeSync(w, map[string]any{})
				return
			}
		}
		writeNotFound(w, "pool not found")
	case "/1.0/system/kernel":
		if r.Method == http.MethodGet {
			writeSync(w, m.kernel)
			return
		}
		var req osapi.SystemKernel
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.kernel.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/logging":
		if r.Method == http.MethodGet {
			writeSync(w, m.logging)
			return
		}
		var req osapi.SystemLogging
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.logging.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/power":
		if r.Method == http.MethodGet {
			writeSync(w, m.power)
			return
		}
		var req osapi.SystemPower
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Config.Screen != nil {
			if req.Config.Screen.BlankTimeout < 0 || req.Config.Screen.BlankTimeout > 60 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type":       "error",
					"error":      "screen blank timeout must be between 0 and 60 minutes",
					"error_code": http.StatusBadRequest,
				})
				return
			}
			m.power.State.Screen.BlankTimeout = req.Config.Screen.BlankTimeout
		}
		m.power.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/provider":
		if r.Method == http.MethodGet {
			writeSync(w, m.provider)
			return
		}
		var req osapi.SystemProvider
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.provider.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/security":
		if r.Method == http.MethodGet {
			writeSync(w, m.security)
			return
		}
		var req osapi.SystemSecurity
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.security.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/update":
		if r.Method == http.MethodGet {
			writeSync(w, m.update)
			return
		}
		var req osapi.SystemUpdate
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.update.Config = req.Config
		writeSync(w, map[string]any{})
	case "/1.0/system/fallback-listener":
		if r.Method == http.MethodGet {
			writeSync(w, m.fallback)
			return
		}
		var req osapi.SystemFallbackListener
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.fallback.Config = req.Config
		m.fallback.State.Active = req.Config.Persistent
		writeSync(w, map[string]any{})
	case "/1.0/system/resources":
		writeSync(w, map[string]any{"cpu": map[string]any{"architecture": "x86_64"}})
	case "/1.0/applications":
		if r.Method == http.MethodGet {
			urls := make([]string, 0, len(m.applications))
			for name := range m.applications {
				urls = append(urls, "/os/1.0/applications/"+name)
			}
			slices.Sort(urls)
			writeSync(w, urls)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.applications[req.Name] = osapi.Application{
			State: osapi.ApplicationState{
				Initialized:       true,
				FriendlyVersion:   "1.0.0",
				Version:           "202611200000",
				AvailableVersions: []string{"202611200000"},
			},
		}
		writeSync(w, map[string]any{})
	case "/1.0/services":
		urls := make([]string, 0, len(m.services))
		for name := range m.services {
			urls = append(urls, "/os/1.0/services/"+name)
		}
		slices.Sort(urls)
		writeSync(w, urls)
	default:
		if strings.HasPrefix(p, "/1.0/applications/") {
			rest := strings.TrimPrefix(p, "/1.0/applications/")
			if name, ok := strings.CutSuffix(rest, "/:remove"); ok {
				delete(m.applications, name)
				writeSync(w, map[string]any{})
				return
			}
			app, exists := m.applications[rest]
			if !exists {
				writeNotFound(w, "application not found")
				return
			}
			if r.Method == http.MethodGet {
				writeSync(w, app)
				return
			}
			var req osapi.Application
			_ = json.NewDecoder(r.Body).Decode(&req)
			app.Config = req.Config
			m.applications[rest] = app
			writeSync(w, map[string]any{})
			return
		}

		if strings.HasPrefix(p, "/1.0/services/") {
			name := strings.TrimPrefix(p, "/1.0/services/")
			existing, ok := m.services[name]
			if !ok {
				writeNotFound(w, "service not found")
				return
			}
			if r.Method == http.MethodGet {
				writeSync(w, existing)
				return
			}
			// Merge incoming config while preserving existing state
			var oldMap, newMap map[string]json.RawMessage
			_ = json.Unmarshal(existing, &oldMap)
			_ = json.NewDecoder(r.Body).Decode(&newMap)
			if cfg, has := newMap["config"]; has {
				oldMap["config"] = cfg
			}
			merged, _ := json.Marshal(oldMap)
			m.services[name] = merged
			writeSync(w, map[string]any{})
			return
		}

		writeNotFound(w, "unknown endpoint: "+p)
	}
}

func generateSelfSignedCert(t *testing.T, cn string, isServer bool) (certPEM, keyPEM []byte, tlsCert tls.Certificate) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	if isServer {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		tmpl.DNSNames = []string{"localhost"}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("creating cert: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("loading keypair: %v", err)
	}

	return certPEM, keyPEM, tlsCert
}
