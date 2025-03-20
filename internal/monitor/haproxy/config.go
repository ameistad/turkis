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
func CreateConfig(deployments []Deployment) (string, error) {
	// HTTPS frontend (existing behavior)
	httpsFrontend := "frontend https-in\n\tbind *:443 ssl crt /usr/local/etc/haproxy/certs/\n"
	// HTTP frontend (new): will redirect all requests to HTTPS.
	httpFrontend := "frontend http-in\n\tbind *:80\n"

	for _, d := range deployments {
		backendName := d.Labels.AppName
		var canonicalACLs []string

		// Process each domain mapping individually.
		for _, domain := range d.Labels.Domains {
			if domain.Canonical != "" {
				canonicalKey := strings.ReplaceAll(domain.Canonical, ".", "_")
				canonicalACLName := fmt.Sprintf("%s_%s_canonical", backendName, canonicalKey)

				// Add canonical ACL to HTTPS frontend.
				httpsFrontend += fmt.Sprintf("\tacl %s hdr(host) -i %s\n", canonicalACLName, domain.Canonical)
				canonicalACLs = append(canonicalACLs, canonicalACLName)

				// Add canonical ACL and redirect rule to HTTP frontend.
				httpFrontend += fmt.Sprintf("\tacl %s hdr(host) -i %s\n", canonicalACLName, domain.Canonical)
				httpFrontend += fmt.Sprintf("\thttp-request redirect code 301 location https://%s%%[req.uri] if %s\n",
					domain.Canonical, canonicalACLName)

				// For each alias, create corresponding ACLs and redirect rules.
				for _, alias := range domain.Aliases {
					if alias != "" {
						aliasKey := strings.ReplaceAll(alias, ".", "_")
						aliasACLName := fmt.Sprintf("%s_%s_alias", backendName, aliasKey)
						// HTTPS rules for alias:
						httpsFrontend += fmt.Sprintf("\tacl %s hdr(host) -i %s\n", aliasACLName, alias)
						httpsFrontend += fmt.Sprintf("\thttp-request redirect code 301 location https://%s%%[req.uri] if %s\n",
							domain.Canonical, aliasACLName)
						// HTTP rules for alias:
						httpFrontend += fmt.Sprintf("\tacl %s hdr(host) -i %s\n", aliasACLName, alias)
						httpFrontend += fmt.Sprintf("\thttp-request redirect code 301 location https://%s%%[req.uri] if %s\n",
							domain.Canonical, aliasACLName)
					}
				}
			}
		}

		// In HTTPS frontend, only requests matching a canonical domain are forwarded.
		if len(canonicalACLs) > 0 {
			httpsFrontend += fmt.Sprintf("\tuse_backend %s if %s\n", backendName, strings.Join(canonicalACLs, " or "))
		}
	}

	// Build backend sections for all deployments.
	backends := ""
	for _, d := range deployments {
		backendName := d.Labels.AppName
		backends += fmt.Sprintf("\nbackend %s\n", backendName)
		for i, inst := range d.Instances {
			backends += fmt.Sprintf("\tserver app%d %s:%s check\n", i+1, inst.IP, inst.Port)
		}
	}

	// ACME challenge
	frontendACMEChallenge := `
    acl is_acme_challenge path_beg /.well-known/acme-challenge/
    use_backend acme_challenge if is_acme_challenge
	`
	backendACMEChallenge := `
backend acme_challenge
    mode http
    # Forward to the monitor container which will handle the ACME challenge
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Proto http
    http-request set-header X-Forwarded-Port %[dst_port]
    http-request set-header Host %[req.hdr(host)]
    server monitor monitor:8080
	`
	backendDefalt := `
backend default_backend
    http-request deny deny_status 404
	`

	// Concatenate HTTPS and HTTP frontends with backends.
	config := httpsFrontend + "\n" + httpFrontend + "\n" + frontendACMEChallenge + "\n" + backends + "\n" + backendACMEChallenge + "\n" + backendDefalt
	return config, nil
}

// TODO: investigate options to use the running haproxy container to validate the config file.
