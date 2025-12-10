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
	m.Start(ctx)
	ch := m.Subscribe()
	time.Sleep(200 * time.Millisecond) // Wait for watcher to start
	if err := os.WriteFile(p, []byte("accounts:\n  aliyun:\n    - provider: aliyun\n      resources: [bwp, slb]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second): // Increase timeout relative to interval
		t.Fatalf("no refresh signal")
	}
}
