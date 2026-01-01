package huawei

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"multicloud-exporter/internal/config"
	providerscommon "multicloud-exporter/internal/providers/common"
)

func TestNewCollector(t *testing.T) {
	cfg := &config.Config{}

	c := NewCollector(cfg, nil)

	assert.NotNil(t, c)
	assert.Equal(t, cfg, c.cfg)
	assert.NotNil(t, c.resCache)
	assert.NotNil(t, c.clientFactory)
}

func TestCollector_GetDefaultResources(t *testing.T) {
	c := &Collector{}
	resources := c.GetDefaultResources()

	assert.Contains(t, resources, "clb")
	assert.Contains(t, resources, "s3")
	assert.Len(t, resources, 2)
}

func TestCollector_CacheOperations(t *testing.T) {
	cfg := &config.Config{
		Server: &config.ServerConf{
			DiscoveryTTL: "1h",
		},
	}
	c := NewCollector(cfg, nil)

	account := config.CloudAccount{
		AccountID: "test-account",
	}
	region := "cn-north-4"
	namespace := "SYS.ELB"
	rtype := "elb"

	// 测试缓存未命中
	ids, hit := c.getCachedIDs(account, region, namespace, rtype)
	assert.False(t, hit)
	assert.Nil(t, ids)

	// 设置缓存
	testIDs := []string{"lb-001", "lb-002", "lb-003"}
	c.setCachedIDs(account, region, namespace, rtype, testIDs)

	// 测试缓存命中
	ids, hit = c.getCachedIDs(account, region, namespace, rtype)
	assert.True(t, hit)
	assert.Equal(t, testIDs, ids)
}

func TestCollector_CacheExpiry(t *testing.T) {
	cfg := &config.Config{
		Server: &config.ServerConf{
			DiscoveryTTL: "1ms", // 极短的 TTL 用于测试过期
		},
	}
	c := NewCollector(cfg, nil)

	account := config.CloudAccount{
		AccountID: "test-account",
	}
	region := "cn-north-4"
	namespace := "SYS.ELB"
	rtype := "elb"

	// 设置缓存
	testIDs := []string{"lb-001"}
	c.setCachedIDs(account, region, namespace, rtype, testIDs)

	// 等待缓存过期
	time.Sleep(5 * time.Millisecond)

	// 测试缓存已过期
	ids, hit := c.getCachedIDs(account, region, namespace, rtype)
	assert.False(t, hit)
	assert.Nil(t, ids)
}

func TestResCacheEntry(t *testing.T) {
	now := time.Now()
	entry := resCacheEntry{
		IDs:       []string{"id1", "id2"},
		UpdatedAt: now,
	}

	assert.Len(t, entry.IDs, 2)
	assert.Contains(t, entry.IDs, "id1")
	assert.Contains(t, entry.IDs, "id2")
	assert.False(t, entry.UpdatedAt.IsZero())
	assert.WithinDuration(t, now, entry.UpdatedAt, time.Second)
}

func TestCollector_CollectWithEmptyAccount(t *testing.T) {
	cfg := &config.Config{}
	c := NewCollector(cfg, nil)

	// 测试空账号不会 panic
	account := config.CloudAccount{}
	require.NotPanics(t, func() {
		c.Collect(account)
	})
}

func TestHuaweiErrorClassifier(t *testing.T) {
	classifier := &providerscommon.HuaweiErrorClassifier{}

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, providerscommon.ErrorStatusUnknown},
		{"auth error 401", &mockError{"401 Unauthorized"}, providerscommon.ErrorStatusAuth},
		{"AK/SK error", &mockError{"AK/SK verification failed"}, providerscommon.ErrorStatusAuth},
		{"throttling error", &mockError{"Request throttling"}, providerscommon.ErrorStatusLimit},
		{"429 error", &mockError{"429 TooManyRequests"}, providerscommon.ErrorStatusLimit},
		{"region error", &mockError{"Invalid region"}, providerscommon.ErrorStatusRegion},
		{"timeout error", &mockError{"Connection timeout"}, providerscommon.ErrorStatusNetwork},
		{"unknown error", &mockError{"Some random error"}, providerscommon.ErrorStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifyHuaweiErrorFunc(t *testing.T) {
	result := providerscommon.ClassifyHuaweiError(nil)
	assert.Equal(t, providerscommon.ErrorStatusUnknown, result)

	result = providerscommon.ClassifyHuaweiError(&mockError{"Authenticate failed"})
	assert.Equal(t, providerscommon.ErrorStatusAuth, result)
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func TestDefaultHuaweiRegions(t *testing.T) {
	// 验证默认区域列表不为空
	assert.NotEmpty(t, defaultHuaweiRegions)
	// 验证包含常用区域
	assert.Contains(t, defaultHuaweiRegions, "cn-north-4")
	assert.Contains(t, defaultHuaweiRegions, "cn-south-1")
}

func TestCollector_CacheKey(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	account := config.CloudAccount{AccountID: "acc-123"}

	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	assert.Equal(t, "acc-123|cn-north-4|SYS.ELB|elb", key)
}
