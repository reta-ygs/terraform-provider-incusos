package client

import (
	"context"
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
	"path/filepath"
	"testing"
	"time"
)

func generateTestCert(t *testing.T, cn string, isServer bool) (certPEM, keyPEM []byte, tlsCert tls.Certificate) {
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

func TestClientMTLSAndServerCertPinning(t *testing.T) {
	serverCertPEM, _, serverTLSCert := generateTestCert(t, "incusos-server", true)
	clientCertPEM, clientKeyPEM, _ := generateTestCert(t, "incusos-client", false)

	clientPool := x509.NewCertPool()
	clientPool.AppendCertsFromPEM(clientCertPEM)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "client cert required", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/os/1.0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":        "sync",
				"status":      "Success",
				"status_code": 200,
				"metadata": map[string]any{
					"environment": map[string]any{
						"hostname":        "node-01",
						"machine_id":      "abc-123",
						"os_name":         "IncusOS",
						"os_version":      "202611200000",
						"system_is_ready": true,
						"uptime":          4200,
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":       "error",
				"error":      "not found",
				"error_code": 404,
			})
		}
	}))
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientPool,
	}
	srv.StartTLS()
	defer srv.Close()

	c, err := New(Config{
		Address:    srv.URL,
		PathPrefix: "/os",
		ClientCert: string(clientCertPEM),
		ClientKey:  string(clientKeyPEM),
		ServerCert: string(serverCertPEM),
	})
	if err != nil {
		t.Fatalf("New client failed: %v", err)
	}

	info, err := c.GetServerInfo(context.Background())
	if err != nil {
		t.Fatalf("GetServerInfo failed: %v", err)
	}
	if info.Environment.Hostname != "node-01" || info.Environment.OSName != "IncusOS" {
		t.Fatalf("unexpected server info: %+v", info)
	}

	_, err = c.GetApplication(context.Background(), "nonexistent")
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound error, got: %v", err)
	}
}

func TestClientUnixSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "keepalive.socket")
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "unix", sockPath)
	if err != nil {
		t.Fatalf("listening on unix socket: %v", err)
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
							"hostname":        "keepalive-node",
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

	c, err := New(Config{
		UnixSocket: sockPath,
		PathPrefix: "/os",
	})
	if err != nil {
		t.Fatalf("New unix socket client failed: %v", err)
	}

	info, err := c.GetServerInfo(context.Background())
	if err != nil {
		t.Fatalf("GetServerInfo via unix socket failed: %v", err)
	}
	if info.Environment.Hostname != "keepalive-node" {
		t.Fatalf("unexpected hostname: %q", info.Environment.Hostname)
	}
}
