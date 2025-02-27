package deploy

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
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

func runContainer(imageName string, env map[string]string, volumes []string, domains []config.Domain, appName string) (string, string, error) {
	deploymentID := time.Now().Format("20060102150405")
	containerName := fmt.Sprintf("%s-turkis-%s", appName, deploymentID)

	args := []string{"run", "-d", "--name", containerName, "--restart", "unless-stopped"}

	// Add all Traefik labels at once by merging maps
	labels := make(map[string]string)
	// 1. Add canonical domain labels
	maps.Copy(labels, traefikLabels(appName, domains, 80))

	// 2. Add alias labels for all domains
	for _, domain := range domains {
		maps.Copy(labels, generateAliasLabels(appName, domain))
	}

	// 3. Add identification labels
	labels["turkis.app"] = appName
	labels["turkis.deployment"] = deploymentID

	// Convert all labels to docker command arguments
	for k, v := range labels {
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

func traefikLabels(serviceName string, domains []config.Domain, containerPort int) map[string]string {
	// Sanitize service name for labels (remove colons)
	sanitizedServiceName := strings.ReplaceAll(serviceName, ":", "-")

	// Build host rules
	hostRules := make([]string, 0)
	for _, domain := range domains {
		hostRules = append(hostRules, fmt.Sprintf("Host(`%s`)", domain.Domain))
	}
	rule := strings.Join(hostRules, " || ")

	// Basic labels with both HTTP and HTTPS routers
	labels := map[string]string{
		"traefik.enable": "true",

		// HTTP router - redirects to HTTPS
		fmt.Sprintf("traefik.http.routers.%s-http.rule", sanitizedServiceName):        rule,
		fmt.Sprintf("traefik.http.routers.%s-http.entrypoints", sanitizedServiceName): "web",
		fmt.Sprintf("traefik.http.routers.%s-http.middlewares", sanitizedServiceName): fmt.Sprintf("%s-redirect-https", sanitizedServiceName),

		// HTTP to HTTPS redirect middleware
		fmt.Sprintf("traefik.http.middlewares.%s-redirect-https.redirectscheme.scheme", sanitizedServiceName):    "https",
		fmt.Sprintf("traefik.http.middlewares.%s-redirect-https.redirectscheme.permanent", sanitizedServiceName): "true",

		// HTTPS router - serves the actual content
		fmt.Sprintf("traefik.http.routers.%s.rule", sanitizedServiceName):             rule,
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", sanitizedServiceName):      "websecure",
		fmt.Sprintf("traefik.http.routers.%s.tls", sanitizedServiceName):              "true",
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", sanitizedServiceName): "letsencrypt",

		// Service configuration
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", sanitizedServiceName): fmt.Sprintf("%d", containerPort),
	}

	return labels
}

func generateAliasLabels(appName string, d config.Domain) map[string]string {
	labels := make(map[string]string)

	// Skip if no aliases defined
	if len(d.Aliases) == 0 {
		return labels
	}

	for _, alias := range d.Aliases {
		aliasKey := sanitize(alias)

		// HTTP router - redirects http://alias.com directly to https://canonical.com
		httpRouterName := fmt.Sprintf("%s-http-alias-%s", appName, aliasKey)
		httpMiddlewareName := fmt.Sprintf("%s-redirect", httpRouterName)

		labels[fmt.Sprintf("traefik.http.routers.%s.rule", httpRouterName)] = fmt.Sprintf("Host(`%s`)", alias)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpRouterName)] = "web"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", httpRouterName)] = "noop@internal"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", httpRouterName)] = httpMiddlewareName
		labels[fmt.Sprintf("traefik.http.routers.%s.priority", httpRouterName)] = "100"

		// Direct HTTP → HTTPS canonical domain middleware
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.regex", httpMiddlewareName)] = "^http://[^/]+(.*)"
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.replacement", httpMiddlewareName)] = fmt.Sprintf("https://%s$1", d.Domain)
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent", httpMiddlewareName)] = "true"

		// HTTPS router - redirects https://alias.com to https://canonical.com
		httpsRouterName := fmt.Sprintf("%s-alias-%s", appName, aliasKey)
		httpsMiddlewareName := fmt.Sprintf("%s-redirect", httpsRouterName)

		// Router configuration
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", httpsRouterName)] = fmt.Sprintf("Host(`%s`)", alias)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpsRouterName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", httpsRouterName)] = "noop@internal"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", httpsRouterName)] = "true"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", httpsRouterName)] = "letsencrypt"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", httpsRouterName)] = httpsMiddlewareName

		// Middleware configuration for HTTPS redirect
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.regex", httpsMiddlewareName)] = "^https://[^/]+(.*)"
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.replacement", httpsMiddlewareName)] = fmt.Sprintf("https://%s$1", d.Domain)
		labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent", httpsMiddlewareName)] = "true"
	}

	return labels
}
