package discovery

import (
	"context"
	"sync"

	"multicloud-exporter/internal/config"
)

// Discoverer defines the interface for cloud product discovery
type Discoverer interface {
	Discover(ctx context.Context, cfg *config.Config) []config.Product
}

var (
	registry = make(map[string]Discoverer)
	mu       sync.RWMutex
)

// Register registers a discoverer for a cloud provider
func Register(provider string, d Discoverer) {
	mu.Lock()
	defer mu.Unlock()
	registry[provider] = d
}

// GetDiscoverer returns the discoverer for a cloud provider
func GetDiscoverer(provider string) (Discoverer, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := registry[provider]
	return d, ok
}

// GetAllDiscoverers returns all registered providers
func GetAllDiscoverers() []string {
	mu.RLock()
	defer mu.RUnlock()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}
