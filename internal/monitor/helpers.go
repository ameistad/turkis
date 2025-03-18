package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/ameistad/turkis/internal/config"
	"github.com/ameistad/turkis/internal/monitor/haproxy"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// Common errors
var (
	ErrNoIPAddress     = errors.New("container has no IP address on the specified network")
	ErrNetworkNotFound = errors.New("specified network not found")
	ErrNoPortsExposed  = errors.New("container has no exposed ports")
)

// ContainerNetworkInfo extracts the container's IP address and exposed ports
func ContainerNetworkIP(container types.ContainerJSON, networkName string) (string, error) {
	// Check if the network exists
	if _, exists := container.NetworkSettings.Networks[networkName]; !exists {
		return "", fmt.Errorf("%w: %s", ErrNetworkNotFound, networkName)
	}

	// Get IP address from the specified network
	ipAddress := container.NetworkSettings.Networks[networkName].IPAddress
	if ipAddress == "" {
		return "", fmt.Errorf("%w: %s", ErrNoIPAddress, networkName)
	}

	return ipAddress, nil
}

func GetDeploymentsFromRunningContainers(ctx context.Context, dockerClient *client.Client) ([]haproxy.Deployment, error) {
	deploymentsMap := make(map[string]haproxy.Deployment)
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}

	for _, containerSummary := range containers {
		container, err := dockerClient.ContainerInspect(ctx, containerSummary.ID)
		if err != nil {
			log.Printf("Failed to inspect container %s: %v", containerSummary.ID, err)
			continue
		}

		labels, err := config.ParseContainerLabels(container.Config.Labels)
		if err != nil {
			continue
		}

		ip, err := ContainerNetworkIP(container, config.DockerNetwork)
		if err != nil {
			log.Printf("Failed to get IP address IP for container %s: %v", container.ID, err)
			continue
		}

		var port string
		if labels.Port != "" {
			port = labels.Port
		} else {
			port = config.DefaultContainerPort
		}

		instance := haproxy.DeploymentInstance{IP: ip, Port: port}

		if deployment, exists := deploymentsMap[labels.AppName]; exists {

			// Only add instances if the deployment ID matches.
			if deployment.Labels.DeploymentID == labels.DeploymentID {
				deployment.Instances = append(deployment.Instances, instance)
				deploymentsMap[labels.AppName] = deployment
			} else {
				// Replace the deployment if the new one has a higher deployment ID.
				if deployment.Labels.DeploymentID < labels.DeploymentID {
					deploymentsMap[labels.AppName] = haproxy.Deployment{Labels: labels, Instances: []haproxy.DeploymentInstance{instance}}
				}
			}
		} else {
			deploymentsMap[labels.AppName] = haproxy.Deployment{Labels: labels, Instances: []haproxy.DeploymentInstance{instance}}
		}
	}
	var deployments []haproxy.Deployment
	for _, deployment := range deploymentsMap {
		deployments = append(deployments, deployment)
	}
	return deployments, nil
}
