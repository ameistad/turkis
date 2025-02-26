package deploy

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Health represents the container's health state.
type Health struct {
	Status string `json:"Status"`
}

// HealthCheckContainer checks the health of a container until it becomes healthy or times out.
func HealthCheckContainer(containerID string) error {
	timeout := 60 * time.Second
	interval := 2 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "inspect", "--format", "{{json .State.Health}}", containerID).Output()
		if err != nil {
			fmt.Printf("Error inspecting container %s: %v. Retrying...\n", containerID, err)
			time.Sleep(interval)
			continue
		}

		trimmed := strings.TrimSpace(string(out))
		// If no health check is defined, Docker returns "null".
		if trimmed == "null" {
			fmt.Printf("Warning: container %s does not have a HEALTHCHECK defined; assuming healthy...\n", containerID)
			return nil
		}

		var health Health
		if err := json.Unmarshal([]byte(trimmed), &health); err != nil {
			fmt.Printf("Error parsing health info for container %s: %v. Retrying...\n", containerID, err)
			time.Sleep(interval)
			continue
		}
		fmt.Printf("Container status: %s\n", health.Status)
		if health.Status == "healthy" {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf(`health check timeout for container %s

	Troubleshooting tips:
	- %s
	- %s
	- %s
	- %s
	`, containerID,
		color.YellowString("Check the container logs: docker logs %s", containerID),
		color.YellowString("Verify that the HEALTHCHECK instruction is correctly defined in your Dockerfile."),
		color.YellowString("Ensure that any dependencies needed by the container are available."),
		color.YellowString("Review the container configuration for any resource constraints."))
}
