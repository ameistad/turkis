package monitor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
)

// Common errors
var (
	ErrNoIPAddress     = errors.New("container has no IP address on the specified network")
	ErrNetworkNotFound = errors.New("specified network not found")
	ErrNoPortsExposed  = errors.New("container has no exposed ports")
)

// ContainerNetworkInfo extracts the container's IP address and exposed ports
func ContainerNetworkInfo(container types.ContainerJSON, networkName string) (string, map[nat.Port][]nat.PortBinding, error) {
	// Check if the network exists
	if _, exists := container.NetworkSettings.Networks[networkName]; !exists {
		return "", nil, fmt.Errorf("%w: %s", ErrNetworkNotFound, networkName)
	}

	// Get IP address from the specified network
	ipAddress := container.NetworkSettings.Networks[networkName].IPAddress
	if ipAddress == "" {
		return "", nil, fmt.Errorf("%w: %s", ErrNoIPAddress, networkName)
	}

	// Get all exposed ports
	exposedPorts := container.NetworkSettings.Ports
	if len(exposedPorts) == 0 {
		// Not returning an error here as a container might legitimately have no ports
		// Just include the empty map with the IP address
	}

	return ipAddress, exposedPorts, nil
}

// PrimaryPort tries to determine the main web port (80, 8080, etc.)
func PrimaryPort(container types.ContainerJSON) (string, error) {
	if container.NetworkSettings == nil || container.NetworkSettings.Ports == nil {
		return "", ErrNoPortsExposed
	}

	// Check if port 80 is exposed
	if _, exists := container.NetworkSettings.Ports[nat.Port("80/tcp")]; exists {
		return "80", nil
	}

	// Check if port 8080 is exposed
	if _, exists := container.NetworkSettings.Ports[nat.Port("8080/tcp")]; exists {
		return "8080", nil
	}

	// Check for any HTTP-like port (common web ports)
	commonPorts := []string{"8000/tcp", "8888/tcp", "3000/tcp", "5000/tcp"}
	for _, portString := range commonPorts {
		port := nat.Port(portString)
		if _, exists := container.NetworkSettings.Ports[port]; exists {
			return strings.Split(portString, "/")[0], nil
		}
	}

	// Check if there's a port defined in labels
	if portLabel, exists := container.Config.Labels["turkis.port"]; exists && portLabel != "" {
		return portLabel, nil
	}

	// Default to the first exposed port if any
	for port := range container.NetworkSettings.Ports {
		return strings.Split(string(port), "/")[0], nil
	}

	// No ports found
	return "", ErrNoPortsExposed
}
