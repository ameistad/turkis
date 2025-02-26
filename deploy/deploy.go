package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/ameistad/turkis/config"
)

// DeployApp builds the Docker image, runs a new container (with volumes), checks its health,
// stops any old containers, and prunes extras.
func DeployApp(appConfig *config.AppConfig) error {
	imageName := appConfig.Name + ":latest"

	// Build the new image.
	if err := buildImage(appConfig.Dockerfile, appConfig.BuildContext, imageName, appConfig.Env); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	// Run a new container and obtain its ID and deployment ID.
	containerID, deploymentID, err := runContainer(imageName, appConfig.Env, appConfig.Volumes, appConfig.Domains, appConfig.Name)
	if err != nil {
		return fmt.Errorf("failed to run new container: %w", err)
	}

	fmt.Printf("Performing health check on container %s...\n", containerID)
	if err := HealthCheckContainer(containerID); err != nil {
		return fmt.Errorf("new container failed health check: %w", err)
	}

	// Stop any old containers so that Traefik routes traffic only to the new container.
	if err := stopOldContainers(appConfig.Name, containerID, deploymentID); err != nil {
		return fmt.Errorf("failed to stop old containers: %w", err)
	}

	// Prune old containers based on configuration.
	if err := pruneOldContainers(appConfig.Name, containerID, appConfig.KeepOldContainers); err != nil {
		return fmt.Errorf("failed to prune old containers: %w", err)
	}

	// Clean up old dangling images
	if err := pruneOldImages(appConfig.Name); err != nil {
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

func runContainer(imageName string, env map[string]string, volumes []string, domains []config.Domain, appName string) (string, string, error) {
	deploymentID := time.Now().Format("20060102150405")
	containerName := fmt.Sprintf("%s-turkis-%s", appName, deploymentID)

	args := []string{"run", "-d", "--name", containerName, "--restart", "unless-stopped"}

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

	// Add volumes.
	for _, vol := range volumes {
		args = append(args, "-v", vol)
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

func sanitize(s string) string {
	return strings.ReplaceAll(s, ".", "_")
}

func stopOldContainers(appName, newContainerID, newDeploymentID string) error {
	out, err := exec.Command("docker", "ps", "--filter", fmt.Sprintf("label=turkis.app=%s", appName), "--format", "{{.ID}}").Output()
	if err != nil {
		return err
	}
	containers := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, id := range containers {
		// Skip if the container ID is empty or matches the new container.
		if id == "" || strings.HasPrefix(newContainerID, id) || strings.HasPrefix(id, newContainerID) {
			continue
		}

		// Inspect the container's deployment label.
		labelOut, err := exec.Command("docker", "inspect", "--format", "{{ index .Config.Labels \"turkis.deployment\" }}", id).Output()
		if err != nil {
			fmt.Printf("Error reading deployment label for container %s: %v. Skipping container...\n", id, err)
			continue
		}
		containerDeploymentID := strings.TrimSpace(string(labelOut))
		if containerDeploymentID != newDeploymentID {
			fmt.Printf("Stopping old container: %s (deployment: %s)\n", id, containerDeploymentID)
			if err := exec.Command("docker", "stop", id).Run(); err != nil {
				fmt.Printf("Error stopping container %s: %v\n", id, err)
			}
		}
	}
	return nil
}

func traefikLabels(serviceName string, hosts []string, containerPort int) map[string]string {
	// Build a Traefik "rule" matching all domains:
	// e.g. "Host(`domain1.com`) || Host(`domain2.com`)"
	hostRules := make([]string, len(hosts))
	for i, host := range hosts {
		hostRules[i] = fmt.Sprintf("Host(`%s`)", host)
	}
	rule := strings.Join(hostRules, " || ")

	// We'll create two routers:
	// 1) "serviceName-http" router on entrypoint "web" --> redirect to HTTPS
	// 2) "serviceName-https" router on entrypoint "websecure" --> serve the app
	httpRouterName := serviceName + "-http"
	httpsRouterName := serviceName + "-https"

	labels := map[string]string{
		// Enable Traefik for this container
		"traefik.enable": "true",

		// 1) HTTP Router: match domains on port 80 => redirect to HTTPS
		fmt.Sprintf("traefik.http.routers.%s.rule", httpRouterName):        rule,
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpRouterName): "web",
		// Attach a redirect middleware to the HTTP router
		fmt.Sprintf("traefik.http.routers.%s.middlewares", httpRouterName): fmt.Sprintf("%s-redirect", httpRouterName),
		// Redirect middleware config
		fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectScheme.scheme", httpRouterName):    "https",
		fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectScheme.permanent", httpRouterName): "true",

		// 2) HTTPS Router: match domains on port 443 => serve the app
		fmt.Sprintf("traefik.http.routers.%s.rule", httpsRouterName):             rule,
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpsRouterName):      "websecure",
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", httpsRouterName): "letsencrypt",
		fmt.Sprintf("traefik.http.routers.%s.tls", httpsRouterName):              "true",
		// Assign a service/port for your actual container
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName): fmt.Sprintf("%d", containerPort),
	}

	return labels
}

func generateAliasLabels(appName string, d config.Domain) map[string]string {
	labels := make(map[string]string)
	for _, alias := range d.Aliases {
		aliasKey := sanitize(alias)

		// HTTP router - redirects http://alias.com to https://www.domain.com directly
		httpRouterName := fmt.Sprintf("%s-http-%s", appName, aliasKey)
		middlewareName := fmt.Sprintf("%s-redirect", httpRouterName)

		labels[fmt.Sprintf("traefik.http.routers.%s.rule", httpRouterName)] = fmt.Sprintf("Host(`%s`)", alias)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpRouterName)] = "web"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", httpRouterName)] = "noop@internal"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", httpRouterName)] = middlewareName
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.regex", middlewareName)] = "^(.*)$"
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.replacement", middlewareName)] = fmt.Sprintf("https://%s$1", d.Domain)
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent", middlewareName)] = "true"

		// HTTPS router - redirects https://alias.com to https://www.domain.com
		httpsRouterName := fmt.Sprintf("%s-https-%s", appName, aliasKey)
		httpsMiddlewareName := fmt.Sprintf("%s-redirect", httpsRouterName)

		labels[fmt.Sprintf("traefik.http.routers.%s.rule", httpsRouterName)] = fmt.Sprintf("Host(`%s`)", alias)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpsRouterName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", httpsRouterName)] = "noop@internal"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", httpsRouterName)] = "true"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", httpsRouterName)] = "letsencrypt"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", httpsRouterName)] = httpsMiddlewareName
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.regex", httpsMiddlewareName)] = "^(.*)$"
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.replacement", httpsMiddlewareName)] = fmt.Sprintf("https://%s$1", d.Domain)
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent", httpsMiddlewareName)] = "true"
	}
	return labels
}

