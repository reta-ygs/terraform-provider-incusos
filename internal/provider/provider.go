package provider

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/cliconfig"

	"github.com/reta-ygs/terraform-provider-incusos/internal/client"
)

var _ provider.Provider = (*IncusOSProvider)(nil)

const defaultNetworkConfirmationTimeout = 60 * time.Second

// IncusOSProvider defines the Terraform provider implementation for IncusOS.
type IncusOSProvider struct {
	version string
}

// IncusOSProviderModel describes the provider configuration data model.
type IncusOSProviderModel struct {
	Remote                     types.String `tfsdk:"remote"`
	ConfigDir                  types.String `tfsdk:"config_dir"`
	Address                    types.String `tfsdk:"address"`
	PathPrefix                 types.String `tfsdk:"path_prefix"`
	ClientCert                 types.String `tfsdk:"client_cert"`
	ClientKey                  types.String `tfsdk:"client_key"`
	ClientCertPath             types.String `tfsdk:"client_cert_path"`
	ClientKeyPath              types.String `tfsdk:"client_key_path"`
	ServerCert                 types.String `tfsdk:"server_cert"`
	ServerCertPath             types.String `tfsdk:"server_cert_path"`
	InsecureSkipVerify         types.Bool   `tfsdk:"insecure_skip_verify"`
	Timeout                    types.String `tfsdk:"timeout"`
	NetworkConfirmationTimeout types.String `tfsdk:"network_confirmation_timeout"`
}

// New returns a provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &IncusOSProvider{
			version: version,
		}
	}
}

func (p *IncusOSProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "incusos"
	resp.Version = p.version
}

func (p *IncusOSProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for managing IncusOS (`incus-osd`) host configuration, storage, services, and applications via the IncusOS HTTPS API.",
		Attributes: map[string]schema.Attribute{
			"remote": schema.StringAttribute{
				MarkdownDescription: "Incus CLI remote name from `config.yml` to load connection address and TLS certificates from. Can also be set via `INCUSOS_REMOTE` or `INCUS_REMOTE` (matching the Incus CLI). Explicit attributes and `INCUSOS_*` environment variables take precedence over remote settings.",
				Optional:            true,
			},
			"config_dir": schema.StringAttribute{
				MarkdownDescription: "Incus CLI configuration directory containing `config.yml`, `client.crt`, `client.key`, and `servercerts/`. Defaults to `INCUS_CONF` or the platform default (`~/.config/incus` on Linux).",
				Optional:            true,
			},
			"address": schema.StringAttribute{
				MarkdownDescription: "Base HTTPS URL of the IncusOS host (e.g., `https://192.0.2.10:8443`). Can also be set via `INCUSOS_ADDRESS`.",
				Optional:            true,
			},
			"path_prefix": schema.StringAttribute{
				MarkdownDescription: "URL path prefix before `/1.0/...`. Defaults to `/os` (the Incus HTTPS proxy path). Can also be set via `INCUSOS_PATH_PREFIX`.",
				Optional:            true,
			},
			"client_cert": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded TLS client certificate for mTLS authentication. Can also be set via `INCUSOS_CLIENT_CERT`.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_key": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded TLS client private key for mTLS authentication. Can also be set via `INCUSOS_CLIENT_KEY`.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_cert_path": schema.StringAttribute{
				MarkdownDescription: "Path to a PEM-encoded TLS client certificate file. Can also be set via `INCUSOS_CLIENT_CERT_PATH`.",
				Optional:            true,
			},
			"client_key_path": schema.StringAttribute{
				MarkdownDescription: "Path to a PEM-encoded TLS client private key file. Can also be set via `INCUSOS_CLIENT_KEY_PATH`.",
				Optional:            true,
			},
			"server_cert": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded server or CA TLS certificate to trust. Supports pinning self-signed Incus server certificates. Can also be set via `INCUSOS_SERVER_CERT`.",
				Optional:            true,
			},
			"server_cert_path": schema.StringAttribute{
				MarkdownDescription: "Path to a PEM-encoded server or CA TLS certificate file. Can also be set via `INCUSOS_SERVER_CERT_PATH`.",
				Optional:            true,
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Disable TLS server certificate verification (not recommended for production). Can also be set via `INCUSOS_INSECURE_SKIP_VERIFY`.",
				Optional:            true,
			},
			"timeout": schema.StringAttribute{
				MarkdownDescription: "HTTP request timeout as a Go duration string (e.g., `60s`, `5m`). Defaults to `60s`. Can also be set via `INCUSOS_TIMEOUT`.",
				Optional:            true,
			},
			"network_confirmation_timeout": schema.StringAttribute{
				MarkdownDescription: "Default rollback confirmation timeout duration for `incusos_system_network` changes (e.g., `60s`, `2m`). Set to `0` or `0s` to disable automatic rollback protection by default. Defaults to `60s`. Can also be set via `INCUSOS_NETWORK_CONFIRMATION_TIMEOUT`.",
				Optional:            true,
			},
		},
	}
}

