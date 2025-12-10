package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type dynD struct{}

func (d *dynD) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	var has bool
	for _, xs := range cfg.AccountsByProvider {
		for _, a := range xs {
			for _, r := range a.Resources {
				if r == "slb" {
					has = true
				}
			}
		}
	}
	if has {
		return []config.Product{{Namespace: "acs_slb_dashboard"}}
	}
	return nil
}

func TestWatchAccountsTrigger(t *testing.T) {
	Register("dyn", &dynD{})
	dir := t.TempDir()
	p := filepath.Join(dir, "accounts.yaml")
	if err := os.WriteFile(p, []byte("accounts:\n  aliyun:\n    - provider: aliyun\n      resources: [bwp]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Setenv("ACCOUNTS_PATH", p)
	cfg := &config.Config{}
	m := NewManager(cfg)
	m.watchInterval = 100 * time.Millisecond // Reduce interval for test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set initial file mod time in manager before starting watcher
	// This ensures the first check in watcher loop sees the initial state
	// and subsequent file write triggers a change.
	if _, err := os.Stat(p); err == nil {
		// We manually set a past time to ensure the next write is seen as newer
		// even if it happens very quickly. However, os.Chtimes might not be supported
		// or have enough precision on all FS.
		// Instead, we rely on manager reading the file on Start.
	}

	m.Start(ctx)
	ch := m.Subscribe()

	// Wait for the watcher to perform at least one check and stabilize
	time.Sleep(200 * time.Millisecond)

	// Write new content
	if err := os.WriteFile(p, []byte("accounts:\n  aliyun:\n    - provider: aliyun\n      resources: [bwp, slb]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Force update mod time to be definitely different if FS resolution is low
	now := time.Now()
	_ = os.Chtimes(p, now, now)

	select {
	case <-ch:
	case <-time.After(2 * time.Second): // Increase timeout relative to interval
		t.Fatalf("no refresh signal")
	}
}
