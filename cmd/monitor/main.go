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
	"syscall"
	"time"

	"github.com/ameistad/turkis/internal/config"
	"github.com/ameistad/turkis/internal/monitor"
	"github.com/ameistad/turkis/internal/monitor/haproxy"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

const (
	// RefreshInterval is how often to refresh the full configuration
	RefreshInterval = 5 * time.Minute
	// CertificatesDir is the directory where certificates are stored
	CertificatesDir = "/usr/local/etc/haproxy/certs"
	// WebRootDir is the directory for ACME HTTP-01 challenges
	WebRootDir = "/var/www/lego"
	// CertRefreshInterval is how often to check for certificate renewals
	CertRefreshInterval = 12 * time.Hour
)

var logger = logrus.New()

type ContainerEvent struct {
	Event     events.Message
	Container types.ContainerJSON
}

func main() {
	// Parse command line flags
	dryRunFlag := flag.Bool("dry-run", false, "Run in dry-run mode (don't actually send commands to HAProxy)")
	flag.Parse()

	// Configure logger
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	dryRunEnv := os.Getenv("DRY_RUN") == "true"
	dryRun := *dryRunFlag || dryRunEnv

	if dryRun {
		fmt.Println("========================")
		fmt.Println("STARTING IN DRY RUN MODE")
		fmt.Println("No changes will be made to HAProxy configuration")
		fmt.Println("========================")
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	haproxyClient := haproxy.NewMasterClient()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel for Docker events
	eventsChan := make(chan ContainerEvent)
	errorsChan := make(chan error)

	// Initialize certificate manager if TLS is enabled
	// var certManager *certificates.Manager
	// var domainWatcher *certificates.DomainWatcher

	// Start Docker event listener
	go listenForDockerEvents(ctx, dockerClient, eventsChan, errorsChan)

	// Start periodic full refresh
	refreshTicker := time.NewTicker(RefreshInterval)
	defer refreshTicker.Stop()

	// Start certificate refresh ticker if TLS is enabled
	certRefreshTicker := time.NewTicker(CertRefreshInterval)
	defer certRefreshTicker.Stop()

	fmt.Printf("Monitor service started on network %s...\n", config.DockerNetwork)

	// Main event loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down gracefully...")
			// Stop certificate manager
			// if certManager != nil {
			// 	certManager.Stop()
			// }
			cancel()
			return
		case e := <-eventsChan:
			switch e.Event.Action {
			case "start":
				log.Printf("Container %s event: %s", e.Event.Action, e.Event.Actor.ID[:12])
				// Get container details

				labels, err := config.ParseContainerLabels(e.Container.Config.Labels)
				if err != nil {
					log.Printf("Error parsing container labels: %v", err)
					continue
				}

				log.Printf("Container %s has app name '%s' and deployment ID '%s'", e.Container.ID[:12], labels.AppName, labels.DeploymentID)

				// Execute in a goroutine to avoid blocking the event loop
				go func() {
					// Create a child context for the deployment process.
					_, cancelDeployment := context.WithCancel(ctx)
					defer cancelDeployment()

					log.Printf("Starting deployment for %s\n", labels.AppName)

					deployments, err := monitor.CreateDeployments(ctx, dockerClient)
					if err != nil {
						log.Printf("Failed to create deployments: %v", err)
						return
					}

					buf, err := haproxy.CreateConfig(deployments)
					if err != nil {
						log.Printf("Failed to create config %v", err)
						return
					}

					configDirPath, err := config.ConfigDirPath()
					if err != nil {
						log.Printf("Failed to determine config directory path: %v", err)
						return
					}

					if !dryRun {
						if err := os.WriteFile(configDirPath, buf.Bytes(), 0644); err != nil {
							log.Printf("Failed to write updated config file: %v", err)
							return
						}
						haproxyClient.SendCommand("reload")
					} else {
						log.Printf("Generated HAProxy config would have been written to %s:\n%s", configDirPath, buf.String())
					}

					log.Printf("Deployment completed for app '%s' (deployment: '%s')",
						labels.AppName, labels.DeploymentID)
				}()

			case "die", "stop", "kill":
				log.Printf("Container %s event: %s", e.Event.Action, e.Event.Actor.ID[:12])

				labels, err := config.ParseContainerLabels(e.Container.Config.Labels)
				if err != nil {
					log.Printf("Error parsing container labels: %v", err)
					continue
				}

				// TODO: clean up old deployements:
				// - remove old containers
				// - remove old certificates
				// - remove old HAProxy backends
				logger.Printf("Removing container %s", labels.AppName)

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
				container, err := dockerClient.ContainerInspect(ctx, containerSummary.ID)
				if err != nil {
					continue
				}

				// Check if container is on our network
				eligible := isContainerEligible(container)
				if !eligible {
					continue
				}

				labels, err := config.ParseContainerLabels(container.Config.Labels)
				if err != nil {
					log.Printf("Error parsing container labels: %v", err)
					continue
				}

				// TODO: do the same as for the start event.
				logger.Printf("Refreshing container %s", labels.AppName)
			}

			log.Println("HAProxy configuration refresh completed")

		case <-certRefreshTicker.C:
			log.Println("Performing periodic certificate refresh")
		}
	}
}

// listenForDockerEvents sets up a listener for Docker events
func listenForDockerEvents(ctx context.Context, dockerClient *client.Client, eventsChan chan ContainerEvent, errorsChan chan error) {
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

				container, err := dockerClient.ContainerInspect(ctx, event.Actor.ID)
				if err != nil {
					log.Printf("Error inspecting container %s: %v", event.Actor.ID[:12], err)
					continue
				}
				eligible := isContainerEligible(container)

				if eligible {
					containerEvent := ContainerEvent{
						Event:     event,
						Container: container,
					}
					eventsChan <- containerEvent
					// TODO: remove this else block. It is only for testing.
				} else {
					log.Printf("Container %s event but not eligible: %s", event.Action, event.Actor.ID[:12])
				}
			}
		case err := <-errs:
			if err != nil {
				errorsChan <- err
				// For non-fatal errors we'll try to reconnect instead of exiting
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
func isContainerEligible(container types.ContainerJSON) bool {
	if container.Config.Labels["turkis.ignore"] == "true" {
		return false
	}

	isOnNetwork := isOnNetworkCheck(container, config.DockerNetwork)
	return isOnNetwork
}

func isOnNetworkCheck(container types.ContainerJSON, networkName string) bool {
	for netName := range container.NetworkSettings.Networks {
		if netName == networkName {
			return true
		}
	}
	return false
}
