package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	osapi "github.com/lxc/incus-os/incus-osd/api"
)

// Config configures a Client connection to an IncusOS host over HTTPS.
type Config struct {
	// Address is the base HTTPS URL of the IncusOS host (e.g., https://192.0.2.10:8443).
	Address string
	// UnixSocket is an optional path to a local Unix domain socket (e.g., an Incus keepalive proxy socket).
	// When set, requests are sent over this Unix socket instead of connecting directly to Address.
	UnixSocket string
	// PathPrefix is prepended before /1.0/... (defaults to "/os" for the Incus HTTPS proxy).
	PathPrefix string
	// ClientCert is the PEM-encoded client TLS certificate.
	ClientCert string
	// ClientKey is the PEM-encoded client TLS private key.
	ClientKey string
	// ClientCertPath is an optional filesystem path to the PEM-encoded client TLS certificate.
	ClientCertPath string
	// ClientKeyPath is an optional filesystem path to the PEM-encoded client TLS private key.
	ClientKeyPath string
	// ServerCert is an optional PEM-encoded server or CA certificate to trust.
	ServerCert string
	// ServerCertPath is an optional filesystem path to the PEM-encoded server or CA certificate.
	ServerCertPath string
	// InsecureSkipVerify disables TLS server certificate verification when true.
	InsecureSkipVerify bool
	// Timeout is the HTTP client timeout (defaults to 60s).
	Timeout time.Duration
}

// Client is an HTTP/HTTPS client for the IncusOS (incus-osd) REST API.
type Client struct {
	baseURL    string
	pathPrefix string
	httpClient *http.Client
	storageMu  sync.Mutex
}

// APIError represents an error returned by the IncusOS REST API.
type APIError struct {
	StatusCode int
	ErrorCode  int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("incusos API error (HTTP %d, code %d): %s", e.StatusCode, e.ErrorCode, e.Message)
	}

	return fmt.Sprintf("incusos API error (HTTP %d, code %d)", e.StatusCode, e.ErrorCode)
}

// IsNotFound reports whether err represents an HTTP 404 Not Found error.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound || apiErr.ErrorCode == http.StatusNotFound
	}

	return false
}

// apiEnvelope matches the standard Incus/IncusOS JSON response envelope.
type apiEnvelope struct {
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Error      string          `json:"error"`
	ErrorCode  int             `json:"error_code"`
	Metadata   json.RawMessage `json:"metadata"`
}

// ServerInfo represents the response metadata from GET /1.0.
type ServerInfo struct {
	Environment ServerEnvironment `json:"environment"`
}

// ServerEnvironment represents the environment fields returned by GET /1.0.
type ServerEnvironment struct {
	Hostname           string `json:"hostname"`
	MachineID          string `json:"machine_id"`
	OSName             string `json:"os_name"`
	OSVersion          string `json:"os_version"`
	OSVersionAlternate string `json:"os_version_alternate"`
	OSVersionNext      string `json:"os_version_next"`
	SystemIsReady      bool   `json:"system_is_ready"`
	Uptime             int64  `json:"uptime"`
}

// New creates a new IncusOS API client from Config.
func New(cfg Config) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	prefix := cfg.PathPrefix
	if prefix != "" {
		prefix = "/" + strings.Trim(prefix, "/")
	}

	if socketPath := strings.TrimSpace(cfg.UnixSocket); socketPath != "" {
		dialer := &net.Dialer{Timeout: timeout}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		}

		return &Client{
			baseURL:    "http://unix.socket",
			pathPrefix: prefix,
			httpClient: &http.Client{
				Transport: transport,
				Timeout:   timeout,
			},
		}, nil
	}

	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("address must be set")
	}

	parsed, err := url.Parse(strings.TrimRight(cfg.Address, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", cfg.Address, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("address scheme must be https (or http for local testing), got %q", parsed.Scheme)
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
		Proxy:           http.ProxyFromEnvironment,
	}

	return &Client{
		baseURL:    parsed.String(),
		pathPrefix: prefix,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}, nil
}

