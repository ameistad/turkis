package certificates

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/sirupsen/logrus"
)

// Config holds the configuration for the certificate manager
type Config struct {
	// Email address for Let's Encrypt notifications
	Email string

	// Directory to store certificates
	CertDir string

	// Directory for HTTP-01 challenge responses
	WebRootDir string

	// HAProxy socket path for notifications
	HAProxySocket string

	// Logger
	Logger *logrus.Logger

	// Staging mode for testing
	TlsStaging bool
}

// Domain represents a domain for which we need a certificate
type Domain struct {
	Name    string   // Primary domain name
	Aliases []string // Alternative domain names
}

// Manager handles TLS certificate operations
type Manager struct {
	config Config
	logger *logrus.Logger
	user   *User
	client *lego.Client

	// Map of domains to certificate info
	domains     map[string]*Domain
	domainMutex sync.RWMutex

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// User implements the ACME User interface
type User struct {
	Email        string
	Registration *registration.Resource
	privateKey   crypto.PrivateKey
}

// GetEmail returns the email address of the user
func (u *User) GetEmail() string {
	return u.Email
}

// GetRegistration returns the registration resource
func (u *User) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey returns the private key
func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.privateKey
}

// NewManager creates a new certificate manager
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Logger == nil {
		cfg.Logger = logrus.New()
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(cfg.CertDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create certificate directory: %w", err)
	}

	if err := os.MkdirAll(cfg.WebRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create webroot directory: %w", err)
	}

	// Setup key manager
	keyDir := filepath.Join(cfg.CertDir, "accounts")
	keyManager, err := NewKeyManager(keyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create key manager: %w", err)
	}

	// Load or create user key
	privateKey, err := keyManager.LoadOrCreateKey(cfg.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to load/create user key: %w", err)
	}

	user := &User{
		Email:      cfg.Email,
		privateKey: privateKey,
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:  cfg,
		logger:  cfg.Logger,
		user:    user,
		domains: make(map[string]*Domain),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Initialize Lego client
	if err := m.initClient(); err != nil {
		cancel()
		return nil, err
	}

	return m, nil
}

// initClient initializes the ACME client
func (m *Manager) initClient() error {
	// Create Lego config
	config := lego.NewConfig(m.user)

	// Use staging server if in staging mode
	if m.config.TlsStaging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}

	// Create client
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create lego client: %w", err)
	}

	// Configure HTTP challenge provider using a server that listens on port 8080
	// HAProxy is configured to forward /.well-known/acme-challenge/* requests to this server
	httpProvider := http01.NewProviderServer("", "8080")
	err = client.Challenge.SetHTTP01Provider(httpProvider)
	if err != nil {
		return fmt.Errorf("failed to set HTTP challenge provider: %w", err)
	}

	m.client = client
	return nil
}

// Start begins the certificate manager operation
func (m *Manager) Start() error {
	// Register user account with Let's Encrypt
	reg, err := m.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("failed to register account: %w", err)
	}
	m.user.Registration = reg

	// Start goroutine for certificate renewal checks
	go m.renewalLoop()

	return nil
}

// Stop shuts down the certificate manager
func (m *Manager) Stop() {
	m.cancel()
}

// renewalLoop periodically checks for certificates that need renewal
func (m *Manager) renewalLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Do an initial check
	m.checkRenewals()

	for {
		select {
		case <-ticker.C:
			m.checkRenewals()
		case <-m.ctx.Done():
			return
		}
	}
}

// checkRenewals checks all managed domains for certificate renewal
func (m *Manager) checkRenewals() {
	m.domainMutex.RLock()
	domains := make([]*Domain, 0, len(m.domains))
	for _, domain := range m.domains {
		domains = append(domains, domain)
	}
	m.domainMutex.RUnlock()

	for _, domain := range domains {
		// Check if certificate exists and needs renewal
		certFile := filepath.Join(m.config.CertDir, domain.Name+".crt")
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			// Certificate doesn't exist, obtain it
			m.logger.Infof("Certificate for %s doesn't exist, obtaining new certificate", domain.Name)
			m.obtainCertificate(domain)
			continue
		}

		// Load cert to check expiry
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(m.config.CertDir, domain.Name+".crt"),
			filepath.Join(m.config.CertDir, domain.Name+".key"),
		)
		if err != nil {
			m.logger.Errorf("Failed to load certificate for %s: %v", domain.Name, err)
			continue
		}

		// Check if certificate is about to expire (30 days threshold)
		if len(cert.Certificate) == 0 {
			m.logger.Errorf("Invalid certificate for %s", domain.Name)
			continue
		}

		parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			m.logger.Errorf("Failed to parse certificate for %s: %v", domain.Name, err)
			continue
		}

		// Renew if certificate expires in less than 30 days
		if time.Until(parsedCert.NotAfter) < 30*24*time.Hour {
			m.logger.Infof("Certificate for %s expires soon, renewing", domain.Name)
			m.renewCertificate(domain)
		}
	}
}

