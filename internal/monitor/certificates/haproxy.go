package certificates

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HAProxyNotifier handles notifying HAProxy about certificate changes
type HAProxyNotifier struct {
	socketPath string
	certDir    string
}

// NewHAProxyNotifier creates a new HAProxy notifier
func NewHAProxyNotifier(socketPath, certDir string) *HAProxyNotifier {
	return &HAProxyNotifier{
		socketPath: socketPath,
		certDir:    certDir,
	}
}

// NotifyCertChange notifies HAProxy about a certificate change
func (h *HAProxyNotifier) NotifyCertChange(domain string) error {
	// Check if PEM file exists
	pemPath := filepath.Join(h.certDir, domain+".pem")
	if _, err := os.Stat(pemPath); os.IsNotExist(err) {
		return fmt.Errorf("PEM file does not exist for domain %s", domain)
	}

	// Determine if this is a new certificate or an update
	// First check if the certificate is already loaded
	cmds := []string{
		"show ssl cert",          // List certs
		fmt.Sprintf("new ssl cert %s", pemPath), // Add new cert
		fmt.Sprintf("set ssl cert %s %s", domain, pemPath), // Update existing cert
		"commit ssl cert",        // Apply changes
	}

	for _, cmd := range cmds {
		// Create a new connection for each command
		conn, err := net.Dial("unix", h.socketPath)
		if err != nil {
			return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
		}
		
		// Set timeout
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		
		// Send command to HAProxy
		if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
			conn.Close()
			return fmt.Errorf("failed to send command to HAProxy: %w", err)
		}

		// Read response
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to read response from HAProxy: %w", err)
		}
		
		// Close the connection after each command
		conn.Close()

		response := string(buf[:n])
		if strings.Contains(response, "Unknown command") {
			return fmt.Errorf("HAProxy does not support the command: %s", cmd)
		}
	}

	return nil
}

// UpdateAllCertificates updates all certificates in HAProxy
func (h *HAProxyNotifier) UpdateAllCertificates() error {
	// Get all .pem files in the certificate directory
	files, err := os.ReadDir(h.certDir)
	if err != nil {
		return fmt.Errorf("failed to read certificate directory: %w", err)
	}

	for _, file := range files {
		// Only process .pem files
		if !strings.HasSuffix(file.Name(), ".pem") {
			continue
		}

		// Extract domain name from file name
		domain := strings.TrimSuffix(file.Name(), ".pem")

		// Notify HAProxy about this certificate
		if err := h.NotifyCertChange(domain); err != nil {
			// Log error but continue with other certificates
			fmt.Printf("Failed to update certificate for %s: %v\n", domain, err)
		}
	}

	return nil
}