package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ameistad/turkis/config"
	"github.com/spf13/cobra"
)

// deployCmd represents the "deploy" command.
var deployCmd = &cobra.Command{
	Use:   "deploy [appName]",
	Short: "Deploy a specific app defined in the YAML config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appName := args[0]
		confFilePath, err := config.DefaultConfigFilePath()
		if err != nil {
			return err
		}
		conf, err := config.LoadConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("failed to load config from '%s': %w", confFilePath, err)
		}
		if err := config.ValidateConfigFile(conf); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		var appCfg *config.AppConfig
		for i := range conf.Apps {
			if conf.Apps[i].Name == appName {
				appCfg = &conf.Apps[i]
				break
			}
		}
		if appCfg == nil {
			return fmt.Errorf("app '%s' not found in config", appName)
		}

		return deployApp(appCfg)
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

// deployApp builds the Docker image, runs a new container, checks its health,
// and stops any old containers so that Traefik routes traffic only to the new one.
func deployApp(appCfg *config.AppConfig) error {
	imageName := appCfg.Name + ":latest"

	// Build the new image.
	if err := buildImage(appCfg.Dockerfile, appCfg.BuildContext, imageName, appCfg.Env); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	// Run a new container and obtain its ID and deployment ID.
	containerID, deploymentID, err := runContainer(imageName, appCfg.Env, appCfg.Domains, appCfg.Name)
	if err != nil {
		return fmt.Errorf("failed to run new container: %w", err)
	}

	fmt.Printf("Performing health check on container %s...\n", containerID)
	// Ensure the container is healthy.
	if err := healthCheckContainer(containerID); err != nil {
		return fmt.Errorf("new container failed health check: %w", err)
	}

	// Stop any old containers so that Traefik routes traffic only to the new container.
	if err := stopOldContainers(appCfg.Name, containerID); err != nil {
		return fmt.Errorf("failed to stop old containers: %w", err)
	}

	fmt.Printf("Successfully deployed app '%s'. New deployment ID: %s\n", appCfg.Name, deploymentID)
	return nil
}

// buildImage builds a Docker image using the specified Dockerfile, build context,
// and environment variables (passed as build arguments).
func buildImage(dockerfile, buildContext, imageName string, buildArgs map[string]string) error {
	args := []string{"build", "-t", imageName, "-f", dockerfile}
	for k, v := range buildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, buildContext)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("Building image '%s'...\n", imageName)
	return cmd.Run()
}

// runContainer starts a new container from the specified image using the new domains configuration.
// It configures the canonical router (via traefikLabels) and, for each alias, attaches extra labels
// to set up a dedicated TLS-enabled router with a redirect middleware.
// The redirect middleware uses a regex to catch all paths and issues a permanent redirect to the canonical domain.
func runContainer(imageName string, env map[string]string, domains []config.Domain, appName string) (string, string, error) {
	deploymentID := time.Now().Format("20060102150405")
	containerName := fmt.Sprintf("%s-turkis-%s", appName, deploymentID)
	args := []string{"run", "-d", "--name", containerName}

	// Aggregate canonical domains from each Domain entry.
	canonicalHosts := []string{}

	// Build a map for additional alias router labels.
	aliasLabels := make(map[string]string)

	// Iterate over all domains configured for the app.
	for _, d := range domains {
		// Add the canonical domain for the main router.
		canonicalHosts = append(canonicalHosts, d.Domain)

		// For every alias, create a dedicated router with redirect middleware.
		for key, value := range generateAliasLabels(appName, d) {
			aliasLabels[key] = value
		}
	}

	// Generate labels for the canonical router using the aggregated canonical domains.
	canonicalLabels := traefikLabels(imageName, canonicalHosts, 80)
	for k, v := range canonicalLabels {
		args = append(args, "-l", fmt.Sprintf("%s=%s", k, v))
	}

	// Append the alias router labels.
	for k, v := range aliasLabels {
		args = append(args, "-l", fmt.Sprintf("%s=%s", k, v))
	}

	// Append custom labels to identify the app and deployment.
	args = append(args, "-l", fmt.Sprintf("turkis.app=%s", appName))
	args = append(args, "-l", fmt.Sprintf("turkis.deployment=%s", deploymentID))

	// Add environment variables.
	for k, v := range env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Attach the container to the traefik-public network.
	args = append(args, "--network", "traefik-public")

	// Finally, set the image to run.
	args = append(args, imageName)

	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	containerID := strings.TrimSpace(string(out))
	fmt.Printf("New container started with ID '%s' and name '%s'\n", containerID, containerName)
	return containerID, deploymentID, nil
}

// sanitize replaces characters that are unsuitable for Traefik label keys.
// For example, it replaces dots with underscores.
func sanitize(s string) string {
	return strings.ReplaceAll(s, ".", "_")
}

// healthCheckContainer continuously inspects the container until its health status is "healthy",
// or if no HEALTHCHECK is defined, assumes the container is healthy.
func healthCheckContainer(containerID string) error {
	timeout := 60 * time.Second
	interval := 2 * time.Second
	deadline := time.Now().Add(timeout)

	type Health struct {
		Status string `json:"Status"`
	}

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
		fmt.Printf("Container %s health status: %s\n", containerID, health.Status)
		if health.Status == "healthy" {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("health check timeout for container %s", containerID)
}

// stopOldContainers stops any running container for the app (identified by the "turkis.app" label),
// except for the container with the given container ID. It uses a prefix match to handle Docker's shortened IDs.
func stopOldContainers(appName, newContainerID string) error {
	out, err := exec.Command("docker", "ps", "--filter", fmt.Sprintf("label=turkis.app=%s", appName), "--format", "{{.ID}}").Output()
	if err != nil {
		return err
	}
	containers := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, id := range containers {
		// Skip if the container ID is empty or matches the new container.
		if id == "" || strings.HasPrefix(newContainerID, id) {
			continue
		}
		fmt.Printf("Stopping old container: %s\n", id)
		if err := exec.Command("docker", "stop", id).Run(); err != nil {
			fmt.Printf("Error stopping container %s: %v\n", id, err)
		}
	}
	return nil
}

// traefikLabels generates and returns a map of labels for Traefik routing.
// It constructs a host rule using the provided hosts and sets the load balancer port.
func traefikLabels(serviceName string, hosts []string, containerPort int) map[string]string {
	hostRules := make([]string, len(hosts))
	for i, host := range hosts {
		hostRules[i] = fmt.Sprintf("Host(`%s`)", host)
	}
	rule := strings.Join(hostRules, " || ")

	return map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", serviceName):                      rule,
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", serviceName):          "letsencrypt",
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", serviceName):               "websecure",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName): fmt.Sprintf("%d", containerPort),
	}
}

func generateAliasLabels(appName string, d config.Domain) map[string]string {
	labels := make(map[string]string)
	for _, alias := range d.Aliases {
		aliasKey := sanitize(alias)
		routerName := fmt.Sprintf("%s-redirect-%s", appName, aliasKey)

		labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", alias)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", routerName)] = "noop@internal"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", routerName)] = "true"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = "letsencrypt"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", routerName)] = routerName
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectRegex.regex", routerName)] = "^(https?://)?[^/]+(.*)$"
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectRegex.replacement", routerName)] = fmt.Sprintf("https://%s$2", d.Domain)
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectRegex.permanent", routerName)] = "true"
	}
	return labels
}
