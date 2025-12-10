package providers

import (
	"sort"
	"testing"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"

	"github.com/stretchr/testify/assert"
)

type mockProvider struct{}

func (m *mockProvider) Collect(account config.CloudAccount) {}
func (m *mockProvider) GetDefaultResources() []string       { return []string{} }

func TestRegistry(t *testing.T) {
	// Backup original registry
	mu.Lock()
	originalRegistry := make(map[string]Factory)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	// Clear registry for test stability
	registry = make(map[string]Factory)
	mu.Unlock()

	// Restore registry after test
	defer func() {
		mu.Lock()
		registry = originalRegistry
		mu.Unlock()
	}()

	mockFactory := func(cfg *config.Config, mgr *discovery.Manager) Provider {
		return &mockProvider{}
	}

	// Test Register
	Register("mock", mockFactory)

	// Test GetFactory
	f, ok := GetFactory("mock")
	assert.True(t, ok)
	assert.NotNil(t, f)
	p := f(nil, nil)
	assert.NotNil(t, p)

	_, ok = GetFactory("non-existent")
	assert.False(t, ok)

	// Test GetAllProviders
	Register("mock2", mockFactory)
	providers := GetAllProviders()
	sort.Strings(providers)
	assert.Equal(t, []string{"mock", "mock2"}, providers)
}
