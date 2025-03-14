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

	// Execute each command in sequence, each with its own connection
	commands := []string{
		"show ssl cert",
		fmt.Sprintf("new ssl cert %s", pemPath),
		fmt.Sprintf("set ssl cert %s %s", domain, pemPath),
		"commit ssl cert",
	}
	
	for i, cmd := range commands {
		// Create a new connection for this command
		conn, err := net.Dial("unix", h.socketPath)
		if err != nil {
			return fmt.Errorf("failed to connect to HAProxy socket for command %d: %w", i+1, err)
		}

		// Set timeout and ensure connection is closed
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		defer conn.Close() // Will be closed at end of function

		// Send command to HAProxy
		if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
			conn.Close() // Close immediately on error
			return fmt.Errorf("failed to send command %d to HAProxy: %w", i+1, err)
		}

		// Read response
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close() // Close immediately on error
			return fmt.Errorf("failed to read response from HAProxy for command %d: %w", i+1, err)
		}

		response := string(buf[:n])
		conn.Close() // Close after reading response

		// Check for error responses
		if strings.Contains(response, "Unknown command") {
			return fmt.Errorf("HAProxy does not support the command: %s", cmd)
		}
		
		// For debugging - print the command and first line of response
		responseLine := strings.Split(response, "\n")[0]
		fmt.Printf("HAProxy command: %s, Response: %s\n", cmd, responseLine)
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