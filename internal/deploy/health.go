package deploy

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// HealthCheckContainer performs an HTTP health check on the specified container.
func HealthCheckContainer(containerID, healthCheckPath string) error {
	// First try to get the container's IP address on turkis-public network
	cmd := exec.Command("docker", "inspect",
		"--format", "{{.NetworkSettings.Networks.turkis-public.IPAddress}}",
		containerID)

	output, err := cmd.CombinedOutput() // Use CombinedOutput to get error messages too
	if err != nil {
		// If that fails, try to connect the container to the turkis-public network
		fmt.Printf("Warning: Container not connected to turkis-public network. Trying to connect it...\n")
		connectCmd := exec.Command("docker", "network", "connect", "turkis-public", containerID)
		if connectErr := connectCmd.Run(); connectErr != nil {
			return fmt.Errorf("failed to connect container to turkis-public network: %w", connectErr)
		}
		
		// Try again after connecting
		cmd = exec.Command("docker", "inspect",
			"--format", "{{.NetworkSettings.Networks.turkis-public.IPAddress}}",
			containerID)
		output, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get container IP after connecting to network: %w", err)
		}
	}

	ipAddress := strings.TrimSpace(string(output))
	if ipAddress == "" {
		// If IP is still empty, try to inspect the container to see all network settings
		inspectCmd := exec.Command("docker", "inspect", "--format", "{{json .NetworkSettings.Networks}}", containerID)
		inspectOutput, inspectErr := inspectCmd.Output()
		if inspectErr == nil {
			fmt.Printf("Available networks for container: %s\n", string(inspectOutput))
		}
		
		return fmt.Errorf("container has no IP address on turkis-public network")
	}

	// Ensure health check path starts with '/'
	if !strings.HasPrefix(healthCheckPath, "/") {
		healthCheckPath = "/" + healthCheckPath
	}

	// Construct health check URL
	healthURL := fmt.Sprintf("http://%s:80%s", ipAddress, healthCheckPath)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Try health checks multiple times
	maxRetries := 10
	retryInterval := 2 * time.Second

	fmt.Printf("Performing health checks against %s\n", healthURL)

	for i := 0; i < maxRetries; i++ {
		resp, err := client.Get(healthURL)
		if err != nil {
			fmt.Printf("Health check attempt %d: Connection error: %v\n", i+1, err)
			time.Sleep(retryInterval)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			fmt.Printf("Health check passed on attempt %d with status code %d\n", i+1, resp.StatusCode)
			return nil
		}

		fmt.Printf("Health check attempt %d: Received status code %d\n", i+1, resp.StatusCode)
		time.Sleep(retryInterval)
	}

	return fmt.Errorf("health check failed after %d attempts", maxRetries)
}
