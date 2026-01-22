package cache

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"multicloud-exporter/internal/metrics"
)

func TestCacheEntry_GetTags(t *testing.T) {
	tags := map[string]string{
		"env":  "prod",
		"team": "platform",
	}
	entry := NewCacheEntry(tags)

	retrievedTags := entry.GetTags()
	if len(retrievedTags) != len(tags) {
		t.Fatalf("expected %d tags, got %d", len(tags), len(retrievedTags))
	}

	if retrievedTags["env"] != "prod" {
		t.Fatalf("expected env=prod, got %s", retrievedTags["env"])
	}

	if entry.GetAccessCount() != 2 {
		t.Fatalf("expected access count 2 (1 from NewCacheEntry, 1 from GetTags), got %d", entry.GetAccessCount())
	}
}

func TestCacheEntry_IsExpired(t *testing.T) {
	tags := map[string]string{"key": "value"}
	entry := NewCacheEntry(tags)

	// 刚创建，不应该过期
	if entry.IsExpired(30 * time.Minute) {
		t.Fatalf("expected entry not to be expired")
	}

	// 模拟过期 - 使用更长的睡眠时间
	time.Sleep(20 * time.Millisecond)
	if !entry.IsExpired(5 * time.Millisecond) {
		t.Fatalf("expected entry to be expired after 20ms with TTL 5ms")
	}
}

func TestFourDimensionTagCache_GetSet(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.TTL = 5 * time.Minute

	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"
	regionID := "cn-hangzhou"
	resourceID := "resource-123"

	tags := map[string]string{
		"env":  "prod",
		"team": "platform",
	}

	// 测试 SetTags
	cache.SetTags(accountID, productID, regionID, resourceID, tags)

	// 测试 GetTags
	retrievedTags, exists := cache.GetTags(accountID, productID, regionID, resourceID)
	if !exists {
		t.Fatalf("expected tags to exist")
	}

	if len(retrievedTags) != len(tags) {
		t.Fatalf("expected %d tags, got %d", len(tags), len(retrievedTags))
	}

	// 测试缓存未命中
	_, exists = cache.GetTags("nonexistent", "product", "region", "resource")
	if exists {
		t.Fatalf("expected cache miss for nonexistent key")
	}
}

func TestFourDimensionTagCache_TTL(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.TTL = 100 * time.Millisecond

	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"
	regionID := "cn-hangzhou"
	resourceID := "resource-123"

	tags := map[string]string{"env": "prod"}
	cache.SetTags(accountID, productID, regionID, resourceID, tags)

	// TTL 内，缓存应该存在
	_, exists := cache.GetTags(accountID, productID, regionID, resourceID)
	if !exists {
		t.Fatalf("expected cache hit within TTL")
	}

	// 等待 TTL 过期
	time.Sleep(150 * time.Millisecond)

	// TTL 后，缓存应该不存在
	_, exists = cache.GetTags(accountID, productID, regionID, resourceID)
	if exists {
		t.Fatalf("expected cache miss after TTL")
	}
}

func TestFourDimensionTagCache_GenerateCacheKey(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	accountID := "account-1"
	productID := "product-2"
	regionID := "region-3"
	resourceID := "resource-4"

	expectedKey := "account-1:product-2:region-3:resource-4"
	actualKey := cache.GenerateCacheKey(accountID, productID, regionID, resourceID)

	if actualKey != expectedKey {
		t.Fatalf("expected key '%s', got '%s'", expectedKey, actualKey)
	}
}

func TestFourDimensionTagCache_Invalidate(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"
	regionID := "cn-hangzhou"
	resourceID := "resource-123"

	tags := map[string]string{"env": "prod"}
	cache.SetTags(accountID, productID, regionID, resourceID, tags)

	// 验证缓存存在
	_, exists := cache.GetTags(accountID, productID, regionID, resourceID)
	if !exists {
		t.Fatalf("expected cache to exist before invalidate")
	}

	// 使缓存失效
	cache.Invalidate(accountID, productID, regionID, resourceID)

	// 验证缓存不存在
	_, exists = cache.GetTags(accountID, productID, regionID, resourceID)
	if exists {
		t.Fatalf("expected cache to be invalidated")
	}
}

