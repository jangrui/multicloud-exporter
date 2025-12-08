package aliyun

import (
    "testing"
    "time"
    "multicloud-exporter/internal/config"
)

func TestCacheTTL(t *testing.T) {
    cfg := &config.Config{ServerConf: &config.ServerConf{DiscoveryTTL: "1s"}}
    a := NewCollector(cfg, nil)
    acc := config.CloudAccount{AccountID: "a"}
    a.setCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs", []string{"i-1"}, map[string]interface{}{"i-1": nil})
    ids, _, ok := a.getCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs")
    if !ok || len(ids) != 1 { t.Fatalf("hit") }
    time.Sleep(1200 * time.Millisecond)
    _, _, ok = a.getCachedIDs(acc, "cn", "acs_ecs_dashboard", "ecs")
    if ok { t.Fatalf("expired") }
}
