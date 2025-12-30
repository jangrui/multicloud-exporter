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

// Benchmark tests for performance measurement

// BenchmarkCollector_Collect measures the performance of a full collection cycle
func BenchmarkCollector_Collect(b *testing.B) {
	// Setup Mock Provider
	mockP := &MockProvider{}
	providers.Register("mock_cloud_bench", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return mockP
	})

	// Create a realistic configuration with multiple accounts and regions
	accounts := make([]config.CloudAccount, 10)
	for i := 0; i < 10; i++ {
		accounts[i] = config.CloudAccount{
			Provider:        "mock_cloud_bench",
			AccountID:       "bench-account-" + string(rune(i)),
			AccessKeyID:     "ak",
			AccessKeySecret: "sk",
			Regions:         []string{"cn-mock-1", "cn-mock-2", "cn-mock-3"},
		}
	}

	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"mock_cloud_bench": accounts,
		},
		Server: &config.ServerConf{
			RegionConcurrency:  4,
			MetricConcurrency:  5,
			ProductConcurrency: 2,
		},
	}
	mgr := discovery.NewManager(cfg)

	c := NewCollector(cfg, mgr)

	// Reset timer before actual benchmarking
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Collect()
	}
}

// BenchmarkCollector_CollectFiltered measures the performance of filtered collection
func BenchmarkCollector_CollectFiltered(b *testing.B) {
	mockP := &MockProvider{}
	providers.Register("mock_cloud_filtered_bench", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return mockP
	})

	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"mock_cloud_filtered_bench": {
				{
					Provider:  "mock_cloud_filtered_bench",
					Resources: []string{"res1"},
				},
			},
		},
	}
	mgr := discovery.NewManager(cfg)

	c := NewCollector(cfg, mgr)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.CollectFiltered("mock_cloud_filtered_bench", "")
	}
}

// BenchmarkCollector_ConcurrentCollect measures performance under concurrent collections
func BenchmarkCollector_ConcurrentCollect(b *testing.B) {
	mockP := &MockProvider{}
	providers.Register("mock_cloud_concurrent", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return mockP
	})

	accounts := make([]config.CloudAccount, 5)
	for i := 0; i < 5; i++ {
		accounts[i] = config.CloudAccount{
			Provider:        "mock_cloud_concurrent",
			AccountID:       "concurrent-account-" + string(rune(i)),
			AccessKeyID:     "ak",
			AccessKeySecret: "sk",
			Regions:         []string{"cn-mock-1", "cn-mock-2"},
		}
	}

	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"mock_cloud_concurrent": accounts,
		},
		Server: &config.ServerConf{
			RegionConcurrency:  2,
			MetricConcurrency:  3,
			ProductConcurrency: 2,
		},
	}
	mgr := discovery.NewManager(cfg)

	c := NewCollector(cfg, mgr)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Collect()
		}
	})
}
