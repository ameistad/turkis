package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ameistad/turkis/internal/config"
)

// TODO: use golang docker client library instead of exec.Command.

// DeployApp builds the Docker image, runs a new container (with volumes), checks its health,
// stops any old containers, and prunes extras.
func DeployApp(appConfig *config.AppConfig) error {

	imageName := appConfig.Name + ":latest"

	// Build the new image.
	if err := buildImage(appConfig.Dockerfile, appConfig.BuildContext, imageName, appConfig.Env); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	// Run a new container and obtain its ID and deployment ID.
	containerID, deploymentID, err := runContainer(imageName, appConfig)
	if err != nil {
		return fmt.Errorf("failed to run new container: %w", err)
	}

	fmt.Printf("Performing health check on container %s...\n", containerID)
	if err := HealthCheckContainer(containerID, appConfig.HealthCheckPath); err != nil {
		return fmt.Errorf("new container failed health check: %w", err)
	}

	// Stop any old containers so that the reverse proxy routes traffic only to the new container.
	if err := StopOldContainers(appConfig.Name, containerID, deploymentID); err != nil {
		return fmt.Errorf("failed to stop old containers: %w", err)
	}

	// Prune old containers based on configuration.
	if err := PruneOldContainers(appConfig.Name, containerID, appConfig.KeepOldContainers); err != nil {
		return fmt.Errorf("failed to prune old containers: %w", err)
	}

	// Clean up old dangling images
	if err := PruneOldImages(appConfig.Name); err != nil {
		fmt.Printf("Warning: failed to prune old images: %v\n", err)
		// We don't return the error here as this is a non-critical step
	}

	fmt.Printf("Successfully deployed app '%s'. New deployment ID: %s\n", appConfig.Name, deploymentID)
	return nil
}

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

func runContainer(imageName string, appConfig *config.AppConfig) (string, string, error) {
	deploymentID := time.Now().Format("20060102150405")
	containerName := fmt.Sprintf("%s-turkis-%s", appConfig.Name, deploymentID)

	args := []string{"run", "-d", "--name", containerName, "--restart", "unless-stopped"}

	// Add all labels at once by merging maps
	labels := make(map[string]string)

	// Add identification labels
	labels["turkis.appName"] = appConfig.Name
	labels["turkis.deployment"] = deploymentID

	// Add health check path if specified
	if appConfig.HealthCheckPath != "" && appConfig.HealthCheckPath != "/" {
		labels["turkis.health-check-path"] = appConfig.HealthCheckPath
	}

	// Add drain time (default 10s)
	labels["turkis.drain-time"] = "10s"
	labels["turkis.acme.email"] = appConfig.ACMEEmail

	// Add domains and their aliases
	addDomainLabels(labels, appConfig.Domains)

	// Convert all labels to docker command arguments
	for k, v := range labels {
		args = append(args, "-l", fmt.Sprintf("%s=%s", k, v))
	}

	// Add environment variables.
	for k, v := range appConfig.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add volumes.
	for _, vol := range appConfig.Volumes {
		args = append(args, "-v", vol)
	}

	// Ensure the network exists before attaching the container
	ensureNetworkCmd := exec.Command("docker", "network", "inspect", config.DockerNetwork)
	if err := ensureNetworkCmd.Run(); err != nil {
		// Network doesn't exist, create it
		fmt.Printf("Network %s doesn't exist. Creating it...\n", config.DockerNetwork)
		createNetworkCmd := exec.Command("docker", "network", "create", config.DockerNetwork)
		if err := createNetworkCmd.Run(); err != nil {
			return "", "", fmt.Errorf("failed to create network %s: %w", config.DockerNetwork, err)
		}
	}

	// Attach the container to the network.
	args = append(args, "--network", config.DockerNetwork)

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

// addDomainLabels adds domain-related labels to the provided labels map
func addDomainLabels(labels map[string]string, domains []config.Domain) {
	// Keep track of all domains for the container
	var allDomains []string

	// Process each domain configuration
	for i, domain := range domains {
		// Add the canonical domain
		domainValue := domain.Domain
		allDomains = append(allDomains, domainValue)
		labels[fmt.Sprintf("turkis.domain.%d", i)] = domainValue

		// Add aliases that should redirect to this canonical domain
		if len(domain.Aliases) > 0 {
			labels[fmt.Sprintf("turkis.domain.%d.canonical", i)] = domainValue
			for j, alias := range domain.Aliases {
				aliasKey := fmt.Sprintf("turkis.domain.%d.alias.%d", i, j)
				labels[aliasKey] = alias
				allDomains = append(allDomains, alias)
			}
		}
	}

	// Add a comma-separated list of all domains for easy access
	if len(allDomains) > 0 {
		labels["turkis.domains.all"] = strings.Join(allDomains, ",")
	}
}
