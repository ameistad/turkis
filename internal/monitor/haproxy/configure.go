package haproxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ameistad/turkis/internal/monitor"
)

type BlueGreenConfig struct {
	// Required parameters
	AppName         string
	NewDeploymentID string
	NewIPAddress    string
	NewPort         string
	Domains         monitor.ContainerDomains

	// Optional parameters with defaults
	HealthCheckPath    string        // Path to check on the new container (default: "/")
	HealthCheckTimeout time.Duration // Timeout for health checks (default: 5s)
	MaxRetries         int           // Max number of health check attempts (default: 10)
	RetryInterval      time.Duration // Time between health check attempts (default: 2s)
	DrainTime          time.Duration // Time to wait before removing old backend (default: 10s)
}

// NewDefaultBlueGreenConfig creates a BlueGreenConfig with default values
func NewDefaultBlueGreenConfig(appName, deploymentID, ipAddress, port string, domains monitor.ContainerDomains) BlueGreenConfig {
	return BlueGreenConfig{
		AppName:            appName,
		NewDeploymentID:    deploymentID,
		NewIPAddress:       ipAddress,
		NewPort:            port,
		Domains:            domains,
		HealthCheckPath:    "/",
		HealthCheckTimeout: 5 * time.Second,
		MaxRetries:         10,
		RetryInterval:      2 * time.Second,
		DrainTime:          10 * time.Second,
	}
}

// ExecuteBlueGreen performs a blue-green deployment
func (c *Client) ExecuteBlueGreen(ctx context.Context, config BlueGreenConfig) error {
	if c.dryRun {
		log.Printf("[DRY RUN] Would execute blue-green deployment for app '%s' with new deployment ID '%s'",
			config.AppName, config.NewDeploymentID)
		return nil
	}

	// Step 1: Find the current deployment ID (if any)
	oldDeploymentID, err := c.getCurrentDeploymentID(config.AppName, config.Domains.Domains[0].Name)
	if err != nil {
		log.Printf("Note: No existing deployment found for %s or error finding it: %v", config.AppName, err)
		// Not finding an old deployment is not an error - might be first deployment
	}

	// Step 2: Configure the new backend but don't route traffic to it yet
	newBackendName := fmt.Sprintf("%s-%s", config.AppName, config.NewDeploymentID)
	log.Printf("Configuring new backend '%s'", newBackendName)

	backend := BackendConfig{
		Name:      newBackendName,
		IPAddress: config.NewIPAddress,
		Port:      config.NewPort,
	}

	if err := c.ConfigureBackend(backend); err != nil {
		return fmt.Errorf("failed to configure new backend: %w", err)
	}

	// Step 3: Perform health checks on the new backend
	if err := performHealthChecks(config); err != nil {
		log.Printf("Health checks failed for %s: %v", newBackendName, err)
		// Optionally remove the new backend since it's unhealthy
		if removeErr := c.RemoveBackend(config.AppName, config.NewDeploymentID); removeErr != nil {
			log.Printf("Warning: Failed to remove unhealthy backend: %v", removeErr)
		}
		return fmt.Errorf("health check failed for new deployment: %w", err)
	}

	log.Printf("Health checks passed for %s", newBackendName)

	// Step 4: Switch traffic to the new backend
	log.Printf("Switching traffic to new backend '%s'", newBackendName)

	// Update domain maps to point to the new backend
	for _, domain := range config.Domains.Domains {
		cmdAddMap := fmt.Sprintf("add map /etc/haproxy/maps/hosts.map %s %s",
			domain.Name, newBackendName)
		_, err := c.SendCommand(cmdAddMap)
		if err != nil {
			log.Printf("Error adding domain mapping for %s: %v", domain.Name, err)
			continue
		}
		log.Printf("Switched domain mapping: %s -> %s", domain.Name, newBackendName)

		// Update redirects as well
		for _, alias := range domain.Aliases {
			cmdAddAlias := fmt.Sprintf("add map /etc/haproxy/maps/redirects.map %s %s",
				alias, domain.Name)
			_, err := c.SendCommand(cmdAddAlias)
			if err != nil {
				log.Printf("Error adding alias redirect for %s: %v", alias, err)
				continue
			}
			log.Printf("Updated alias redirect: %s -> %s", alias, domain.Name)
		}
	}

	// Step 5: If there was an old backend, wait for drain time then remove it
	if oldDeploymentID != "" && oldDeploymentID != config.NewDeploymentID {
		oldBackendName := fmt.Sprintf("%s-%s", config.AppName, oldDeploymentID)

		// First set the old backend to maintenance mode to stop new connections
		cmdMaint := fmt.Sprintf("set server %s/server1 state maint", oldBackendName)
		if _, err := c.SendCommand(cmdMaint); err != nil {
			log.Printf("Warning: Failed to set old backend to maintenance mode: %v", err)
			// This is a warning, not a critical error
		} else {
			log.Printf("Set old backend '%s' to maintenance mode", oldBackendName)
		}

		// Wait for drain time
		log.Printf("Waiting %s for connections to drain from old backend '%s'",
			config.DrainTime, oldBackendName)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(config.DrainTime):
			// Continue to removal
		}

		// Now remove the old backend
		log.Printf("Removing old backend '%s'", oldBackendName)
		if err := c.RemoveBackend(config.AppName, oldDeploymentID); err != nil {
			log.Printf("Warning: Failed to remove old backend: %v", err)
			// Continue despite error - not critical
		}
	}

	log.Printf("Blue-green deployment completed successfully for app '%s' (new deployment: '%s')",
		config.AppName, config.NewDeploymentID)
	return nil
}

