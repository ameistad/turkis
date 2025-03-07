package deploy

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/ameistad/turkis/config"
	"github.com/fatih/color"
)

// getContainerIP wraps the docker inspect call to retrieve a container's IP on a given network.
func getContainerIP(containerID, networkName string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format",
		fmt.Sprintf("{{(index .NetworkSettings.Networks \"%s\").IPAddress}}", networkName),
		containerID)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get container IP: %w", err)
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no IP address found for container %s on network %s", containerID, networkName)
	}
	return ip, nil
}

// HealthCheckContainer performs an HTTP health check on the specified container.
func HealthCheckContainer(containerID string, healthCheckPath string) error {
	ip, err := getContainerIP(containerID, config.DockerNetwork)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%d%s", ip, config.DefaultContainerPort, healthCheckPath)

	// Set up a context with timeout for the overall health check process.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	interval := 2 * time.Second
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		// Check if context is done.
		select {
		case <-ctx.Done():
			return fmt.Errorf(`health check timeout for URL %s

Troubleshooting tips:
- %s
- %s
- %s
- %s`,
				url,
				color.YellowString("Check the container logs."),
				color.YellowString("Verify that the health endpoint is correctly implemented and accessible."),
				color.YellowString("Ensure that any dependencies needed by the container are available."),
				color.YellowString("Review the container configuration for any resource constraints."))
		default:
			resp, err := client.Get(url)
			if err != nil {
				fmt.Printf("Error GET %s: %v. Retrying...\n", url, err)
				time.Sleep(interval)
				continue
			}
			// Ensure the body is closed immediately.
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Printf("Health check passed: %s returned status %d\n", url, resp.StatusCode)
				return nil
			}
			fmt.Printf("Health check returned status %d for %s. Retrying...\n", resp.StatusCode, url)
			time.Sleep(interval)
		}
	}
}

// Health represents the container's health state.
// type Health struct {
// 	Status string `json:"Status"`
// }

// func ImageHasHealthCheck(imageName string) (bool, error) {
// 	// Use --format to extract the Healthcheck configuration as JSON.
// 	out, err := exec.Command("docker", "inspect", "--format", "{{json .Config.Healthcheck}}", imageName).Output()
// 	if err != nil {
// 		return false, fmt.Errorf("docker inspect failed: %w", err)
// 	}
// 	trimmed := strings.TrimSpace(string(out))
// 	if trimmed == "null" {
// 		// Healthcheck not defined.
// 		return false, nil
// 	}
// 	// Try parsing the JSON to be sure it's valid.
// 	var hc interface{}
// 	if err := json.Unmarshal([]byte(trimmed), &hc); err != nil {
// 		return false, fmt.Errorf("failed to parse healthcheck JSON: %w", err)
// 	}
// 	return true, nil
// }

// // HealthCheckContainer checks the health of a container until it becomes healthy or times out.
// func HealthCheckContainer(containerID string) error {
// 	timeout := 60 * time.Second
// 	interval := 2 * time.Second
// 	deadline := time.Now().Add(timeout)

// 	for time.Now().Before(deadline) {
// 		out, err := exec.Command("docker", "inspect", "--format", "{{json .State.Health}}", containerID).Output()
// 		if err != nil {
// 			fmt.Printf("Error inspecting container %s: %v. Retrying...\n", containerID, err)
// 			time.Sleep(interval)
// 			continue
// 		}

// 		trimmed := strings.TrimSpace(string(out))
// 		// If no health check is defined, Docker returns "null".
// 		if trimmed == "null" {
// 			fmt.Printf("Warning: container %s does not have a HEALTHCHECK defined; assuming healthy...\n", containerID)
// 			return nil
// 		}

// 		var health Health
// 		if err := json.Unmarshal([]byte(trimmed), &health); err != nil {
// 			fmt.Printf("Error parsing health info for container %s: %v. Retrying...\n", containerID, err)
// 			time.Sleep(interval)
// 			continue
// 		}
// 		fmt.Printf("Container status: %s\n", health.Status)
// 		if health.Status == "healthy" {
// 			return nil
// 		}
// 		time.Sleep(interval)
// 	}
// 	return fmt.Errorf(`health check timeout for container %s

// 	Troubleshooting tips:
// 	- %s
// 	- %s
// 	- %s
// 	- %s
// 	`, containerID,
// 		color.YellowString("Check the container logs: docker logs %s", containerID),
// 		color.YellowString("Verify that the HEALTHCHECK instruction is correctly defined in your Dockerfile."),
// 		color.YellowString("Ensure that any dependencies needed by the container are available."),
// 		color.YellowString("Review the container configuration for any resource constraints."))
// }
