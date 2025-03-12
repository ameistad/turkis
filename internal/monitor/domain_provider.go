package monitor

import (
	"sync"
)

// DomainProviderImpl implements the certificates.DomainProvider interface
type DomainProviderImpl struct {
	// Container domains indexed by container ID
	containers     map[string]ContainerDomains
	containerMutex sync.RWMutex
}

// NewDomainProvider creates a new domain provider
func NewDomainProvider() *DomainProviderImpl {
	return &DomainProviderImpl{
		containers: make(map[string]ContainerDomains),
	}
}

// GetAllDomains implements the certificates.DomainProvider interface
// Returns a map of domain name -> aliases
func (dp *DomainProviderImpl) GetAllDomains() map[string][]string {
	dp.containerMutex.RLock()
	defer dp.containerMutex.RUnlock()
	
	// Create a map of domain name -> aliases
	domains := make(map[string][]string)
	
	// Collect domains from all containers
	for _, containerDomains := range dp.containers {
		for _, domain := range containerDomains.Domains {
			// Add domain to the map (we overwrite duplicates, which is fine
			// as long as the aliases are the same)
			domains[domain.Name] = domain.Aliases
		}
	}
	
	return domains
}

// AddContainer adds a container's domains to the provider
func (dp *DomainProviderImpl) AddContainer(containerID string, domains ContainerDomains) {
	dp.containerMutex.Lock()
	defer dp.containerMutex.Unlock()
	
	dp.containers[containerID] = domains
}

// RemoveContainer removes a container's domains from the provider
func (dp *DomainProviderImpl) RemoveContainer(containerID string) {
	dp.containerMutex.Lock()
	defer dp.containerMutex.Unlock()
	
	delete(dp.containers, containerID)
}