func (p *IncusOSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data IncusOSProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := stringOrEnv(data.Address, "INCUSOS_ADDRESS")
	clientCert := stringOrEnv(data.ClientCert, "INCUSOS_CLIENT_CERT")
	clientKey := stringOrEnv(data.ClientKey, "INCUSOS_CLIENT_KEY")
	clientCertPath := stringOrEnv(data.ClientCertPath, "INCUSOS_CLIENT_CERT_PATH")
	clientKeyPath := stringOrEnv(data.ClientKeyPath, "INCUSOS_CLIENT_KEY_PATH")
	serverCert := stringOrEnv(data.ServerCert, "INCUSOS_SERVER_CERT")
	serverCertPath := stringOrEnv(data.ServerCertPath, "INCUSOS_SERVER_CERT_PATH")

	remoteName := stringOrEnv(data.Remote, "INCUSOS_REMOTE")
	if remoteName == "" {
		remoteName = os.Getenv("INCUS_REMOTE")
	}

	configDir := stringOrEnv(data.ConfigDir, "INCUS_CONF")

	// Resolve unset connection parameters from the Incus CLI config (~/.config/incus or INCUS_CONF)
	// when no explicit address is provided or when a remote/config_dir is explicitly selected.
	var unixSocket string
	var unixSocketReconnect func(context.Context) (string, error)
	hasExplicitInsecure := (!data.InsecureSkipVerify.IsNull() && !data.InsecureSkipVerify.IsUnknown()) || os.Getenv("INCUSOS_INSECURE_SKIP_VERIFY") != ""
	hasExplicitTLS := clientCert != "" || clientKey != "" || clientCertPath != "" || clientKeyPath != "" || serverCert != "" || serverCertPath != "" || hasExplicitInsecure
	hasExplicitConfigDir := !data.ConfigDir.IsNull() && !data.ConfigDir.IsUnknown()
	needRemoteLookup := address == "" || remoteName != "" || hasExplicitConfigDir
	if needRemoteLookup {
		allowKeepAlive := address == "" && !hasExplicitTLS
		canSpawnKeepAlive := allowKeepAlive && !hasExplicitConfigDir
		needClientCert := clientCert == "" && clientCertPath == ""
		needClientKey := clientKey == "" && clientKeyPath == ""

		resolved, err := resolveIncusRemote(ctx, configDir, remoteName, allowKeepAlive, canSpawnKeepAlive, needClientCert, needClientKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Resolve Incus Remote", err.Error())

			return
		}

		if resolved != nil {
			if allowKeepAlive && resolved.unixSocket != "" {
				unixSocket = resolved.unixSocket
				unixSocketReconnect = resolved.unixSocketReconnect
			}
			if address == "" {
				address = resolved.address
			}
			if needClientCert {
				clientCert = resolved.clientCert
				clientCertPath = resolved.clientCertPath
			}
			if needClientKey {
				clientKey = resolved.clientKey
			}
			if serverCert == "" && serverCertPath == "" {
				serverCertPath = resolved.serverCertPath
			}
		}
	}

	if unixSocket == "" && address == "" {
		resp.Diagnostics.AddError(
			"Missing IncusOS Address",
			"Configure `address` (or `INCUSOS_ADDRESS`), or specify an Incus remote via `remote` / `INCUSOS_REMOTE` / `INCUS_REMOTE`.",
		)

		return
	}

	pathPrefix := "/os"
	if !data.PathPrefix.IsNull() && !data.PathPrefix.IsUnknown() {
		pathPrefix = data.PathPrefix.ValueString()
	} else if envPrefix, ok := os.LookupEnv("INCUSOS_PATH_PREFIX"); ok {
		pathPrefix = envPrefix
	}

	insecure := false
	if !data.InsecureSkipVerify.IsNull() && !data.InsecureSkipVerify.IsUnknown() {
		insecure = data.InsecureSkipVerify.ValueBool()
	} else if envVal := os.Getenv("INCUSOS_INSECURE_SKIP_VERIFY"); envVal != "" {
		parsed, err := strconv.ParseBool(envVal)
		if err != nil {
			resp.Diagnostics.AddError("Invalid INCUSOS_INSECURE_SKIP_VERIFY", fmt.Sprintf("Failed to parse boolean: %s", err))

			return
		}

		insecure = parsed
	}

	timeout := 60 * time.Second
	timeoutStr := stringOrEnv(data.Timeout, "INCUSOS_TIMEOUT")
	if timeoutStr != "" {
		parsed, err := time.ParseDuration(timeoutStr)
		if err != nil {
			resp.Diagnostics.AddError("Invalid Timeout", fmt.Sprintf("Failed to parse timeout duration %q: %s", timeoutStr, err))

			return
		}

		timeout = parsed
	}

	networkConfirmationTimeout := defaultNetworkConfirmationTimeout
	if raw := stringOrEnv(data.NetworkConfirmationTimeout, "INCUSOS_NETWORK_CONFIRMATION_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid Network Confirmation Timeout",
				fmt.Sprintf("Failed to parse network_confirmation_timeout duration %q: %s", raw, err),
			)

			return
		}

		if parsed < 0 {
			resp.Diagnostics.AddError(
				"Invalid Network Confirmation Timeout",
				fmt.Sprintf("network_confirmation_timeout must be zero or positive, got %q", raw),
			)

			return
		}

		networkConfirmationTimeout = parsed
	}

	c, err := client.New(client.Config{
		Address:             address,
		UnixSocket:          unixSocket,
		UnixSocketReconnect: unixSocketReconnect,
		PathPrefix:          pathPrefix,
		ClientCert:          clientCert,
		ClientKey:           clientKey,
		ClientCertPath:      clientCertPath,
		ClientKeyPath:       clientKeyPath,
		ServerCert:          serverCert,
		ServerCertPath:      serverCertPath,
		InsecureSkipVerify:  insecure,
		Timeout:             timeout,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create IncusOS Client", err.Error())

		return
	}

	cfg := &providerConfig{
		client:                     c,
		networkConfirmationTimeout: networkConfirmationTimeout,
	}

	resp.DataSourceData = cfg
	resp.ResourceData = cfg
}

