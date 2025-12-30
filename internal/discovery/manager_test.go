package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"testing"
)

type testD struct{ prods []config.Product }

func (d *testD) Discover(ctx context.Context, cfg *config.Config) []config.Product { return d.prods }

func TestManagerRefreshVersion(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{}}
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
		"aliyun": {{Resources: []string{"ecs", "bwp"}}},
	}
	m := NewManager(cfg)
	s := m.accountsSignature()
	if s == "" {
		t.Fatalf("empty signature")
	}
}

func TestManagerSubscription(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{}}
	m := NewManager(cfg)
	ch := m.Subscribe()
	if ch == nil {
		t.Fatal("subscribe returned nil")
	}
	m.Unsubscribe(ch)
}

func TestManagerUpdatedAt(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{}}
	m := NewManager(cfg)
	// Initially zero or near zero?
	_ = m.UpdatedAt()

	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh err: %v", err)
	}
	ts := m.UpdatedAt()
	if ts.IsZero() {
		t.Fatal("updatedAt is zero after refresh")
	}
}

// Benchmark tests for performance measurement

// BenchmarkManager_Refresh measures the performance of discovery refresh
func BenchmarkManager_Refresh(b *testing.B) {
	// Create a configuration with multiple discoverers
	cfg := &config.Config{
		Server: &config.ServerConf{},
	}

	// Register multiple test discoverers
	prods1 := make([]config.Product, 10)
	for i := 0; i < 10; i++ {
		prods1[i] = config.Product{
			Namespace: "namespace-" + string(rune(i)),
		}
	}
	Register("test1", &testD{prods: prods1})

	prods2 := make([]config.Product, 5)
	for i := 0; i < 5; i++ {
		prods2[i] = config.Product{
			Namespace: "namespace-" + string(rune(i+10)),
		}
	}
	Register("test2", &testD{prods: prods2})

	m := NewManager(cfg)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := m.Refresh(context.Background()); err != nil {
			b.Fatalf("refresh err: %v", err)
		}
	}
}

// BenchmarkManager_Get measures the performance of retrieving products
func BenchmarkManager_Get(b *testing.B) {
	cfg := &config.Config{Server: &config.ServerConf{}}

	// Register discoverers
	prods := make([]config.Product, 20)
	for i := 0; i < 20; i++ {
		prods[i] = config.Product{
			Namespace: "bench-ns-" + string(rune(i)),
		}
	}
	Register("bench", &testD{prods: prods})

	m := NewManager(cfg)

	// Initialize with products
	if err := m.Refresh(context.Background()); err != nil {
		b.Fatalf("refresh err: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = m.Get()
	}
}

// BenchmarkManager_Subscribe measures the performance of subscription operations
func BenchmarkManager_Subscribe(b *testing.B) {
	cfg := &config.Config{Server: &config.ServerConf{}}
	m := NewManager(cfg)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := m.Subscribe()
		if ch == nil {
			b.Fatal("subscribe returned nil")
		}
		m.Unsubscribe(ch)
	}
}

// BenchmarkManager_Version measures the performance of version retrieval
func BenchmarkManager_Version(b *testing.B) {
	cfg := &config.Config{Server: &config.ServerConf{}}
	prods := []config.Product{
		{Namespace: "ns1"},
		{Namespace: "ns2"},
	}
	Register("version_test", &testD{prods: prods})

	m := NewManager(cfg)
	if err := m.Refresh(context.Background()); err != nil {
		b.Fatalf("refresh err: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = m.Version()
	}
}
