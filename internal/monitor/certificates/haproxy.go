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

	// First, check if certificate is already loaded
	conn1, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
	}
	conn1.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn1.Write([]byte("show ssl cert\n")); err != nil {
		conn1.Close()
		return fmt.Errorf("failed to send command to HAProxy: %w", err)
	}
	buf1 := make([]byte, 4096)
	n1, err := conn1.Read(buf1)
	conn1.Close()
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}
	certListing := string(buf1[:n1])

	// Create a new connection for adding/updating the certificate
	conn2, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
	}
	defer conn2.Close()
	conn2.SetDeadline(time.Now().Add(5 * time.Second))

	// Add the new certificate (using the absolute path)
	cmd := fmt.Sprintf("new ssl cert %s\n", pemPath)
	if _, err := conn2.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("failed to send command to HAProxy: %w", err)
	}
	buf2 := make([]byte, 4096)
	n2, err := conn2.Read(buf2)
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}
	addResponse := string(buf2[:n2])
	
	// Create a new connection for associating the certificate with the domain
	conn3, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
	}
	defer conn3.Close()
	conn3.SetDeadline(time.Now().Add(5 * time.Second))
	
	// Set the certificate for the domain - this binds the cert to the specific domain
	cmdSet := fmt.Sprintf("set ssl cert %s %s\n", domain, pemPath)
	if _, err := conn3.Write([]byte(cmdSet)); err != nil {
		return fmt.Errorf("failed to send set command to HAProxy: %w", err)
	}
	buf3 := make([]byte, 4096)
	n3, err := conn3.Read(buf3)
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}
	setResponse := string(buf3[:n3])

	// Create a new connection for committing the changes
	conn4, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
	}
	defer conn4.Close()
	conn4.SetDeadline(time.Now().Add(5 * time.Second))
	
	// Commit the changes
	if _, err := conn4.Write([]byte("commit ssl cert\n")); err != nil {
		return fmt.Errorf("failed to send commit command to HAProxy: %w", err)
	}
	buf4 := make([]byte, 4096)
	n4, err := conn4.Read(buf4)
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}
	commitResponse := string(buf4[:n4])

	// Check for errors in responses
	for cmd, resp := range map[string]string{
		"new ssl cert": addResponse,
		"set ssl cert": setResponse,
		"commit ssl cert": commitResponse,
	} {
		if strings.Contains(resp, "Unknown command") {
			return fmt.Errorf("HAProxy does not support the command: %s", cmd)
		}
	}

	// Update the frontend bind line to include the new certificate
	conn5, err := net.Dial("unix", h.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy socket: %w", err)
	}
	defer conn5.Close()
	conn5.SetDeadline(time.Now().Add(5 * time.Second))
	
	// Add certificate to the SSL frontend
	updateCmd := fmt.Sprintf("set ssl cert-list 1 %s\n", pemPath)
	if _, err := conn5.Write([]byte(updateCmd)); err != nil {
		return fmt.Errorf("failed to send update command to HAProxy: %w", err)
	}
	buf5 := make([]byte, 4096)
	n5, err := conn5.Read(buf5)
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}
	updateResponse := string(buf5[:n5])
	
	// Check for errors
	if strings.Contains(updateResponse, "Unknown command") {
		return fmt.Errorf("HAProxy does not support the set ssl cert-list command")
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