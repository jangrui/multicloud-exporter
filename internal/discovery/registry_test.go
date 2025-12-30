package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"testing"
)

type dummyD struct{}

func (d *dummyD) Discover(ctx context.Context, cfg *config.Config) []config.Product { return nil }

func TestRegistry(t *testing.T) {
	Register("x", &dummyD{})
	if _, ok := GetDiscoverer("x"); !ok {
		t.Fatalf("missing discoverer")
	}
	xs := GetAllDiscoverers()
	found := false
	for _, k := range xs {
		if k == "x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("not listed")
	}
}
