package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ameistad/turkis/internal/config"
	"github.com/ameistad/turkis/internal/monitor"
	"github.com/ameistad/turkis/internal/monitor/certificates"
	"github.com/ameistad/turkis/internal/monitor/haproxy"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

const (
	// HAProxySocketPath is the path to the HAProxy admin socket
	HAProxySocketPath = "/var/run/haproxy/admin.sock"
	// HAProxyTCPSocket is the TCP socket for HAProxy admin commands
	HAProxyTCPSocket = "127.0.0.1:9999"
	// LabelPrefix is the prefix for Turkis-specific labels
	LabelPrefix = "turkis"
	// RefreshInterval is how often to refresh the full configuration
	RefreshInterval = 5 * time.Minute
	// CertificatesDir is the directory where certificates are stored
	CertificatesDir = "/usr/local/etc/haproxy/certs"
	// WebRootDir is the directory for ACME HTTP-01 challenges
	WebRootDir = "/var/www/lego"
	// CertRefreshInterval is how often to check for certificate renewals
	CertRefreshInterval = 12 * time.Hour
)

var (
	// Track the latest known deployment ID for each app
	deploymentRegistry     = make(map[string]string) // app name -> deployment ID
	deploymentRegistryLock sync.RWMutex

	// Logger for certificate manager
	logger = logrus.New()
)