// obtainCertificate requests a new certificate for the domain
func (m *Manager) obtainCertificate(domain *Domain) {
	// Prepare domains list (main domain + aliases)
	domains := []string{domain.Name}
	domains = append(domains, domain.Aliases...)

	m.logger.Infof("Obtaining certificate for domains: %v", domains)

	// Request certificate
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certificates, err := m.client.Certificate.Obtain(request)
	if err != nil {
		m.logger.Errorf("Failed to obtain certificate for %s: %v", domain.Name, err)
		return
	}

	// Save the certificate
	err = m.saveCertificate(domain.Name, certificates)
	if err != nil {
		m.logger.Errorf("Failed to save certificate for %s: %v", domain.Name, err)
		return
	}

	// Notify HAProxy
	m.logger.Infof("Notifying HAProxy about new certificate for %s", domain.Name)
	err = m.notifyHAProxy(domain.Name)
	if err != nil {
		m.logger.Errorf("Failed to notify HAProxy about new certificate for %s: %v", domain.Name, err)
	} else {
		m.logger.Infof("Successfully notified HAProxy about new certificate for %s", domain.Name)
	}
}

// renewCertificate renews an existing certificate
func (m *Manager) renewCertificate(domain *Domain) {
	// Implementation similar to obtainCertificate but using Renew instead of Obtain
	// For now, we'll just reuse obtain since the Lego client handles both cases
	m.obtainCertificate(domain)
}

// saveCertificate saves the certificate files to disk
func (m *Manager) saveCertificate(domain string, cert *certificate.Resource) error {
	// Save certificate
	certPath := filepath.Join(m.config.CertDir, domain+".crt")
	if err := os.WriteFile(certPath, cert.Certificate, 0644); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Save private key
	keyPath := filepath.Join(m.config.CertDir, domain+".key")
	if err := os.WriteFile(keyPath, cert.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Create HAProxy PEM (concat cert and key)
	pemPath := filepath.Join(m.config.CertDir, domain+".pem")
	pemContent := append(cert.Certificate, '\n')
	pemContent = append(pemContent, cert.PrivateKey...)

	if err := os.WriteFile(pemPath, pemContent, 0600); err != nil {
		return fmt.Errorf("failed to save PEM file: %w", err)
	}

	return nil
}

// notifyHAProxy notifies HAProxy about the new certificate
func (m *Manager) notifyHAProxy(domain string) error {
	// Create HAProxy notifier
	notifier := NewHAProxyNotifier(m.config.HAProxySocket, m.config.CertDir)

	// Notify HAProxy about the certificate change
	return notifier.NotifyCertChange(domain)
}

// AddDomain adds a domain to be managed for certificates
func (m *Manager) AddDomain(domain *Domain) {
	m.domainMutex.Lock()
	defer m.domainMutex.Unlock()

	// Skip if we're already managing this domain
	if _, exists := m.domains[domain.Name]; exists {
		return
	}

	m.domains[domain.Name] = domain

	// Start a goroutine to obtain certificate if needed
	go func() {
		// Check if certificate exists
		certFile := filepath.Join(m.config.CertDir, domain.Name+".crt")
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			// Certificate doesn't exist, obtain it
			m.obtainCertificate(domain)
		}
	}()
}

// RemoveDomain removes a domain from being managed
func (m *Manager) RemoveDomain(domainName string) {
	m.domainMutex.Lock()
	defer m.domainMutex.Unlock()

	delete(m.domains, domainName)
	// Note: We don't delete the certificate files as they might be needed again
}