func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding home directory in %q: %w", p, err)
		}

		if p == "~" {
			return home, nil
		}

		return filepath.Join(home, p[2:]), nil
	}

	return p, nil
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // user-configurable option
	}

	certPEM := cfg.ClientCert
	if certPEM == "" && cfg.ClientCertPath != "" {
		expanded, err := expandHome(cfg.ClientCertPath)
		if err != nil {
			return nil, err
		}

		b, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("reading client_cert_path %q: %w", expanded, err)
		}

		certPEM = string(b)
	}

	keyPEM := cfg.ClientKey
	if keyPEM == "" && cfg.ClientKeyPath != "" {
		expanded, err := expandHome(cfg.ClientKeyPath)
		if err != nil {
			return nil, err
		}

		b, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("reading client_key_path %q: %w", expanded, err)
		}

		keyPEM = string(b)
	}

	if certPEM != "" || keyPEM != "" {
		if certPEM == "" || keyPEM == "" {
			return nil, errors.New("both client certificate and client key must be provided together")
		}

		cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("loading client TLS keypair: %w", err)
		}

		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	serverCertPEM := cfg.ServerCert
	if serverCertPEM == "" && cfg.ServerCertPath != "" {
		expanded, err := expandHome(cfg.ServerCertPath)
		if err != nil {
			return nil, err
		}

		b, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("reading server_cert_path %q: %w", expanded, err)
		}

		serverCertPEM = string(b)
	}

	if serverCertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(serverCertPEM)) {
			return nil, errors.New("failed to parse server_cert PEM")
		}

		tlsCfg.RootCAs = pool

		// Also support pinning a self-signed Incus server leaf certificate directly (matching Incus CLI behavior).
		if block, _ := pem.Decode([]byte(serverCertPEM)); block != nil {
			if pinnedCert, err := x509.ParseCertificate(block.Bytes); err == nil && !pinnedCert.IsCA {
				expectedFP := sha256.Sum256(pinnedCert.Raw)
				tlsCfg.InsecureSkipVerify = true // verified manually via VerifyConnection below
				tlsCfg.VerifyConnection = func(cs tls.ConnectionState) error {
					if len(cs.PeerCertificates) == 0 {
						return errors.New("server presented no TLS certificate")
					}

					gotFP := sha256.Sum256(cs.PeerCertificates[0].Raw)
					if !bytes.Equal(gotFP[:], expectedFP[:]) {
						return fmt.Errorf("server certificate fingerprint mismatch: got %x, expected %x", gotFP, expectedFP)
					}

					return nil
				}
			}
		}
	}

	return tlsCfg, nil
}

func (c *Client) endpointURL(path string) string {
	cleanPath := "/" + strings.TrimLeft(path, "/")

	return c.baseURL + c.pathPrefix + cleanPath
}

func (c *Client) do(ctx context.Context, method, path string, reqBody any, dst any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshaling request body for %s %s: %w", method, path, err)
		}

		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpointURL(path), bodyReader)
	if err != nil {
		return fmt.Errorf("creating HTTP request %s %s: %w", method, path, err)
	}

	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body for %s %s: %w", method, path, err)
	}

	var env apiEnvelope
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &env); err != nil {
			if resp.StatusCode >= 400 {
				return &APIError{
					StatusCode: resp.StatusCode,
					Message:    strings.TrimSpace(string(rawBody)),
				}
			}

			return fmt.Errorf("decoding response envelope for %s %s: %w", method, path, err)
		}
	}

	if env.Type == "error" || resp.StatusCode >= 400 {
		msg := env.Error
		if msg == "" {
			msg = env.Status
		}

		return &APIError{
			StatusCode: resp.StatusCode,
			ErrorCode:  env.ErrorCode,
			Message:    msg,
		}
	}

	if dst != nil && len(env.Metadata) > 0 && string(env.Metadata) != "null" {
		if err := json.Unmarshal(env.Metadata, dst); err != nil {
			return fmt.Errorf("decoding metadata for %s %s: %w", method, path, err)
		}
	}

	return nil
}

// --- Server ---

