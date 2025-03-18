package haproxy

import (
	"fmt"
	"strings"

	"github.com/ameistad/turkis/internal/config"
)

type DeploymentInstance struct {
	IP   string
	Port string
}

// Deployment holds the ContainerLabels, IP and Port for a container.
type Deployment struct {
	Labels    *config.ContainerLabels
	Instances []DeploymentInstance
}

// GenerateMultiConfig creates an HAProxy 3.1 config for multiple deployments.
// It creates a single frontend that binds on port 443 and defines ACLs for each
// deployment based on their domains, and then it defines separate backend sections.
func GenerateConfig(deployments []Deployment) (string, error) {
	// Build the frontend section with a common bind on 443 (with TLS certificates)
	frontend := "frontend https-in\n\tbind *:443 ssl crt /usr/local/etc/haproxy/certs/\n"
	for _, d := range deployments {
		// Collect unique domains (canonical and aliases) for the given deployment.
		domainSet := make(map[string]struct{})
		for _, domain := range d.Labels.Domains {
			if domain.Canonical != "" {
				domainSet[domain.Canonical] = struct{}{}
			}
			for _, alias := range domain.Aliases {
				if alias != "" {
					domainSet[alias] = struct{}{}
				}
			}
		}

		var domains []string
		for dname := range domainSet {
			domains = append(domains, dname)
		}
		aclHosts := strings.Join(domains, " ")

		// Derive a backend name from the container's AppName.
		backendName := d.Labels.AppName

		// Append ACL rule and backend usage to the frontend section.
		frontend += fmt.Sprintf("\tacl host_%s hdr(host) -i %s\n", backendName, aclHosts)
		frontend += fmt.Sprintf("\tuse_backend %s if host_%s\n", backendName, backendName)
	}

	// Build backend sections for all deployments.
	backends := ""
	for _, d := range deployments {
		backendName := d.Labels.AppName
		backends += fmt.Sprintf("\nbackend %s\n", backendName)

		// Loop over each instance and add a unique server entry.
		for i, inst := range d.Instances {
			backends += fmt.Sprintf("\tserver app%d %s:%s check\n", i+1, inst.IP, inst.Port)
		}
	}

	config := frontend + "\n" + backends
	return config, nil
}

// TODO: investigate options to use the running haproxy container to validate the config file.
