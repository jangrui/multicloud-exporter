package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"testing"
)

type testD struct{ prods []config.Product }

func (d *testD) Discover(ctx context.Context, cfg *config.Config) []config.Product { return d.prods }

func TestManagerRefreshVersion(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{NoSavepoint: true}}
	Register("t", &testD{prods: []config.Product{{Namespace: "n"}}})
	m := NewManager(cfg)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh err: %v", err)
	}
	v1 := m.Version()
	Register("t2", &testD{prods: []config.Product{{Namespace: "n2"}}})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh2 err: %v", err)
	}
	v2 := m.Version()
	if v2 <= v1 {
		t.Fatalf("version not increased")
	}
}

func TestAccountsSignature(t *testing.T) {
	cfg := &config.Config{}
	cfg.AccountsByProvider = map[string][]config.CloudAccount{
		"aliyun": []config.CloudAccount{{Resources: []string{"ecs", "bwp"}}},
	}
	m := NewManager(cfg)
	s := m.accountsSignature()
	if s == "" {
		t.Fatalf("empty signature")
	}
}
