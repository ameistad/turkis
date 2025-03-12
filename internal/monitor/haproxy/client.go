package haproxy

import (
	"fmt"
	"log"
	"net"
)

// Client represents an HAProxy runtime API client
type Client struct {
	socketPath string
	dryRun     bool
}

// NewClient creates a new HAProxy client
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		dryRun:     false,
	}
}

// NewDryRunClient creates a new HAProxy client in dry run mode
func NewDryRunClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		dryRun:     true,
	}
}

// SetDryRun enables or disables dry run mode
func (c *Client) SetDryRun(dryRun bool) {
	c.dryRun = dryRun
}

// IsDryRun returns whether the client is in dry run mode
func (c *Client) IsDryRun() bool {
	return c.dryRun
}

// SendCommand sends a command to the HAProxy socket and returns the response
// In dry run mode, it just logs the command without sending it
func (c *Client) SendCommand(command string) (string, error) {
	if c.dryRun {
		log.Printf("[DRY RUN] Would send to HAProxy: %s", command)
		return "[DRY RUN] Command not actually sent", nil
	}

	// Only attempt socket connection if not in dry run mode
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return "", fmt.Errorf("could not connect to HAProxy socket: %w", err)
	}
	defer conn.Close()

	// Send the command
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		return "", fmt.Errorf("error sending command to HAProxy: %w", err)
	}

	// Read the response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("error reading from HAProxy socket: %w", err)
	}

	return string(buf[:n]), nil
}

// SendTCPCommand sends a command to HAProxy via TCP socket
func SendTCPCommand(address string, command string, dryRun bool) (string, error) {
	if dryRun {
		log.Printf("[DRY RUN] Would send to HAProxy TCP socket: %s", command)
		return "[DRY RUN] Command not actually sent", nil
	}

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return "", fmt.Errorf("could not connect to HAProxy TCP socket: %w", err)
	}
	defer conn.Close()

	// Send the command
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		return "", fmt.Errorf("error sending command to HAProxy: %w", err)
	}

	// Read the response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("error reading from HAProxy socket: %w", err)
	}

	return string(buf[:n]), nil
}
