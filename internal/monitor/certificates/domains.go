package certificates

import (
	"sync"
)

// DomainProvider is an interface for getting domains from container configurations
type DomainProvider interface {
	// GetAllDomains returns all domains currently in use
	GetAllDomains() map[string][]string // domain -> aliases
}

// DomainWatcher watches for domain changes and updates the certificate manager
type DomainWatcher struct {
	manager  *Manager
	provider DomainProvider
	
	// For tracking domains we've already processed
	knownDomains map[string]struct{}
	domainMutex  sync.Mutex
}

// NewDomainWatcher creates a new domain watcher
func NewDomainWatcher(manager *Manager, provider DomainProvider) *DomainWatcher {
	return &DomainWatcher{
		manager:      manager,
		provider:     provider,
		knownDomains: make(map[string]struct{}),
	}
}

// SyncDomains synchronizes domains from the provider to the certificate manager
func (dw *DomainWatcher) SyncDomains() {
	dw.domainMutex.Lock()
	defer dw.domainMutex.Unlock()
	
	// Get all domains from the provider
	domains := dw.provider.GetAllDomains()
	
	// Track domains we've seen in this cycle
	seenDomains := make(map[string]struct{})
	
	// Add new domains to the certificate manager
	for domainName, aliases := range domains {
		seenDomains[domainName] = struct{}{}
		
		// Skip if we already know about this domain
		if _, exists := dw.knownDomains[domainName]; exists {
			continue
		}
		
		// Add domain to certificate manager
		dw.manager.AddDomain(&Domain{
			Name:    domainName,
			Aliases: aliases,
		})
		
		// Mark as known
		dw.knownDomains[domainName] = struct{}{}
	}
	
	// Remove domains that are no longer in use
	for domainName := range dw.knownDomains {
		if _, exists := seenDomains[domainName]; !exists {
			// Domain is no longer in use
			dw.manager.RemoveDomain(domainName)
			delete(dw.knownDomains, domainName)
		}
	}
}