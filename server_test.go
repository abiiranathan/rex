package rex

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestHandler is a simple handler for testing
type TestHandler struct{}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func TestNewServer(t *testing.T) {
	handler := &TestHandler{}
	server := NewServer(":0", handler)

	if server == nil {
		t.Fatal("Expected server to not be nil")
	}

	if server.Handler != handler {
		t.Error("Handler not properly set")
	}

	// Verify default timeouts
	if server.ReadTimeout != 5*time.Second {
		t.Errorf("Expected ReadTimeout to be 5s, got %v", server.ReadTimeout)
	}
	if server.WriteTimeout != 10*time.Second {
		t.Errorf("Expected WriteTimeout to be 10s, got %v", server.WriteTimeout)
	}
	if server.IdleTimeout != 15*time.Second {
		t.Errorf("Expected IdleTimeout to be 15s, got %v", server.IdleTimeout)
	}

	// Verify HTTP/2 support
	if server.TLSConfig == nil {
		t.Error("TLS Config not initialized")
	}
	if len(server.TLSConfig.NextProtos) != 2 ||
		server.TLSConfig.NextProtos[0] != "h2" ||
		server.TLSConfig.NextProtos[1] != "http/1.1" {
		t.Error("HTTP/2 not properly configured")
	}
}

func TestServerOptions(t *testing.T) {
	tests := []struct {
		name     string
		option   ServerOption
		validate func(*testing.T, *Server)
	}{
		{
			name:   "ReadTimeout",
			option: WithReadTimeout(20 * time.Second),
			validate: func(t *testing.T, s *Server) {
				if s.ReadTimeout != 20*time.Second {
					t.Errorf("ReadTimeout not set correctly, got %v", s.ReadTimeout)
				}
			},
		},
		{
			name:   "WriteTimeout",
			option: WithWriteTimeout(25 * time.Second),
			validate: func(t *testing.T, s *Server) {
				if s.WriteTimeout != 25*time.Second {
					t.Errorf("WriteTimeout not set correctly, got %v", s.WriteTimeout)
				}
			},
		},
		{
			name:   "IdleTimeout",
			option: WithIdleTimeout(30 * time.Second),
			validate: func(t *testing.T, s *Server) {
				if s.IdleTimeout != 30*time.Second {
					t.Errorf("IdleTimeout not set correctly, got %v", s.IdleTimeout)
				}
			},
		},
		{
			name: "TLSConfig",
			option: WithTLSConfig(&tls.Config{
				MinVersion: tls.VersionTLS13,
			}),
			validate: func(t *testing.T, s *Server) {
				if s.TLSConfig.MinVersion != tls.VersionTLS13 {
					t.Error("TLS Config not set correctly")
				}

				// Verify HTTP/2 support maintained
				if !slices.Contains(s.TLSConfig.NextProtos, "h2") {
					t.Error("HTTP/2 support not maintained in TLS config")
				}
			},
		},
		{
			name: "HTTP2Options",
			option: WithHTTP2Options(http2.Server{
				MaxHandlers: 1000,
			}),
			validate: func(t *testing.T, s *Server) {
				// Not much we can validate here as http2.Server settings
				// are not directly accessible after configuration
				if s.TLSConfig == nil || !slices.Contains(s.TLSConfig.NextProtos, "h2") {
					t.Error("HTTP/2 support not maintained")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer(":0", &TestHandler{}, tt.option)
			tt.validate(t, server)
		})
	}
}

func TestServerShutdown(t *testing.T) {
	server := NewServer(":0", &TestHandler{})

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			t.Errorf("Expected ErrServerClosed, got %v", err)
		}
	}()

	// Allow server to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	go func() {
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	server.Shutdown(2 * time.Second)
}

// CertConfig holds configuration for certificate generation
type CertConfig struct {
	Organization string
	CommonName   string
	DNSNames     []string
	IPAddresses  []net.IP
	ValidFrom    time.Time
	ValidFor     time.Duration
}

// DefaultCertConfig returns a default configuration for testing
func DefaultCertConfig() CertConfig {
	return CertConfig{
		Organization: "Test Org",
		CommonName:   "localhost",
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		ValidFrom:    time.Now(),
		ValidFor:     365 * 24 * time.Hour, // 1 year
	}
}

// GenerateCert generates a self-signed certificate and key pair
func GenerateCert(config CertConfig) (certPEM, keyPEM []byte, err error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{config.Organization},
			CommonName:   config.CommonName,
		},
		DNSNames:    config.DNSNames,
		IPAddresses: config.IPAddresses,

		NotBefore: config.ValidFrom,
		NotAfter:  config.ValidFrom.Add(config.ValidFor),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// Encode private key
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	return certPEM, keyPEM, nil
}

// WriteCertFiles writes the certificate and key to files
func WriteCertFiles(certPEM, keyPEM []byte, certPath, keyPath string) error {
	// Create directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return err
	}

	// Write cert file
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}

	// Write key file
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return err
	}

	return nil
}

func TestGenerateCert(t *testing.T) {
	config := DefaultCertConfig()
	certPEM, keyPEM, err := GenerateCert(config)

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify PEM blocks are not empty
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Error("Generated PEM blocks are empty")
	}

	// Try to parse the certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify certificate fields
	if cert.Subject.Organization[0] != config.Organization {
		t.Errorf("Expected org %s, got %s", config.Organization, cert.Subject.Organization[0])
	}
	if cert.Subject.CommonName != config.CommonName {
		t.Errorf("Expected CN %s, got %s", config.CommonName, cert.Subject.CommonName)
	}
}

func TestWriteCertFiles(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "cert-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Generate and write certificates
	certPEM, keyPEM, err := GenerateCert(DefaultCertConfig())
	if err != nil {
		t.Fatal(err)
	}

	err = WriteCertFiles(certPEM, keyPEM, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify files exist with correct permissions
	certInfo, err := os.Stat(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if certInfo.Mode().Perm() != 0644 {
		t.Errorf("Expected cert file permissions 0644, got %v", certInfo.Mode().Perm())
	}

	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Errorf("Expected key file permissions 0600, got %v", keyInfo.Mode().Perm())
	}

	// Verify we can load the certificate
	config, err := LoadTLSConfig(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(config.Certificates) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(config.Certificates))
	}
}
