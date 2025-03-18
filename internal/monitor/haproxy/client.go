package haproxy

import (
	"fmt"
	"net"
)

const (
	SocketPath = "/var/run/haproxy/admin.sock"
	TCPSocket  = "127.0.0.1:9999"
)

// Client represents an HAProxy runtime API client
type Client struct {
	socketPath string
}

// NewClient creates a new HAProxy client
func NewClient() *Client {
	return &Client{
		socketPath: SocketPath,
	}
}

// SendCommand sends a command to the HAProxy Unix socket and returns the response
func (c *Client) SendCommand(command string) (string, error) {
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
func SendTCPCommand(command string) (string, error) {
	conn, err := net.Dial("tcp", TCPSocket)
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