// getCurrentDeploymentID tries to find the current deployment ID for an app by checking the domain mappings
func (c *Client) getCurrentDeploymentID(appName, sampleDomain string) (string, error) {
	// Get the current backend for this domain
	cmdShowMap := fmt.Sprintf("show map /etc/haproxy/maps/hosts.map %s", sampleDomain)
	resp, err := c.SendCommand(cmdShowMap)
	if err != nil {
		return "", fmt.Errorf("failed to check current backend: %w", err)
	}

	// Extract backend name from response
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == sampleDomain {
			currentBackend := parts[1]
			// Backend name format is "appName-deploymentID"
			if strings.HasPrefix(currentBackend, appName+"-") {
				deploymentID := strings.TrimPrefix(currentBackend, appName+"-")
				return deploymentID, nil
			}
		}
	}

	return "", fmt.Errorf("no current deployment found for app '%s'", appName)
}

// performHealthChecks ensures the new container is ready to receive traffic
func performHealthChecks(config BlueGreenConfig) error {
	healthURL := fmt.Sprintf("http://%s:%s%s",
		config.NewIPAddress,
		config.NewPort,
		config.HealthCheckPath)

	client := &http.Client{
		Timeout: config.HealthCheckTimeout,
	}

	log.Printf("Performing health checks against %s", healthURL)

	for i := 0; i < config.MaxRetries; i++ {
		req, err := http.NewRequest("GET", healthURL, nil)
		if err != nil {
			log.Printf("Health check attempt %d: Error creating request: %v", i+1, err)
			time.Sleep(config.RetryInterval)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Health check attempt %d: Connection error: %v", i+1, err)
			time.Sleep(config.RetryInterval)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			log.Printf("Health check passed on attempt %d with status code %d", i+1, resp.StatusCode)
			return nil
		}

		log.Printf("Health check attempt %d: Received status code %d", i+1, resp.StatusCode)
		time.Sleep(config.RetryInterval)
	}

	return fmt.Errorf("health check failed after %d attempts", config.MaxRetries)
}

// BackendConfig represents the backend configuration for a container
type BackendConfig struct {
	Name      string
	IPAddress string
	Port      string
}

// ConfigureBackend sets up a backend server in HAProxy
func (c *Client) ConfigureBackend(backend BackendConfig) error {
	// In dry run mode, just log what we would do and simulate the response
	if c.dryRun {
		log.Printf("[DRY RUN] Would configure backend: %s with server at %s:%s",
			backend.Name, backend.IPAddress, backend.Port)

		// Early return for dry run mode
		return nil
	}

	// Check if backend exists, create if not
	cmdCheckBackend := fmt.Sprintf("show backend %s", backend.Name)
	resp, err := c.SendCommand(cmdCheckBackend)
	if err != nil || strings.Contains(resp, "No such backend") {
		cmdAddBackend := fmt.Sprintf("add backend %s", backend.Name)
		_, err = c.SendCommand(cmdAddBackend)
		if err != nil {
			return fmt.Errorf("failed to create backend %s: %w", backend.Name, err)
		}
		log.Printf("Created new HAProxy backend: %s", backend.Name)
	}

	// Configure server in the backend
	serverName := "server1" // Using a fixed name since we only have one server per backend
	serverAddr := fmt.Sprintf("%s:%s", backend.IPAddress, backend.Port)

	// Check if server exists first
	cmdCheckServer := fmt.Sprintf("show servers state %s", backend.Name)
	respServer, _ := c.SendCommand(cmdCheckServer)

	if !strings.Contains(respServer, serverName) {
		// Add server if it doesn't exist
		cmdAddServer := fmt.Sprintf("add server %s/%s %s",
			backend.Name, serverName, serverAddr)
		_, err = c.SendCommand(cmdAddServer)
		if err != nil {
			return fmt.Errorf("failed to add server to backend %s: %w", backend.Name, err)
		}
	} else {
		// Update server if it exists
		cmdSetAddr := fmt.Sprintf("set server %s/%s addr %s",
			backend.Name, serverName, serverAddr)
		_, err = c.SendCommand(cmdSetAddr)
		if err != nil {
			return fmt.Errorf("failed to update server address: %w", err)
		}
	}

	// Enable the server
	cmdEnableServer := fmt.Sprintf("set server %s/%s state ready",
		backend.Name, serverName)
	_, err = c.SendCommand(cmdEnableServer)
	if err != nil {
		return fmt.Errorf("failed to enable server: %w", err)
	}

	log.Printf("Configured backend %s with server at %s", backend.Name, serverAddr)
	return nil
}

