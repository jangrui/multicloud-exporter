package discovery

import (
	"context"
	"strconv"
	"testing"
	"time"

	"multicloud-exporter/internal/config"
)

func TestManagerRefreshAndNotify(t *testing.T) {
	aliyunDiscoverFn = func(ctx context.Context, cfg *config.Config) []config.Product {
		return []config.Product{{Namespace: "acs_ecs_dashboard", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"CPUUtilization"}}}}}
	}
	tencentDiscoverFn = func(ctx context.Context, cfg *config.Config) []config.Product {
		return []config.Product{{Namespace: "QCE/BWP", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"InTraffic"}}}}}
	}
	cfg := &config.Config{AccountsList: []config.CloudAccount{{Provider: "aliyun", AccountID: "a1", AccessKeyID: "x", AccessKeySecret: "y", Regions: []string{"cn-hangzhou"}, Resources: []string{"ecs"}}}}
	m := NewManager(cfg)
	ch := m.Subscribe()
	defer m.Unsubscribe(ch)
	ctx := context.Background()
	_ = m.Refresh(ctx)
	got := m.Get()
	if len(got["aliyun"]) == 0 || got["aliyun"][0].Namespace != "acs_ecs_dashboard" {
		t.Fatalf("unexpected aliyun products")
	}
	aliyunDiscoverFn = func(ctx context.Context, cfg *config.Config) []config.Product { return []config.Product{} }
	_ = m.Refresh(ctx)
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no notification")
	}
}

func BenchmarkManagerRefresh(b *testing.B) {
	big := make([]string, 0, 20000)
	for i := 0; i < 20000; i++ {
		big = append(big, "m"+strconv.Itoa(i))
	}
	aliyunDiscoverFn = func(ctx context.Context, cfg *config.Config) []config.Product {
		return []config.Product{{Namespace: "acs_ecs_dashboard", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: big}}}}
	}
	tencentDiscoverFn = func(ctx context.Context, cfg *config.Config) []config.Product { return []config.Product{} }
	cfg := &config.Config{}
	m := NewManager(cfg)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Refresh(ctx)
	}
}
