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
	fmt.Printf("=== CERTIFICATE UPDATE PROCESS FOR %s ===\n", domain)
	
	// Check if PEM file exists and has content
	pemPath := filepath.Join(h.certDir, domain+".pem")
	if fileInfo, err := os.Stat(pemPath); os.IsNotExist(err) {
		return fmt.Errorf("PEM file does not exist for domain %s", domain)
	} else {
		fmt.Printf("PEM file found at %s (size: %d bytes)\n", pemPath, fileInfo.Size())
		
		// Check file content to ensure it's a valid certificate
		pemData, err := os.ReadFile(pemPath)
		if err != nil {
			fmt.Printf("WARNING: Failed to read PEM file: %v\n", err)
		} else {
			if len(pemData) < 100 {
				fmt.Printf("WARNING: PEM file might be invalid - very small size: %d bytes\n", len(pemData))
			} else {
				fmt.Printf("PEM file content looks valid (%d bytes)\n", len(pemData))
			}
		}
	}

	// Debug the HAProxy configuration first
	fmt.Println("=== CHECKING CURRENT HAPROXY CONFIGURATION ===")
	debugHAProxyConfig(h.socketPath)
	
	// Execute each command in sequence with detailed logging
	commands := []string{
		"show ssl cert",                               // 1. List existing certs
		fmt.Sprintf("new ssl cert %s", pemPath),       // 2. Add new cert
		fmt.Sprintf("set ssl cert %s %s", domain, pemPath), // 3. Associate domain with cert
		"commit ssl cert",                             // 4. Apply changes
	}
	
	fmt.Println("=== APPLYING CERTIFICATE UPDATES ===")
	for i, cmd := range commands {
		fmt.Printf("Command %d/%d: %s\n", i+1, len(commands), cmd)
		
		// Create a new connection for this command
		conn, err := net.Dial("unix", h.socketPath)
		if err != nil {
			fmt.Printf("ERROR: Failed to connect to HAProxy socket: %v\n", err)
			return fmt.Errorf("failed to connect to HAProxy socket for command %d: %w", i+1, err)
		}

		// Set timeout
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Send command to HAProxy
		if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
			conn.Close()
			fmt.Printf("ERROR: Failed to send command: %v\n", err)
			return fmt.Errorf("failed to send command %d to HAProxy: %w", i+1, err)
		}

		// Read response
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			fmt.Printf("ERROR: Failed to read response: %v\n", err)
			return fmt.Errorf("failed to read response from HAProxy for command %d: %w", i+1, err)
		}

		response := string(buf[:n])
		conn.Close()

		// Format and print response
		responseLines := strings.Split(response, "\n")
		if len(responseLines) > 10 {
			// For long responses, print first 5 and last 5 lines
			fmt.Println("Response (truncated):")
			for i := 0; i < 5; i++ {
				if i < len(responseLines) && responseLines[i] != "" {
					fmt.Printf("  %s\n", responseLines[i])
				}
			}
			fmt.Println("  [...]")
			for i := len(responseLines) - 5; i < len(responseLines); i++ {
				if i >= 0 && i < len(responseLines) && responseLines[i] != "" {
					fmt.Printf("  %s\n", responseLines[i])
				}
			}
		} else {
			// For short responses, print all lines
			fmt.Println("Response:")
			for _, line := range responseLines {
				if line != "" {
					fmt.Printf("  %s\n", line)
				}
			}
		}

		// Check for error responses
		if strings.Contains(response, "Unknown command") {
			fmt.Printf("ERROR: Command not supported: %s\n", cmd)
			return fmt.Errorf("HAProxy does not support the command: %s", cmd)
		}
	}

	// Check frontend binding config to ensure SSL is enabled
	fmt.Println("=== CHECKING FRONTEND CONFIGURATION ===")
	
	// Get current bind settings
	bindConn, err := net.Dial("unix", h.socketPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to HAProxy socket for bind check: %v\n", err)
		return fmt.Errorf("failed to connect to HAProxy socket for bind check: %w", err)
	}
	bindConn.SetDeadline(time.Now().Add(5 * time.Second))
	
	// Show binds for HTTPS frontend
	if _, err := bindConn.Write([]byte("show bind\n")); err != nil {
		bindConn.Close()
		fmt.Printf("ERROR: Failed to send 'show bind' command: %v\n", err)
		return fmt.Errorf("failed to check HAProxy binds: %w", err)
	}
	
	bindBuf := make([]byte, 8192)
	bindN, err := bindConn.Read(bindBuf)
	bindConn.Close()
	if err != nil {
		fmt.Printf("ERROR: Failed to read bind response: %v\n", err)
		return fmt.Errorf("failed to read HAProxy binds: %w", err)
	}
	
	binds := string(bindBuf[:bindN])
	fmt.Println("Current bind configuration:")
	for _, line := range strings.Split(binds, "\n") {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}
	
	// Check if frontend has SSL enabled
	httpsBindFound := false
	sslBindFound := false
	
	for _, line := range strings.Split(binds, "\n") {
		if strings.Contains(line, "https-in") {
			httpsBindFound = true
			if strings.Contains(line, "ssl") {
				sslBindFound = true
				fmt.Println("✓ SSL is ENABLED on the HTTPS frontend")
			}
		}
	}
	
	if !httpsBindFound {
		fmt.Println("✗ HTTPS frontend binding not found!")
	} else if !sslBindFound {
		fmt.Println("✗ SSL is NOT enabled on the HTTPS frontend - certificates won't be used!")
		
		// Try to update the frontend to enable SSL
		fmt.Println("Attempting to enable SSL on HTTPS frontend...")
		
		sslConn, err := net.Dial("unix", h.socketPath)
		if err != nil {
			fmt.Printf("ERROR: Failed to connect to HAProxy socket for SSL configuration: %v\n", err)
		} else {
			sslConn.SetDeadline(time.Now().Add(5 * time.Second))
			
			// This command is experimental - it might not work on all HAProxy versions
			sslEnableCmd := "set ssl cert-list 1 " + pemPath
			fmt.Printf("Sending command: %s\n", sslEnableCmd)
			
			if _, err := sslConn.Write([]byte(sslEnableCmd + "\n")); err != nil {
				fmt.Printf("ERROR: Failed to send SSL enable command: %v\n", err)
			} else {
				sslBuf := make([]byte, 4096)
				sslN, err := sslConn.Read(sslBuf)
				if err != nil {
					fmt.Printf("ERROR: Failed to read SSL enable response: %v\n", err)
				} else {
					sslResp := string(sslBuf[:sslN])
					fmt.Printf("Response: %s\n", sslResp)
					
					if strings.Contains(sslResp, "Unknown command") || strings.Contains(sslResp, "Error") {
						fmt.Println("✗ Failed to enable SSL on the frontend")
					} else {
						fmt.Println("✓ SSL configuration updated on the frontend")
					}
				}
			}
			sslConn.Close()
		}
	}
	
	// Verify the certificate is present in HAProxy's certificate storage
	fmt.Println("=== VERIFYING CERTIFICATE STORAGE ===")
	verifyConn, err := net.Dial("unix", h.socketPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to HAProxy socket for verification: %v\n", err)
	} else {
		verifyConn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := verifyConn.Write([]byte("show ssl cert\n")); err != nil {
			fmt.Printf("ERROR: Failed to send certificate verification command: %v\n", err)
		} else {
			verifyBuf := make([]byte, 8192)
			verifyN, err := verifyConn.Read(verifyBuf)
			if err != nil {
				fmt.Printf("ERROR: Failed to read certificate verification response: %v\n", err)
			} else {
				verifyCerts := string(verifyBuf[:verifyN])
				fmt.Println("Certificate storage:")
				certFound := false
				
				for _, line := range strings.Split(verifyCerts, "\n") {
					if strings.Contains(line, pemPath) {
						certFound = true
						fmt.Printf("  ✓ %s\n", line)
					} else if line != "" {
						fmt.Printf("  %s\n", line)
					}
				}
				
				if !certFound {
					fmt.Printf("✗ Certificate %s not found in HAProxy's certificate storage!\n", pemPath)
				}
			}
		}
		verifyConn.Close()
	}
	
	fmt.Println("=== CERTIFICATE UPDATE PROCESS COMPLETE ===")
	return nil
}