func main() {
	// Parse command line flags
	dryRunFlag := flag.Bool("dry-run", false, "Run in dry-run mode (don't actually send commands to HAProxy)")
	noTLSFlag := flag.Bool("no-tls", false, "Disable TLS certificate management")
	tlsStagingFlag := flag.Bool("tls-staging", false, "Use Let's Encrypt staging environment")
	flag.Parse()

	// Configure logger
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Check both flag and environment variables
	dryRunEnv := os.Getenv("DRY_RUN") == "true"
	dryRun := *dryRunFlag || dryRunEnv

	noTLSEnv := os.Getenv("NO_TLS") == "true"
	noTLS := *noTLSFlag || noTLSEnv

	tlsStagingEnv := os.Getenv("TLS_STAGING") == "true"
	tlsStaging := *tlsStagingFlag || tlsStagingEnv

	if dryRun {
		fmt.Println("========================")
		fmt.Println("STARTING IN DRY RUN MODE")
		fmt.Println("No changes will be made to HAProxy configuration")
		fmt.Println("========================")
	}

	if noTLS {
		fmt.Println("========================")
		fmt.Println("TLS CERTIFICATE MANAGEMENT DISABLED")
		fmt.Println("No certificates will be requested or renewed")
		fmt.Println("========================")
	}

	if tlsStaging {
		fmt.Println("========================")
		fmt.Println("TLS CERTIFICATE STAGING ENABLED")
		fmt.Println("Using Let's Encrypt staging environment")
		fmt.Println("========================")
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	var haproxyClient *haproxy.Client
	if dryRun {
		// In dry run mode, use an empty string for socket path since it won't be used
		haproxyClient = haproxy.NewDryRunClient("")
	} else {
		haproxyClient = haproxy.NewClient(HAProxySocketPath)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel for Docker events
	eventsChan := make(chan events.Message)
	errorsChan := make(chan error)

	// Initialize domain provider for certificate manager
	domainProvider := monitor.NewDomainProvider()

	// Initialize certificate manager if TLS is enabled
	var certManager *certificates.Manager
	var domainWatcher *certificates.DomainWatcher

	if !noTLS {
		// Certificate manager will be initialized when we get an email address from container labels
		certManager = nil
	}

	// Start Docker event listener
	go listenForDockerEvents(ctx, dockerClient, eventsChan, errorsChan)

	// Start periodic full refresh
	refreshTicker := time.NewTicker(RefreshInterval)
	defer refreshTicker.Stop()

	// Start certificate refresh ticker if TLS is enabled
	var certRefreshTicker *time.Ticker
	if !noTLS {
		certRefreshTicker = time.NewTicker(CertRefreshInterval)
		defer certRefreshTicker.Stop()
	}

	fmt.Printf("Monitor service started on network %s...\n", config.DockerNetwork)

	// Main event loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down gracefully...")
			// Stop certificate manager
			if certManager != nil {
				certManager.Stop()
			}
			cancel()
			return
		case event := <-eventsChan:
			switch event.Action {
			case "start":
				log.Printf("Container %s event: %s", event.Action, event.Actor.ID[:12])
				// Get container details
				container, err := dockerClient.ContainerInspect(ctx, event.Actor.ID)
				if err != nil {
					log.Printf("failed to inspect container: %v", err)
					continue
				}

				// Get network info
				ipAddress, _, err := monitor.ContainerNetworkInfo(container, config.DockerNetwork)
				if err != nil {
					log.Printf("Error getting container network info: %v", err)
					continue
				}

				// Get port info
				primaryPort, err := monitor.PrimaryPort(container)
				if err != nil {
					log.Printf("Error getting container primary port: %v", err)
					continue
				}

				// Parse domain config
				domains, err := monitor.ParseContainerDomains(container.Config.Labels)
				if err != nil {
					log.Printf("Error parsing container domains: %v", err)
					continue
				}

				// Check if this is a valid container with domains configured
				if len(domains.Domains) == 0 {
					log.Printf("Container %s has no domains configured, skipping", container.ID[:12])
					continue
				}

				// Check for email label and initialize certificate manager if needed
				if !noTLS {
					email, hasEmail := container.Config.Labels["turkis.acme.email"]
					if hasEmail && email != "" && certManager == nil {
						// Initialize certificate manager with email from container label
						certConfig := certificates.Config{
							Email:         email,
							CertDir:       CertificatesDir,
							WebRootDir:    WebRootDir,
							HAProxySocket: HAProxySocketPath,
							Logger:        logger,
							TlsStaging:    tlsStaging,
						}

						var err error
						certManager, err = certificates.NewManager(certConfig)
						if err != nil {
							logger.Fatalf("Failed to initialize certificate manager: %v", err)
						}

						// Start certificate manager
						if err := certManager.Start(); err != nil {
							logger.Fatalf("Failed to start certificate manager: %v", err)
						}

						// Initialize domain watcher
						domainWatcher = certificates.NewDomainWatcher(certManager, domainProvider)

						logger.Infof("Certificate manager initialized and started with email: %s", email)
					}

					// Add to domain provider for certificate management if we have a cert manager
					if certManager != nil && domainWatcher != nil {
						domainProvider.AddContainer(container.ID, domains)
						// Trigger domain sync
						go domainWatcher.SyncDomains()
					} else {
						logger.Warn("Certificate manager not initialized - TLS is enabled but no container with turkis.acme.email label found")
					}
				}

				// Get health check path from labels or use default
				healthCheckPath := "/"
				if path, ok := container.Config.Labels["turkis.health-check-path"]; ok && path != "" {
					healthCheckPath = path
				}

				// Get drain time from labels or use default
				drainTime := 10 * time.Second
				if dt, ok := container.Config.Labels["turkis.drain-time"]; ok && dt != "" {
					if parsed, err := time.ParseDuration(dt); err == nil {
						drainTime = parsed
					}
				}

				// Set up blue-green deployment config
				bgConfig := haproxy.NewDefaultBlueGreenConfig(
					domains.AppName,
					domains.DeploymentID,
					ipAddress,
					primaryPort,
					domains,
				)

				// Customize based on container labels
				bgConfig.HealthCheckPath = healthCheckPath
				bgConfig.DrainTime = drainTime

				// Execute blue-green deployment in a goroutine to avoid blocking the event loop
				go func() {
					// Create a context with timeout for the deployment process
					deployCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
					defer cancel()

					log.Printf("Starting blue-green deployment for app '%s' (deployment: '%s')",
						domains.AppName, domains.DeploymentID)

					err := haproxyClient.ExecuteBlueGreen(deployCtx, bgConfig)
					if err != nil {
						log.Printf("Blue-green deployment failed: %v", err)
					} else {
						// Update the deployment registry with the new deployment ID
						deploymentRegistryLock.Lock()
						deploymentRegistry[domains.AppName] = domains.DeploymentID
						deploymentRegistryLock.Unlock()

						log.Printf("Blue-green deployment completed for app '%s' (deployment: '%s')",
							domains.AppName, domains.DeploymentID)
					}
				}()

			case "die", "stop", "kill":
				log.Printf("Container %s event: %s", event.Action, event.Actor.ID[:12])

				// Get container details to find app name and deployment ID
				container, err := dockerClient.ContainerInspect(ctx, event.Actor.ID)
				if err != nil {
					log.Printf("Failed to inspect container for removal: %v", err)
					continue
				}

				// Extract app name and deployment ID from labels
				appName := container.Config.Labels["turkis.appName"]
				deploymentID := container.Config.Labels["turkis.deployment"]

				// Remove from domain provider for certificate management
				if !noTLS && domainProvider != nil {
					domainProvider.RemoveContainer(container.ID)
					// Trigger domain sync only if domainWatcher is initialized
					if domainWatcher != nil {
						go domainWatcher.SyncDomains()
					}
				}

				if appName != "" {
					// Remove HAProxy configuration for this container
					err = haproxyClient.RemoveBackend(appName, deploymentID)
					if err != nil {
						log.Printf("Error removing HAProxy configuration: %v", err)
						continue
					}
					log.Printf("Successfully removed HAProxy configuration for container %s", container.ID[:12])
				} else {
					log.Printf("Container %s has no app name label, skipping HAProxy cleanup", container.ID[:12])
				}
			}

		case err := <-errorsChan:
			log.Printf("Error from Docker events: %v", err)
		case <-refreshTicker.C:
			// Periodic full refresh
			log.Println("Performing periodic HAProxy configuration refresh")

			// Get all running containers on our network
			containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{})
			if err != nil {
				log.Printf("Error listing containers for refresh: %v", err)
				continue
			}

			for _, containerSummary := range containers {
				// Check if container is on our network
				eligible, err := isContainerEligible(ctx, dockerClient, containerSummary.ID)
				if err != nil || !eligible {
					continue
				}

				// Get full container details
				container, err := dockerClient.ContainerInspect(ctx, containerSummary.ID)
				if err != nil {
					continue
				}

				// Process container same as for "start" event
				ipAddress, _, _ := monitor.ContainerNetworkInfo(container, config.DockerNetwork)
				primaryPort, _ := monitor.PrimaryPort(container)
				domains, _ := monitor.ParseContainerDomains(container.Config.Labels)

				if ipAddress != "" && primaryPort != "" && len(domains.Domains) > 0 {
					// Refresh HAProxy configuration
					_ = haproxyClient.ConfigureFromDomains(domains, ipAddress, primaryPort)
				}
			}

			log.Println("HAProxy configuration refresh completed")

			// Update domain provider with current containers
			if !noTLS {
				// Reset domain provider to ensure it's in sync with running containers
				domainProvider = monitor.NewDomainProvider()

				// Also check for email label in any container
				var emailFound bool

				// Add all running containers to domain provider
				for _, containerSummary := range containers {
					// Skip if not on our network
					eligible, err := isContainerEligible(ctx, dockerClient, containerSummary.ID)
					if err != nil || !eligible {
						continue
					}

					container, err := dockerClient.ContainerInspect(ctx, containerSummary.ID)
					if err != nil {
						continue
					}

					// Check for email label to initialize certificate manager if needed
					if certManager == nil {
						if email, hasEmail := container.Config.Labels["turkis.acme.email"]; hasEmail && email != "" {
							emailFound = true

							// Initialize certificate manager with email from container label
							certConfig := certificates.Config{
								Email:         email,
								CertDir:       CertificatesDir,
								WebRootDir:    WebRootDir,
								HAProxySocket: HAProxySocketPath,
								Logger:        logger,
								TlsStaging:    tlsStaging,
							}

							var err error
							certManager, err = certificates.NewManager(certConfig)
							if err != nil {
								logger.Fatalf("Failed to initialize certificate manager: %v", err)
							}

							// Start certificate manager
							if err := certManager.Start(); err != nil {
								logger.Fatalf("Failed to start certificate manager: %v", err)
							}

							logger.Infof("Certificate manager initialized during refresh with email: %s", email)
						}
					}

					domains, err := monitor.ParseContainerDomains(container.Config.Labels)
					if err != nil || len(domains.Domains) == 0 {
						continue
					}

					domainProvider.AddContainer(containerSummary.ID, domains)
				}

				if !emailFound && certManager == nil {
					logger.Warn("Certificate manager not initialized - no container with turkis.acme.email label found")
				}

				// Sync domains with certificate manager if we have one
				if certManager != nil {
					domainWatcher = certificates.NewDomainWatcher(certManager, domainProvider)
					if domainWatcher != nil {
						domainWatcher.SyncDomains()
					}
				}
			}

		case <-certRefreshTicker.C:
			if !noTLS && certManager != nil {
				// Trigger domain sync to check for certificate renewals
				log.Println("Performing periodic certificate refresh")
				domainWatcher.SyncDomains()
			}
		}
	}
}

