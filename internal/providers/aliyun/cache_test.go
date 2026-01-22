package aliyun

import (
	"testing"
	"time"

	"multicloud-exporter/internal/config"
)

func TestCacheTTL(t *testing.T) {
	cfg := &config.Config{Server: &config.ServerConf{DiscoveryTTL: "1s"}}
	a := NewCollector(cfg, nil, nil)
	acc := config.CloudAccount{AccountID: "a"}
	a.setCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs", []string{"i-1"}, map[string]interface{}{"i-1": nil})
	ids, _, ok := a.getCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs")
	if !ok || len(ids) != 1 {
		t.Fatalf("hit")
	}
	time.Sleep(1200 * time.Millisecond)
	_, _, ok = a.getCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs")
	if ok {
		t.Fatalf("expired")
	}
}

func TestCacheEmptyResult(t *testing.T) {
	cfg := &config.Config{}
	a := NewCollector(cfg, nil, nil)
	acc := config.CloudAccount{AccountID: "test"}

	// 第一次调用：尝试缓存空结果
	a.setCachedIDs(acc, "cn-test", "acs_alb", "alb", []string{}, map[string]interface{}{})

	// 第二次调用：应该缓存未命中（因为空结果不会被缓存）
	ids, _, ok := a.getCachedIDs(acc, "cn-test", "acs_alb", "alb")
	if ok {
		t.Fatalf("空结果不应该被缓存")
	}
	if len(ids) != 0 {
		t.Fatalf("空结果应该返回空列表")
	}
}
