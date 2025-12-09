package tencent

import (
	"multicloud-exporter/internal/config"
	"testing"
	"time"
)

type eStr string

func (e eStr) Error() string { return string(e) }

func TestClassifyTencentError(t *testing.T) {
	if classifyTencentError(eStr("AuthFailure")) != "auth_error" {
		t.Fatalf("auth")
	}
	if classifyTencentError(eStr("RequestLimitExceeded")) != "limit_error" {
		t.Fatalf("limit")
	}
	if classifyTencentError(eStr("timeout")) != "network_error" {
		t.Fatalf("net")
	}
}

func TestTencentCacheTTL(t *testing.T) {
	cfg := &config.Config{ServerConf: &config.ServerConf{DiscoveryTTL: "1s"}}
	c := NewCollector(cfg, nil)
	acc := config.CloudAccount{AccountID: "a"}
	c.setCachedIDs(acc, "ap", "QCE/BWP", "bwp", []string{"b-1"})
	if ids, ok := c.getCachedIDs(acc, "ap", "QCE/BWP", "bwp"); !ok || len(ids) != 1 {
		t.Fatalf("hit")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, ok := c.getCachedIDs(acc, "ap", "QCE/BWP", "bwp"); ok {
		t.Fatalf("expired")
	}
}

func TestTencentDefaultResources(t *testing.T) {
	c := NewCollector(nil, nil)
	resources := c.GetDefaultResources()
	hasCOS := false
	for _, r := range resources {
		if r == "cos" {
			hasCOS = true
			break
		}
	}
	if !hasCOS {
		t.Fatalf("default resources should contain cos")
	}
}