// promptPassphraseFunc prompts for a private key passphrase. Overridable in tests.
var promptPassphraseFunc = promptPassphraseTTY

type resolvedRemote struct {
	address             string
	unixSocket          string
	unixSocketReconnect func(context.Context) (string, error)
	clientCert          string
	clientCertPath      string
	clientKey           string
	serverCertPath      string
}

func startKeepAliveProxy(ctx context.Context, cfg *cliconfig.Config, remoteName string) (string, error) {
	type result struct {
		srv incus.InstanceServer
		err error
	}

	ch := make(chan result, 1)
	go func() {
		srv, err := cfg.GetInstanceServer(remoteName)
		ch <- result{srv: srv, err: err}
	}()

	select {
	case <-ctx.Done():
		go func() {
			if res := <-ch; res.srv != nil {
				res.srv.Disconnect()
			}
		}()

		return "", ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		defer res.srv.Disconnect()

		info, err := res.srv.GetConnectionInfo()
		if err != nil {
			return "", err
		}

		return info.SocketPath, nil
	}
}

func resolveIncusRemote(ctx context.Context, configDir, remoteName string, allowKeepAlive, canSpawnKeepAlive, needClientCert, needClientKey bool) (*resolvedRemote, error) {
	configPath := ""
	if configDir != "" {
		expanded := configDir
		if expanded == "~" || strings.HasPrefix(expanded, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("expanding home directory in config_dir %q: %w", configDir, err)
			}

			if expanded == "~" {
				expanded = home
			} else {
				expanded = filepath.Join(home, expanded[2:])
			}
		}

		configPath = filepath.Join(expanded, "config.yml")
	}

	cfg, err := cliconfig.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading Incus CLI config: %w", err)
	}
	cfg.PromptPassword = promptPassphraseFunc

	if remoteName == "" {
		remoteName = cfg.DefaultRemote
	}

	if remoteName == "" {
		return nil, nil
	}

	rem, ok := cfg.Remotes[remoteName]
	if !ok {
		return nil, fmt.Errorf("incus remote %q not found in %s", remoteName, cfg.ConfigPath("config.yml"))
	}

	if rem.Protocol != "" && rem.Protocol != "incus" {
		return nil, fmt.Errorf("incus remote %q uses unsupported protocol %q (only \"incus\" HTTPS remotes are supported for IncusOS)", remoteName, rem.Protocol)
	}

	addr := rem.LastWorkingAddr
	if addr == "" && len(rem.Addrs) > 0 {
		addr = rem.Addrs[0]
	}

	if !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "http://") {
		return nil, fmt.Errorf("incus remote %q has non-HTTPS address %q; IncusOS requires an HTTPS remote", remoteName, addr)
	}

	if allowKeepAlive && rem.KeepAlive > 0 {
		var reconnect func(context.Context) (string, error)
		if canSpawnKeepAlive {
			reconnect = func(reconnCtx context.Context) (string, error) {
				sock, reconnErr := startKeepAliveProxy(reconnCtx, cfg, remoteName)
				if reconnErr != nil {
					return "", reconnErr
				}
				if sock == "" {
					return "", fmt.Errorf("incus keepalive proxy for remote %q did not expose a unix socket", remoteName)
				}

				return sock, nil
			}
		}

		socketPath := filepath.Join(cfg.ConfigDir, "keepalive", fmt.Sprintf("%s.socket", remoteName))
		dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
		if conn, dialErr := dialer.DialContext(ctx, "unix", socketPath); dialErr == nil {
			_ = conn.Close()

			return &resolvedRemote{
				unixSocket:          socketPath,
				unixSocketReconnect: reconnect,
			}, nil
		}

		// Keepalive proxy is not running yet; start it when using the standard/INCUS_CONF config dir
		// (since Incus cliconfig spawns `incus remote proxy` inheriting INCUS_CONF).
		if canSpawnKeepAlive {
			sock, startErr := startKeepAliveProxy(ctx, cfg, remoteName)
			if startErr != nil {
				return nil, fmt.Errorf("connecting to Incus remote %q (keepalive): %w", remoteName, startErr)
			}

			if sock != "" {
				return &resolvedRemote{
					unixSocket:          sock,
					unixSocketReconnect: reconnect,
				}, nil
			}
		}
	}

	out := &resolvedRemote{
		address: addr,
	}

	if needClientKey {
		tlsCert, tlsKey, _, err := cfg.GetClientCertificate(remoteName)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate for remote %q: %w", remoteName, err)
		}

		if needClientCert {
			out.clientCert = tlsCert
		}
		out.clientKey = tlsKey
	} else if needClientCert {
		certPath := cfg.ConfigPath("client.crt")
		if cfg.HasRemoteClientCertificate(remoteName) {
			certPath = cfg.ConfigPath("clientcerts", fmt.Sprintf("%s.crt", remoteName))
		}
		if fileExists(certPath) {
			out.clientCertPath = certPath
		}
	}

	if p := cfg.ServerCertPath(remoteName); fileExists(p) {
		out.serverCertPath = p
	}

	return out, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}

	info, err := os.Stat(filepath.Clean(path)) //nolint:gosec // path is resolved from Incus cliconfig

	return err == nil && !info.IsDir()
}