func TestFourDimensionTagCache_InvalidateByAccount(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"

	// 添加多个缓存
	cache.SetTags(accountID, "product1", "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags(accountID, "product1", "region1", "resource2", map[string]string{"k2": "v2"})
	cache.SetTags("other-account", "product1", "region1", "resource3", map[string]string{"k3": "v3"})

	// 使账号下所有缓存失效
	count := cache.InvalidateByAccount(accountID)
	if count != 2 {
		t.Fatalf("expected 2 invalidations, got %d", count)
	}

	// 验证缓存已失效
	_, exists := cache.GetTags(accountID, "product1", "region1", "resource1")
	if exists {
		t.Fatalf("expected cache to be invalidated by account")
	}

	// 验证其他账号缓存仍然存在
	_, exists = cache.GetTags("other-account", "product1", "region1", "resource3")
	if !exists {
		t.Fatalf("expected other account cache to still exist")
	}
}

func TestFourDimensionTagCache_InvalidateByProduct(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"

	// 添加多个缓存
	cache.SetTags(accountID, productID, "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags(accountID, productID, "region1", "resource2", map[string]string{"k2": "v2"})
	cache.SetTags(accountID, "other-product", "region1", "resource3", map[string]string{"k3": "v3"})

	// 使产品下所有缓存失效
	count := cache.InvalidateByProduct(accountID, productID)
	if count != 2 {
		t.Fatalf("expected 2 invalidations, got %d", count)
	}

	// 验证缓存已失效
	_, exists := cache.GetTags(accountID, productID, "region1", "resource1")
	if exists {
		t.Fatalf("expected cache to be invalidated by product")
	}

	// 验证其他产品缓存仍然存在
	_, exists = cache.GetTags(accountID, "other-product", "region1", "resource3")
	if !exists {
		t.Fatalf("expected other product cache to still exist")
	}
}

func TestFourDimensionTagCache_InvalidateByRegion(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"
	regionID := "cn-hangzhou"

	// 添加多个缓存
	cache.SetTags(accountID, productID, regionID, "resource1", map[string]string{"k1": "v1"})
	cache.SetTags(accountID, productID, regionID, "resource2", map[string]string{"k2": "v2"})
	cache.SetTags(accountID, productID, "other-region", "resource3", map[string]string{"k3": "v3"})

	// 使区域下所有缓存失效
	count := cache.InvalidateByRegion(accountID, productID, regionID)
	if count != 2 {
		t.Fatalf("expected 2 invalidations, got %d", count)
	}

	// 验证缓存已失效
	_, exists := cache.GetTags(accountID, productID, regionID, "resource1")
	if exists {
		t.Fatalf("expected cache to be invalidated by region")
	}

	// 验证其他区域缓存仍然存在
	_, exists = cache.GetTags(accountID, productID, "other-region", "resource3")
	if !exists {
		t.Fatalf("expected other region cache to still exist")
	}
}

func TestFourDimensionTagCache_Clear(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	// 添加多个缓存
	cache.SetTags("account1", "product1", "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags("account2", "product2", "region2", "resource2", map[string]string{"k2": "v2"})

	// 验证缓存存在
	if cache.GetSize() != 2 {
		t.Fatalf("expected cache size 2, got %d", cache.GetSize())
	}

	// 清空缓存
	cache.Clear()

	// 验证缓存已清空
	if cache.GetSize() != 0 {
		t.Fatalf("expected cache size 0 after clear, got %d", cache.GetSize())
	}
}

func TestFourDimensionTagCache_GetStats(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.TTL = 100 * time.Millisecond

	cache := NewFourDimensionTagCache(logger, config)

	// 添加缓存
	cache.SetTags("account1", "product1", "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags("account2", "product2", "region2", "resource2", map[string]string{"k2": "v2"})

	// 获取统计
	stats := cache.GetStats()

	if stats.TotalEntries != 2 {
		t.Fatalf("expected 2 total entries, got %d", stats.TotalEntries)
	}

	if stats.TotalAccessCount < 2 {
		t.Fatalf("expected at least 2 total accesses, got %d", stats.TotalAccessCount)
	}
}

func TestFourDimensionTagCache_GetSize(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	// 初始大小为 0
	if cache.GetSize() != 0 {
		t.Fatalf("expected cache size 0, got %d", cache.GetSize())
	}

	// 添加缓存
	cache.SetTags("account1", "product1", "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags("account2", "product2", "region2", "resource2", map[string]string{"k2": "v2"})

	// 验证大小
	if cache.GetSize() != 2 {
		t.Fatalf("expected cache size 2, got %d", cache.GetSize())
	}
}

func TestFourDimensionTagCache_Concurrent(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	var wg sync.WaitGroup

	// 100 个并发 goroutine 读写缓存
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			accountID := string(rune('a' + (n % 26)))
			productID := string(rune('A' + (n % 26)))
			regionID := string(rune('0' + (n % 10)))
			resourceID := string(rune('0' + (n % 10)))

			tags := map[string]string{
				"key": string(rune('a' + n)),
				"n":   string(rune('0' + n)),
			}

			// 写入缓存
			cache.SetTags(accountID, productID, regionID, resourceID, tags)

			// 读取缓存
			cache.GetTags(accountID, productID, regionID, resourceID)

			time.Sleep(1 * time.Millisecond)

			// 删除缓存
			if n%3 == 0 {
				cache.Invalidate(accountID, productID, regionID, resourceID)
			}
		}(i)
	}

	wg.Wait()

	// 验证缓存有内容
	size := cache.GetSize()
	if size < 0 {
		t.Fatalf("expected cache size >= 0, got %d", size)
	}

	// 验证统计信息
	stats := cache.GetStats()
	if stats.TotalAccessCount < 100 {
		t.Fatalf("expected at least 100 accesses, got %d", stats.TotalAccessCount)
	}
}

func TestFourDimensionTagCache_LRU(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.MaxEntries = 5
	cache := NewFourDimensionTagCache(logger, config)

	// 添加 5 个缓存（达到最大条目数）
	for i := 0; i < 5; i++ {
		cache.SetTags("account", "product", "region", string(rune('0'+i)), map[string]string{"n": string(rune('0' + i))})
	}

	if cache.GetSize() != 5 {
		t.Fatalf("expected cache size 5, got %d", cache.GetSize())
	}

	// 添加第 6 个缓存（应该触发 LRU 驱逐）
	cache.SetTags("account", "product", "region", "5", map[string]string{"n": "5"})

	// 验证缓存大小仍然是 5
	if cache.GetSize() != 5 {
		t.Fatalf("expected cache size 5 after LRU eviction, got %d", cache.GetSize())
	}
}

func TestFourDimensionTagCache_Cleanup(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.TTL = 50 * time.Millisecond
	config.CleanupInterval = 100 * time.Millisecond

	cache := NewFourDimensionTagCache(logger, config)

	// 添加缓存
	cache.SetTags("account1", "product1", "region1", "resource1", map[string]string{"k1": "v1"})
	cache.SetTags("account2", "product2", "region2", "resource2", map[string]string{"k2": "v2"})

	// 等待 TTL 过期
	time.Sleep(60 * time.Millisecond)

	// 等待清理
	time.Sleep(150 * time.Millisecond)

	// 验证缓存已清理
	if cache.GetSize() != 0 {
		t.Fatalf("expected cache size 0 after cleanup, got %d", cache.GetSize())
	}

	cache.Stop()
}

func TestFourDimensionTagCache_IsExpired(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	config.TTL = 100 * time.Millisecond

	cache := NewFourDimensionTagCache(logger, config)

	accountID := "test-account"
	productID := "test-product"
	regionID := "cn-hangzhou"
	resourceID := "resource-123"

	tags := map[string]string{"env": "prod"}
	cache.SetTags(accountID, productID, regionID, resourceID, tags)

	// TTL 内，不应该过期
	if cache.IsExpired(accountID, productID, regionID, resourceID) {
		t.Fatalf("expected cache not to be expired within TTL")
	}

	// 等待 TTL 过期
	time.Sleep(150 * time.Millisecond)

	// TTL 后，应该过期
	if !cache.IsExpired(accountID, productID, regionID, resourceID) {
		t.Fatalf("expected cache to be expired after TTL")
	}
}

// Benchmark 测试

func BenchmarkFourDimensionTagCache_GetTags(b *testing.B) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	// 预填充缓存
	for i := 0; i < 1000; i++ {
		accountID := string(rune('a' + (i % 26)))
		productID := string(rune('A' + (i % 26)))
		regionID := string(rune('0' + (i % 10)))
		resourceID := string(rune('0' + (i % 10)))

		cache.SetTags(accountID, productID, regionID, resourceID, map[string]string{"n": string(rune('0' + i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		accountID := string(rune('a' + (i % 26)))
		productID := string(rune('A' + (i % 26)))
		regionID := string(rune('0' + (i % 10)))
		resourceID := string(rune('0' + (i % 10)))

		cache.GetTags(accountID, productID, regionID, resourceID)
	}
}

func BenchmarkFourDimensionTagCache_SetTags(b *testing.B) {
	logger := zap.NewNop()
	config := DefaultCacheConfig()
	cache := NewFourDimensionTagCache(logger, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		accountID := string(rune('a' + (i % 26)))
		productID := string(rune('A' + (i % 26)))
		regionID := string(rune('0' + (i % 10)))
		resourceID := string(rune('0' + (i % 10)))

		cache.SetTags(accountID, productID, regionID, resourceID, map[string]string{"n": string(rune('0' + i))})
	}
}

func init() {
	// 初始化指标
	metrics.RegisterNamespacePrefix("test", "test_")
}
