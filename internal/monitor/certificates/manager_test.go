package certificates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewManager(t *testing.T) {
	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "certificate-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	certDir := filepath.Join(tmpDir, "certs")
	webRootDir := filepath.Join(tmpDir, "webroot")

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Test config
	cfg := Config{
		Email:         "test@example.com",
		CertDir:       certDir,
		WebRootDir:    webRootDir,
		HAProxySocket: filepath.Join(tmpDir, "haproxy.sock"),
		Logger:        logger,
		TlsStaging:    true, // Always use staging for tests
	}

	// Create manager
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Verify directories were created
	if _, err := os.Stat(certDir); os.IsNotExist(err) {
		t.Errorf("Certificate directory was not created")
	}

	if _, err := os.Stat(webRootDir); os.IsNotExist(err) {
		t.Errorf("Webroot directory was not created")
	}

	// Clean up by stopping the manager
	manager.Stop()
}

func TestDomainProvider(t *testing.T) {
	// Create a test domain provider
	provider := &testDomainProvider{
		domains: map[string][]string{
			"example.com": {"www.example.com"},
			"test.com":    {"www.test.com", "api.test.com"},
		},
	}

	// Get domains from provider
	domains := provider.GetAllDomains()

	// Verify domains are returned correctly
	if len(domains) != 2 {
		t.Errorf("Expected 2 domains, got %d", len(domains))
	}

	// Check specific domains
	if aliases, ok := domains["example.com"]; !ok || len(aliases) != 1 {
		t.Errorf("Expected example.com with 1 alias")
	}

	if aliases, ok := domains["test.com"]; !ok || len(aliases) != 2 {
		t.Errorf("Expected test.com with 2 aliases")
	}
}

type testDomainProvider struct {
	domains map[string][]string
}

func (p *testDomainProvider) GetAllDomains() map[string][]string {
	return p.domains
}