// ConfigureFromDomains sets up HAProxy configuration for container domains
func (c *Client) ConfigureFromDomains(domains monitor.ContainerDomains, ipAddress, port string) error {
	if len(domains.Domains) == 0 {
		return fmt.Errorf("no domains to configure")
	}

	// Generate a unique backend name based on app and deployment ID
	backendName := domains.AppName
	if domains.DeploymentID != "" {
		backendName = fmt.Sprintf("%s-%s", domains.AppName, domains.DeploymentID)
	}

	// Print summary in dry run mode
	if c.dryRun {
		log.Printf("[DRY RUN] Would configure backend '%s' at %s:%s with the following domains:",
			backendName, ipAddress, port)
		for _, domain := range domains.Domains {
			log.Printf("[DRY RUN]   - Domain: %s", domain.Name)
			for _, alias := range domain.Aliases {
				log.Printf("[DRY RUN]     - Alias: %s -> %s", alias, domain.Name)
			}
		}

		// In dry-run mode, just simulate the configuration commands
		log.Printf("[DRY RUN] Would execute:")
		log.Printf("[DRY RUN]   add backend %s", backendName)
		log.Printf("[DRY RUN]   add server %s/server1 %s:%s", backendName, ipAddress, port)
		log.Printf("[DRY RUN]   set server %s/server1 state ready", backendName)

		for _, domain := range domains.Domains {
			log.Printf("[DRY RUN]   add map /etc/haproxy/maps/hosts.map %s %s", domain.Name, backendName)
			for _, alias := range domain.Aliases {
				log.Printf("[DRY RUN]   add map /etc/haproxy/maps/redirects.map %s %s", alias, domain.Name)
			}
		}

		return nil
	}

	// Configure the backend with the container's IP and port
	backend := BackendConfig{
		Name:      backendName,
		IPAddress: ipAddress,
		Port:      port,
	}

	if err := c.ConfigureBackend(backend); err != nil {
		return err
	}

	// Configure frontend mapping for each domain
	for _, domain := range domains.Domains {
		// Add main domain to frontend mapping
		cmdAddMap := fmt.Sprintf("add map /etc/haproxy/maps/hosts.map %s %s",
			domain.Name, backendName)
		_, err := c.SendCommand(cmdAddMap)
		if err != nil {
			log.Printf("Error adding domain mapping for %s: %v", domain.Name, err)
			continue
		}
		log.Printf("Added domain mapping: %s -> %s", domain.Name, backendName)

		// Add each alias with a redirect rule
		for _, alias := range domain.Aliases {
			// For aliases, we'll add them to a separate redirect map
			cmdAddAlias := fmt.Sprintf("add map /etc/haproxy/maps/redirects.map %s %s",
				alias, domain.Name)
			_, err := c.SendCommand(cmdAddAlias)
			if err != nil {
				log.Printf("Error adding alias redirect for %s: %v", alias, err)
				continue
			}
			log.Printf("Added alias redirect: %s -> %s", alias, domain.Name)
		}
	}

	return nil
}

// RemoveBackend removes a backend and its associated frontend rules
func (c *Client) RemoveBackend(appName, deploymentID string) error {
	backendName := appName
	if deploymentID != "" {
		backendName = fmt.Sprintf("%s-%s", appName, deploymentID)
	}

	if c.dryRun {
		log.Printf("[DRY RUN] Would remove backend '%s' and its associated mappings", backendName)
		log.Printf("[DRY RUN] Would execute:")
		log.Printf("[DRY RUN]   show map /etc/haproxy/maps/hosts.map")
		log.Printf("[DRY RUN]   del map /etc/haproxy/maps/hosts.map [associated domains]")
		log.Printf("[DRY RUN]   del backend %s", backendName)
		return nil
	}

	// First try to identify all associated domains
	cmdShowMap := "show map /etc/haproxy/maps/hosts.map"
	mapContent, err := c.SendCommand(cmdShowMap)
	if err != nil {
		return fmt.Errorf("could not fetch domain mappings: %w", err)
	}

	// Parse the map content to find domains associated with this backend
	for _, line := range strings.Split(mapContent, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == backendName {
			domain := parts[0]
			// Remove the domain mapping
			cmdDelMap := fmt.Sprintf("del map /etc/haproxy/maps/hosts.map %s", domain)
			if _, err := c.SendCommand(cmdDelMap); err != nil {
				log.Printf("Failed to remove domain mapping for %s: %v", domain, err)
			} else {
				log.Printf("Removed domain mapping for %s", domain)
			}
		}
	}

	// Now delete the backend
	cmdDelBackend := fmt.Sprintf("del backend %s", backendName)
	_, err = c.SendCommand(cmdDelBackend)
	if err != nil {
		return fmt.Errorf("failed to delete backend %s: %w", backendName, err)
	}

	log.Printf("Removed backend %s", backendName)
	return nil
}