func pruneOldContainers(appName, newContainerID string, keepCount int) error {
	out, err := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("label=turkis.app=%s", appName), "--format", "{{.ID}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w - output: %s", err, string(out))
	}

	ids := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == "") {
		return nil
	}

	var containers []ContainerInfo
	for _, id := range ids {
		if id == "" {
			continue
		}
		labelOut, err := exec.Command("docker", "inspect", "--format", "{{ index .Config.Labels \"turkis.deployment\" }}", id).CombinedOutput()
		if err != nil {
			fmt.Printf("Error inspecting container %s for deployment label: %v\n", id, err)
			continue
		}

		depID := strings.TrimSpace(string(labelOut))
		// Validate deployment ID format (should be a timestamp like 20060102150405)
		if len(depID) != 14 || !isNumeric(depID) {
			fmt.Printf("Warning: Container %s has invalid deployment ID format: %s\n", id, depID)
		}

		containers = append(containers, ContainerInfo{ID: id, DeploymentID: depID})
	}

	var oldContainers []ContainerInfo
	for _, c := range containers {
		if c.ID == newContainerID {
			continue
		}
		oldContainers = append(oldContainers, c)
	}

	// Sort by deployment ID (newer ones first)
	sort.Slice(oldContainers, func(i, j int) bool {
		return oldContainers[i].DeploymentID > oldContainers[j].DeploymentID
	})

	if len(oldContainers) <= keepCount {
		fmt.Println("No extra containers to prune.")
		return nil
	}

	for _, c := range oldContainers[keepCount:] {
		fmt.Printf("Pruning container %s (deployment: %s)\n", c.ID, c.DeploymentID)
		out, err := exec.Command("docker", "rm", c.ID).CombinedOutput()
		if err != nil {
			fmt.Printf("Error pruning container %s: %v, details: %s\n", c.ID, err, string(out))
		}
	}
	return nil
}

// Helper function to check if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func pruneOldImages(appName string) error {
	fmt.Println("Pruning dangling images...")

	// First, remove unused images related to this app
	listCmd := exec.Command("docker", "images", "--filter", fmt.Sprintf("reference=%s", appName), "--format", "{{.ID}}")
	output, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error listing images for %s: %w (%s)", appName, err, string(output))
	}

	imageIDs := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, id := range imageIDs {
		if id == "" {
			continue
		}

		// Check if the image is not being used
		inspectCmd := exec.Command("docker", "inspect", "--format", "{{.RepoTags}}", id)
		inspectOut, err := inspectCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: could not inspect image %s: %v\n", id, err)
			continue
		}

		// Skip the latest tag
		if strings.Contains(string(inspectOut), fmt.Sprintf("%s:latest", appName)) {
			continue
		}

		fmt.Printf("Removing old image: %s\n", id)
		removeCmd := exec.Command("docker", "rmi", id)
		removeOut, err := removeCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: could not remove image %s: %v (%s)\n", id, err, string(removeOut))
		}
	}

	// Then, prune dangling images (no tag) system-wide
	pruneCmd := exec.Command("docker", "image", "prune", "--force")
	pruneCmd.Stdout = os.Stdout
	pruneCmd.Stderr = os.Stderr
	if err := pruneCmd.Run(); err != nil {
		return fmt.Errorf("error pruning dangling images: %w", err)
	}

	return nil
}
