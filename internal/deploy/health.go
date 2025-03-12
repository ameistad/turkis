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
	// First get the container's IP address
	cmd := exec.Command("docker", "inspect",
		"--format", "{{.NetworkSettings.Networks.turkis-public.IPAddress}}",
		containerID)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get container IP: %w", err)
	}

	ipAddress := strings.TrimSpace(string(output))
	if ipAddress == "" {
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
