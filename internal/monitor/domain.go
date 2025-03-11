package monitor

import (
	"fmt"
	"strings"
)

// Domain represents a canonical domain and its associated aliases
type Domain struct {
	Name    string   // The canonical domain name
	Aliases []string // Domain aliases that should redirect to the canonical domain
}

// ContainerDomains stores all domains for a container
type ContainerDomains struct {
	AppName        string   // Application name
	DeploymentID   string   // Unique deployment identifier
	Domains        []Domain // All domains for this container
	AllDomainsList []string // Flattened list of all domains and aliases
}

// ParseContainerDomains extracts domains from container labels
func ParseContainerDomains(containerLabels map[string]string) (ContainerDomains, error) {
	result := ContainerDomains{
		AppName:      containerLabels["turkis.app"],
		DeploymentID: containerLabels["turkis.deployment"],
		Domains:      []Domain{},
	}

	// Validate that we have the required labels
	if result.AppName == "" {
		return ContainerDomains{}, fmt.Errorf("container labels missing required 'turkis.app' label")
	}

	// Parse the all-domains list if available
	if allDomainsStr, ok := containerLabels["turkis.domains.all"]; ok && allDomainsStr != "" {
		result.AllDomainsList = strings.Split(allDomainsStr, ",")
	}

	// First, identify all the domain indices we have
	domainIndices := make(map[int]bool)
	for label := range containerLabels {
		if !strings.HasPrefix(label, "turkis.domain.") {
			continue
		}

		parts := strings.Split(label, ".")
		if len(parts) >= 3 {
			index := -1
			if _, err := fmt.Sscanf(parts[2], "%d", &index); err == nil && index >= 0 {
				domainIndices[index] = true
			}
		}
	}

	// Check if we found any domains
	if len(domainIndices) == 0 {
		return ContainerDomains{}, fmt.Errorf("container has no 'turkis.domain.*' labels")
	}

	// Then process each domain index
	for index := range domainIndices {
		domainKey := fmt.Sprintf("turkis.domain.%d", index)
		canonicalName := containerLabels[domainKey]

		if canonicalName == "" {
			return ContainerDomains{}, fmt.Errorf("domain index %d is missing its domain name", index)
		}

		// Create a new domain entry
		domain := Domain{
			Name:    canonicalName,
			Aliases: []string{},
		}

		// Collect all aliases for this domain
		aliasPrefix := fmt.Sprintf("turkis.domain.%d.alias.", index)
		for label, value := range containerLabels {
			if strings.HasPrefix(label, aliasPrefix) {
				domain.Aliases = append(domain.Aliases, value)
			}
		}

		result.Domains = append(result.Domains, domain)
	}

	return result, nil
}