// listenForDockerEvents sets up a listener for Docker events
func listenForDockerEvents(ctx context.Context, dockerClient *client.Client, eventsChan chan events.Message, errorsChan chan error) {
	// Set up filter for container events
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "container")

	// Start listening for events
	eventOptions := types.EventsOptions{
		Filters: filterArgs,
	}

	events, errs := dockerClient.Events(ctx, eventOptions)

	// Forward events and errors to our channels
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			// Only process events for containers on our network
			if event.Action == "start" || event.Action == "die" || event.Action == "stop" || event.Action == "kill" {
				eligible, err := isContainerEligible(ctx, dockerClient, event.Actor.ID)
				if err != nil {
					log.Printf("Error checking if we should handle container: %v", err)
					continue
				}

				if eligible {
					// log.Printf("Container %s event on network %s: %s", event.Action, config.DockerNetwork, event.Actor.ID[:12])
					eventsChan <- event
					// TODO: remove this else block. It is only for testing.
				} else {
					log.Printf("Container %s event not on network: %s", event.Action, event.Actor.ID[:12])
				}
			}
		case err := <-errs:
			if err != nil {
				errorsChan <- err
				// For non-fatal errors, you might want to reconnect instead of exiting
				if err != io.EOF && !strings.Contains(err.Error(), "connection refused") {
					// Attempt to reconnect
					time.Sleep(5 * time.Second)
					events, errs = dockerClient.Events(ctx, eventOptions)
					continue
				}
			}
			return
		}
	}
}

// isContainerEligible checks if a container should be handled by turkis.
func isContainerEligible(ctx context.Context, dockerClient *client.Client, containerID string) (bool, error) {
	container, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}

	// check if container has the ignore label.
	if container.Config.Labels["turkis.ignore"] == "true" {
		return false, nil
	}

	// Check if container is on our network
	for netName := range container.NetworkSettings.Networks {
		if netName == config.DockerNetwork {
			return true, nil
		}
	}

	return false, nil
}