func (p *IncusOSProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// System resources
		NewSystemNetworkResource,
		NewSystemStorageResource,
		NewSystemStoragePoolResource,
		NewSystemStorageVolumeResource,
		NewSystemKernelResource,
		NewSystemLoggingResource,
		NewSystemPowerResource,
		NewSystemProviderResource,
		NewSystemSecurityResource,
		NewSystemUpdateResource,
		NewSystemFallbackListenerResource,

		// Application resources
		NewApplicationResource,

		// Service resources
		NewServiceCephResource,
		NewServiceISCSIResource,
		NewServiceLinstorResource,
		NewServiceLVMResource,
		NewServiceMultipathResource,
		NewServiceNetbirdResource,
		NewServiceNVMEResource,
		NewServiceOVNResource,
		NewServiceTailscaleResource,
		NewServiceUSBIPResource,
	}
}

func (p *IncusOSProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewServerDataSource,
		NewSystemStorageDataSource,
		NewSystemSecurityDataSource,
		NewSystemNetworkDataSource,
		NewSystemResourcesDataSource,
		NewApplicationsDataSource,
		NewServicesDataSource,
	}
}

func stringOrEnv(val types.String, envKey string) string {
	if !val.IsNull() && !val.IsUnknown() {
		return val.ValueString()
	}

	return os.Getenv(envKey)
}
