package collector

import (
	"sync"
	"testing"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"

	"github.com/stretchr/testify/assert"
)

// MockProvider for testing
type MockProvider struct {
	CollectCalled bool
	mu            sync.Mutex
}

func (m *MockProvider) Collect(account config.CloudAccount) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CollectCalled = true
}

func (m *MockProvider) GetDefaultResources() []string {
	return []string{"mock_res"}
}

func TestCollector_Collect(t *testing.T) {
	// 1. Setup Mock Provider
	mockP := &MockProvider{}
	providers.Register("mock_cloud", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return mockP
	})

	// 2. Config
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"mock_cloud": {
				{
					Provider:        "mock_cloud",
					AccountID:       "test-account",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-mock"},
				},
			},
		},
		Server: &config.ServerConf{
			RegionConcurrency:  1,
			MetricConcurrency:  1,
			ProductConcurrency: 1,
		},
	}
	mgr := discovery.NewManager(cfg)

	// 3. Create Collector
	c := NewCollector(cfg, mgr)

	// 4. Run Collect
	c.Collect()

	// 5. Verify
	mockP.mu.Lock()
	called := mockP.CollectCalled
	mockP.mu.Unlock()
	assert.True(t, called, "Provider.Collect should be called")

	// Verify Status
	status := c.GetStatus()
	assert.NotEmpty(t, status.LastStart)
	assert.NotEmpty(t, status.LastEnd)
}

func TestCollector_CollectFiltered(t *testing.T) {
	// 1. Setup Mock Provider
	mockP := &MockProvider{}
	providers.Register("mock_cloud_filter", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return mockP
	})

	// 2. Config
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"mock_cloud_filter": {
				{
					Provider:  "mock_cloud_filter",
					Resources: []string{"res1"},
				},
			},
			"other_cloud": {
				{
					Provider:  "other_cloud",
					Resources: []string{"res2"},
				},
			},
		},
	}
	mgr := discovery.NewManager(cfg)

	c := NewCollector(cfg, mgr)

	// 3. Collect Filtered (should match)
	c.CollectFiltered("mock_cloud_filter", "")
	mockP.mu.Lock()
	called := mockP.CollectCalled
	mockP.mu.Unlock()
	assert.True(t, called, "Provider.Collect should be called for matching filter")

	// Reset
	mockP.mu.Lock()
	mockP.CollectCalled = false
	mockP.mu.Unlock()

	// 4. Collect Filtered (should not match)
	c.CollectFiltered("other_cloud", "")
	mockP.mu.Lock()
	called = mockP.CollectCalled
	mockP.mu.Unlock()
	assert.False(t, called, "Provider.Collect should NOT be called for non-matching filter")
}