// debugHAProxyConfig prints detailed HAProxy configuration information
func debugHAProxyConfig(socketPath string) {
	// Get basic stats
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to HAProxy socket: %v\n", err)
		return
	}
	
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("show info\n")); err != nil {
		conn.Close()
		fmt.Printf("ERROR: Failed to send info command: %v\n", err)
		return
	}
	
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	conn.Close()
	if err != nil {
		fmt.Printf("ERROR: Failed to read info response: %v\n", err)
		return
	}
	
	info := string(buf[:n])
	fmt.Println("HAProxy version info:")
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "Version:") || strings.HasPrefix(line, "Release_date:") {
			fmt.Printf("  %s\n", line)
		}
	}
	
	// Check frontend configuration
	frontConn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to HAProxy socket: %v\n", err)
		return
	}
	
	frontConn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := frontConn.Write([]byte("show frontend\n")); err != nil {
		frontConn.Close()
		fmt.Printf("ERROR: Failed to send frontend command: %v\n", err)
		return
	}
	
	frontBuf := make([]byte, 8192)
	frontN, err := frontConn.Read(frontBuf)
	frontConn.Close()
	if err != nil {
		fmt.Printf("ERROR: Failed to read frontend response: %v\n", err)
		return
	}
	
	frontends := string(frontBuf[:frontN])
	fmt.Println("Frontend configuration:")
	for _, line := range strings.Split(frontends, "\n") {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}
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