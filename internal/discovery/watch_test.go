package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDiscoverer is a simple discoverer for testing purposes.
// It returns a product if the "slb" resource is present in the config.
type mockDiscoverer struct{}

func (d *mockDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	for _, xs := range cfg.AccountsByProvider {
		for _, a := range xs {
			for _, r := range a.Resources {
				if r == "slb" {
					return []config.Product{{Namespace: "mock_namespace"}}
				}
			}
		}
	}
	return nil
}

func TestManager_Watch(t *testing.T) {
	// 1. Setup
	// Use a unique name to avoid conflicts if tests run in parallel
	mockName := "mock_watch"
	Register(mockName, &mockDiscoverer{})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "accounts.yaml")

	// helper to update config file and force mtime update
	writeConfig := func(content string) {
		err := os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		// Force mtime forward to ensure fsnotify/polling catches it
		// even if it happens within the same tick.
		now := time.Now().Add(time.Second)
		err = os.Chtimes(configPath, now, now)
		require.NoError(t, err)
	}

	// Initial config: only "bwp", mock discoverer returns nil
	writeConfig(`accounts:
  aliyun:
    - provider: aliyun
      resources: [bwp]
`)

	t.Setenv("ACCOUNTS_PATH", configPath)

	cfg := &config.Config{}
	m := NewManager(cfg)
	// Set a fast polling interval for testing
	m.watchInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := m.Subscribe()

	// 2. Start Manager
	m.Start(ctx)

	// Allow some time for initial load from file (triggered by watchAccounts)
	// and drain any pending events so we start with a clean slate.
	time.Sleep(200 * time.Millisecond)
	select {
	case <-ch:
	default:
	}

	// Verify initial state
	// Should not contain mockName because "slb" is not in resources yet
	require.NotContains(t, m.Get(), mockName)

	// 3. Test Cases
	t.Run("DetectConfigChange", func(t *testing.T) {
		// Update config to include "slb", triggering discovery
		writeConfig(`accounts:
  aliyun:
    - provider: aliyun
      resources: [bwp, slb]
`)

		// Wait for update signal
		select {
		case <-ch:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for config refresh")
		}

		// Verify update happened
		products := m.Get()
		require.Contains(t, products, mockName)
		require.Equal(t, "mock_namespace", products[mockName][0].Namespace)
		require.Greater(t, m.Version(), int64(0))
	})

	t.Run("IgnoreSameContent", func(t *testing.T) {
		currentVersion := m.Version()

		// Write same content
		writeConfig(`accounts:
  aliyun:
    - provider: aliyun
      resources: [bwp, slb]
`)

		select {
		case <-ch:
			t.Fatal("Should not receive signal for identical config")
		case <-time.After(200 * time.Millisecond):
			// Expected timeout (no event)
		}

		assert.Equal(t, currentVersion, m.Version())
	})

	t.Run("HandleInvalidConfig", func(t *testing.T) {
		currentVersion := m.Version()

		// Write invalid YAML
		writeConfig(`invalid_yaml: [unclosed_bracket`)

		select {
		case <-ch:
			t.Fatal("Should not receive signal for invalid config")
		case <-time.After(200 * time.Millisecond):
			// Expected timeout (no event)
		}

		assert.Equal(t, currentVersion, m.Version())
	})
}
