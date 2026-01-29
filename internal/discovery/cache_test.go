package discovery

import (
	"testing"
	"time"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
)

func newTestDiscoveryMetaCache(t *testing.T, ttl time.Duration) *discoveryMetaCache {
	t.Helper()
	return &discoveryMetaCache{
		ttl:     ttl,
		entries: make(map[string]discoveryMetaCacheEntry),
	}
}

func TestDiscoveryMetaCache_SetGet(t *testing.T) {
	cache := newTestDiscoveryMetaCache(t, time.Hour)
	account := config.CloudAccount{Provider: "tencent", AccountID: "acc-1"}
	metricGroups := []config.MetricGroup{
		{MetricList: []string{"MetricA", "MetricB"}},
	}

	cache.Set("tencent", "QCE/LB", account, metricGroups)
	got, ok := cache.Get("tencent", "QCE/LB", account)

	assert.True(t, ok)
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].MetricList, "MetricA")
	assert.Contains(t, got[0].MetricList, "MetricB")
}

func TestDiscoveryMetaCache_TTLExpired(t *testing.T) {
	cache := newTestDiscoveryMetaCache(t, time.Second)
	account := config.CloudAccount{Provider: "huawei", AccountID: "acc-2"}
	key := buildDiscoveryCacheKey("huawei", "SYS.ELB", account.AccountID)
	cache.entries[key] = discoveryMetaCacheEntry{
		Provider:     "huawei",
		Namespace:    "SYS.ELB",
		AccountID:    account.AccountID,
		MetricGroups: []config.MetricGroup{{MetricList: []string{"MetricX"}}},
		Fingerprint:  buildAccountFingerprint(account),
		UpdatedAt:    time.Now().Add(-2 * time.Second),
	}

	_, ok := cache.Get("huawei", "SYS.ELB", account)
	assert.False(t, ok)
}

func TestDiscoveryMetaCache_FingerprintMismatch(t *testing.T) {
	cache := newTestDiscoveryMetaCache(t, time.Hour)
	account := config.CloudAccount{Provider: "aliyun", AccountID: "acc-3"}
	key := buildDiscoveryCacheKey("aliyun", "acs_oss_dashboard", account.AccountID)
	cache.entries[key] = discoveryMetaCacheEntry{
		Provider:     "aliyun",
		Namespace:    "acs_oss_dashboard",
		AccountID:    account.AccountID,
		MetricGroups: []config.MetricGroup{{MetricList: []string{"MetricY"}}},
		Fingerprint:  "invalid",
		UpdatedAt:    time.Now(),
	}

	_, ok := cache.Get("aliyun", "acs_oss_dashboard", account)
	assert.False(t, ok)
}
