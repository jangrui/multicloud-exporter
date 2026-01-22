package huawei

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"multicloud-exporter/internal/config"
	providerscommon "multicloud-exporter/internal/providers/common"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"", 0},
		{"1s", 1 * time.Second},
		{"1m", 1 * time.Minute},
		{"1h", 1 * time.Hour},
		{"100ms", 100 * time.Millisecond},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollector_SupportsInternalSharding(t *testing.T) {
	c := &Collector{}
	assert.True(t, c.SupportsInternalSharding())
}

func TestCollector_SetDegradationManager(t *testing.T) {
	c := &Collector{}
	mgr := &providerscommon.Manager{}

	c.SetDegradationManager(mgr)

	assert.Equal(t, mgr, c.degradeMgr)
}

func TestCacheKey(t *testing.T) {
	c := &Collector{}
	account := config.CloudAccount{
		AccountID: "test-account",
	}

	result := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	expected := "test-account|cn-north-4|SYS.ELB|elb"
	assert.Equal(t, expected, result)
}

func TestSetCachedIDs(t *testing.T) {
	c := &Collector{
		resCache: make(map[string]resCacheEntry),
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}
	ids := []string{"id1", "id2"}

	c.setCachedIDs(account, "cn-north-4", "SYS.ELB", "elb", ids)

	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	entry, ok := c.resCache[key]
	assert.True(t, ok)
	assert.Equal(t, ids, entry.IDs)
}

func TestSetCachedIDs_EmptyIDs(t *testing.T) {
	c := &Collector{
		resCache: make(map[string]resCacheEntry),
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}

	c.setCachedIDs(account, "cn-north-4", "SYS.ELB", "elb", []string{})

	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	_, ok := c.resCache[key]
	assert.False(t, ok, "空 ID 列表不应缓存")
}

func TestGetCachedIDs_Miss(t *testing.T) {
	c := &Collector{
		resCache: make(map[string]resCacheEntry),
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}

	ids, hit := c.getCachedIDs(account, "cn-north-4", "SYS.ELB", "elb")
	assert.Nil(t, ids)
	assert.False(t, hit)
}

func TestGetCachedIDs_Hit(t *testing.T) {
	c := &Collector{
		resCache: make(map[string]resCacheEntry),
		cfg:      &config.Config{},
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}
	ids := []string{"id1", "id2"}
	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	c.resCache[key] = resCacheEntry{
		IDs:       ids,
		UpdatedAt: time.Now(),
	}

	cachedIDs, hit := c.getCachedIDs(account, "cn-north-4", "SYS.ELB", "elb")
	assert.True(t, hit)
	assert.Equal(t, ids, cachedIDs)
}

func TestGetCachedIDs_Expired(t *testing.T) {
	c := &Collector{
		resCache: make(map[string]resCacheEntry),
		cfg:      &config.Config{},
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}
	ids := []string{"id1", "id2"}
	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	c.resCache[key] = resCacheEntry{
		IDs:       ids,
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}

	cachedIDs, hit := c.getCachedIDs(account, "cn-north-4", "SYS.ELB", "elb")
	assert.False(t, hit)
	assert.Nil(t, cachedIDs)
}

func TestIsOBSCapacityMetric(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		expected   bool
	}{
		{"容量指标1", "capacity_byte", true},
		{"容量指标2", "capacity_used_byte", true},
		{"对象数量指标1", "object_num_total", true},
		{"对象数量指标2", "object_num_appendable", true},
		{"非容量指标", "traffic_byte", false},
		{"请求速率指标", "api_request_count_per_second", false},
		{"空字符串", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOBSCapacityMetric(tt.metricName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectRegion(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}

	tests := []struct {
		name      string
		resources []string
	}{
		{"无资源", []string{}},
		{"所有资源", []string{"*"}},
		{"仅ELB", []string{"elb"}},
		{"仅OBS", []string{"obs"}},
		{"CLB别名", []string{"clb"}},
		{"S3别名", []string{"s3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account.Resources = tt.resources
			c.collectRegion(account, "cn-north-4")
		})
	}
}

func TestCollect(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}
	account := config.CloudAccount{
		AccountID: "test-account",
	}

	tests := []struct {
		name    string
		regions []string
	}{
		{"默认区域", []string{}},
		{"通配符区域", []string{"*"}},
		{"指定区域", []string{"cn-north-4", "cn-south-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account.Regions = tt.regions
			regions := account.Regions
			if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
				regions = []string{"cn-north-4"} // Mock default region
			}
			for _, region := range regions {
				c.collectRegion(account, region)
			}
		})
	}
}
