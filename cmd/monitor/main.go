package main

import (
	"context"
	"errors"
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
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
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
)

func main() {
	// Initialize Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel for Docker events
	eventsChan := make(chan events.Message)
	errorsChan := make(chan error)

	// Start Docker event listener
	go listenForDockerEvents(ctx, dockerClient, eventsChan, errorsChan)

	// Start periodic full refresh
	refreshTicker := time.NewTicker(RefreshInterval)
	defer refreshTicker.Stop()

	fmt.Printf("Monitor service started on network %s...\n", config.DockerNetwork)

	// Main event loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down gracefully...")
			cancel()
			return
		case event := <-eventsChan:
			// Process Docker events

			switch event.Action {
			case "start":
				log.Printf("Container %s event: %s", event.Action, event.Actor.ID[:12])
				// Get container details
				container, err := dockerClient.ContainerInspect(ctx, event.Actor.ID)
				if err != nil {
					log.Printf("failed to inspect container: %v", err)
				}

				ipAddress, exposedPorts, err := monitor.ContainerNetworkInfo(container, config.DockerNetwork)
				if err != nil {
					if errors.Is(err, monitor.ErrNoIPAddress) {
						log.Printf("Warning: Container %s has no IP address on network %s, skipping",
							container.ID[:12], config.DockerNetwork)
					} else {
						log.Printf("Error getting container network info: %v", err)
					}
				} else {
					log.Printf("Container %s has IP: %s with %d exposed ports",
						container.ID[:12], ipAddress, len(exposedPorts))
				}

				primaryPort, err := monitor.PrimaryPort(container)
				if err != nil {
					if errors.Is(err, monitor.ErrNoPortsExposed) {
						log.Printf("Note: Container %s has no exposed ports", container.ID[:12])
					} else {
						log.Printf("Error getting container primary port: %v", err)
					}
				} else {
					log.Printf("Container %s primary port: %s", container.ID[:12], primaryPort)
				}

				domains, err := monitor.ParseContainerDomains(container.Config.Labels)
				if err != nil {
					log.Printf("Error parsing container domains: %v", err)
					continue
				}
				// Log what domains we found
				for _, domain := range domains.Domains {
					if len(domain.Aliases) > 0 {
						log.Printf("Domain: %s with %d aliases", domain.Name, len(domain.Aliases))
						for i, alias := range domain.Aliases {
							log.Printf("  Alias %d: %s -> %s", i, alias, domain.Name)
						}
					} else {
						log.Printf("Domain: %s (no aliases)", domain.Name)
					}
				}
			case "die", "stop", "kill":
				log.Printf("Container %s event: %s", event.Action, event.Actor.ID[:12])
				// Update HAProxy configuration
				// TODO: Implement
			}

		case err := <-errorsChan:
			log.Printf("Error from Docker events: %v", err)
		case <-refreshTicker.C:
			// Periodic full refresh
			log.Println("Performing periodic HAProxy configuration refresh")
			// TODO: Implement
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
				isOnNetwork, err := isContainerOnNetwork(ctx, dockerClient, event.Actor.ID, config.DockerNetwork)
				if err != nil {
					log.Printf("Error checking container network: %v", err)
					continue
				}

				if isOnNetwork {
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

// isContainerOnNetwork checks if a container is connected to the specified network
func isContainerOnNetwork(ctx context.Context, dockerClient *client.Client, containerID, networkName string) (bool, error) {
	container, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}

	for netName := range container.NetworkSettings.Networks {
		if netName == networkName {
			return true, nil
		}
	}

	return false, nil
}