// GetServerInfo calls GET /1.0.
func (c *Client) GetServerInfo(ctx context.Context) (*ServerInfo, error) {
	var out ServerInfo
	if err := c.do(ctx, http.MethodGet, "/1.0", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// --- System endpoints ---

// GetSystemNetwork calls GET /1.0/system/network.
func (c *Client) GetSystemNetwork(ctx context.Context) (*osapi.SystemNetwork, error) {
	var out osapi.SystemNetwork
	if err := c.do(ctx, http.MethodGet, "/1.0/system/network", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemNetwork calls PUT /1.0/system/network.
func (c *Client) UpdateSystemNetwork(ctx context.Context, cfg osapi.SystemNetwork) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/network", cfg, nil)
}

// ConfirmSystemNetwork calls POST /1.0/system/network/:confirm.
func (c *Client) ConfirmSystemNetwork(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/1.0/system/network/:confirm", nil, nil)
}

// GetSystemStorage calls GET /1.0/system/storage.
func (c *Client) GetSystemStorage(ctx context.Context) (*osapi.SystemStorage, error) {
	var out osapi.SystemStorage
	if err := c.do(ctx, http.MethodGet, "/1.0/system/storage", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemStorage calls PUT /1.0/system/storage.
func (c *Client) UpdateSystemStorage(ctx context.Context, cfg osapi.SystemStorage) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/storage", cfg, nil)
}

// MutateSystemStorage serializes read-modify-write updates to /1.0/system/storage
// so concurrent Terraform resources (e.g. schedules and pools) do not clobber each other.
func (c *Client) MutateSystemStorage(ctx context.Context, fn func(*osapi.SystemStorage) error) error {
	c.storageMu.Lock()
	defer c.storageMu.Unlock()

	current, err := c.GetSystemStorage(ctx)
	if err != nil {
		return err
	}

	if err := fn(current); err != nil {
		return err
	}

	return c.UpdateSystemStorage(ctx, *current)
}

// DeleteStoragePool calls POST /1.0/system/storage/:delete-pool.
func (c *Client) DeleteStoragePool(ctx context.Context, name string) error {
	payload := struct {
		Name string `json:"name"`
	}{Name: name}

	return c.do(ctx, http.MethodPost, "/1.0/system/storage/:delete-pool", payload, nil)
}

// CreateStorageVolumeRequest is the payload for POST /1.0/system/storage/:create-volume.
type CreateStorageVolumeRequest struct {
	Pool  string `json:"pool"`
	Name  string `json:"name"`
	Quota int    `json:"quota,omitempty"`
	Use   string `json:"use"`
}

// CreateStorageVolume calls POST /1.0/system/storage/:create-volume.
func (c *Client) CreateStorageVolume(ctx context.Context, req CreateStorageVolumeRequest) error {
	return c.do(ctx, http.MethodPost, "/1.0/system/storage/:create-volume", req, nil)
}

// DeleteStorageVolumeRequest is the payload for POST /1.0/system/storage/:delete-volume.
type DeleteStorageVolumeRequest struct {
	Pool  string `json:"pool"`
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

// DeleteStorageVolume calls POST /1.0/system/storage/:delete-volume.
func (c *Client) DeleteStorageVolume(ctx context.Context, req DeleteStorageVolumeRequest) error {
	return c.do(ctx, http.MethodPost, "/1.0/system/storage/:delete-volume", req, nil)
}

// GetSystemKernel calls GET /1.0/system/kernel.
func (c *Client) GetSystemKernel(ctx context.Context) (*osapi.SystemKernel, error) {
	var out osapi.SystemKernel
	if err := c.do(ctx, http.MethodGet, "/1.0/system/kernel", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemKernel calls PUT /1.0/system/kernel.
func (c *Client) UpdateSystemKernel(ctx context.Context, cfg osapi.SystemKernel) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/kernel", cfg, nil)
}

// GetSystemLogging calls GET /1.0/system/logging.
func (c *Client) GetSystemLogging(ctx context.Context) (*osapi.SystemLogging, error) {
	var out osapi.SystemLogging
	if err := c.do(ctx, http.MethodGet, "/1.0/system/logging", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemLogging calls PUT /1.0/system/logging.
func (c *Client) UpdateSystemLogging(ctx context.Context, cfg osapi.SystemLogging) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/logging", cfg, nil)
}

// GetSystemPower calls GET /1.0/system/power.
func (c *Client) GetSystemPower(ctx context.Context) (*osapi.SystemPower, error) {
	var out osapi.SystemPower
	if err := c.do(ctx, http.MethodGet, "/1.0/system/power", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemPower calls PUT /1.0/system/power.
func (c *Client) UpdateSystemPower(ctx context.Context, cfg osapi.SystemPower) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/power", cfg, nil)
}

// GetSystemProvider calls GET /1.0/system/provider.
func (c *Client) GetSystemProvider(ctx context.Context) (*osapi.SystemProvider, error) {
	var out osapi.SystemProvider
	if err := c.do(ctx, http.MethodGet, "/1.0/system/provider", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemProvider calls PUT /1.0/system/provider.
func (c *Client) UpdateSystemProvider(ctx context.Context, cfg osapi.SystemProvider) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/provider", cfg, nil)
}

// GetSystemSecurity calls GET /1.0/system/security.
func (c *Client) GetSystemSecurity(ctx context.Context) (*osapi.SystemSecurity, error) {
	var out osapi.SystemSecurity
	if err := c.do(ctx, http.MethodGet, "/1.0/system/security", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemSecurity calls PUT /1.0/system/security.
func (c *Client) UpdateSystemSecurity(ctx context.Context, cfg osapi.SystemSecurity) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/security", cfg, nil)
}

// GetSystemUpdate calls GET /1.0/system/update.
func (c *Client) GetSystemUpdate(ctx context.Context) (*osapi.SystemUpdate, error) {
	var out osapi.SystemUpdate
	if err := c.do(ctx, http.MethodGet, "/1.0/system/update", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemUpdate calls PUT /1.0/system/update.
func (c *Client) UpdateSystemUpdate(ctx context.Context, cfg osapi.SystemUpdate) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/update", cfg, nil)
}

// GetSystemFallbackListener calls GET /1.0/system/fallback-listener.
func (c *Client) GetSystemFallbackListener(ctx context.Context) (*osapi.SystemFallbackListener, error) {
	var out osapi.SystemFallbackListener
	if err := c.do(ctx, http.MethodGet, "/1.0/system/fallback-listener", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateSystemFallbackListener calls PUT /1.0/system/fallback-listener.
func (c *Client) UpdateSystemFallbackListener(ctx context.Context, cfg osapi.SystemFallbackListener) error {
	return c.do(ctx, http.MethodPut, "/1.0/system/fallback-listener", cfg, nil)
}

// GetSystemResources calls GET /1.0/system/resources and returns the raw JSON metadata.
func (c *Client) GetSystemResources(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/1.0/system/resources", nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// --- Services endpoints ---

// ListServices calls GET /1.0/services and returns the service names.
func (c *Client) ListServices(ctx context.Context) ([]string, error) {
	var urls []string
	if err := c.do(ctx, http.MethodGet, "/1.0/services", nil, &urls); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(urls))
	for _, u := range urls {
		parts := strings.Split(strings.TrimRight(u, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			names = append(names, parts[len(parts)-1])
		}
	}

	return names, nil
}

// GetService calls GET /1.0/services/{name} and decodes metadata into dst.
func (c *Client) GetService(ctx context.Context, name string, dst any) error {
	return c.do(ctx, http.MethodGet, "/1.0/services/"+url.PathEscape(name), nil, dst)
}

// UpdateService calls PUT /1.0/services/{name}.
func (c *Client) UpdateService(ctx context.Context, name string, payload any) error {
	return c.do(ctx, http.MethodPut, "/1.0/services/"+url.PathEscape(name), payload, nil)
}

// ResetService calls POST /1.0/services/{name}/:reset.
func (c *Client) ResetService(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/1.0/services/"+url.PathEscape(name)+"/:reset", nil, nil)
}

// --- Applications endpoints ---

// ListApplications calls GET /1.0/applications and returns the installed application names.
func (c *Client) ListApplications(ctx context.Context) ([]string, error) {
	var urls []string
	if err := c.do(ctx, http.MethodGet, "/1.0/applications", nil, &urls); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(urls))
	for _, u := range urls {
		parts := strings.Split(strings.TrimRight(u, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			names = append(names, parts[len(parts)-1])
		}
	}

	return names, nil
}

// InstallApplication calls POST /1.0/applications.
func (c *Client) InstallApplication(ctx context.Context, name string) error {
	payload := struct {
		Name string `json:"name"`
	}{Name: name}

	return c.do(ctx, http.MethodPost, "/1.0/applications", payload, nil)
}

// GetApplication calls GET /1.0/applications/{name}.
func (c *Client) GetApplication(ctx context.Context, name string) (*osapi.Application, error) {
	var out osapi.Application
	if err := c.do(ctx, http.MethodGet, "/1.0/applications/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateApplication calls PUT /1.0/applications/{name}.
func (c *Client) UpdateApplication(ctx context.Context, name string, cfg osapi.Application) error {
	return c.do(ctx, http.MethodPut, "/1.0/applications/"+url.PathEscape(name), cfg, nil)
}

// RemoveApplication calls POST /1.0/applications/{name}/:remove.
func (c *Client) RemoveApplication(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/1.0/applications/"+url.PathEscape(name)+"/:remove", nil, nil)
}
