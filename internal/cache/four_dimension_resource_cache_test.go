package cache

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestFourDimensionResourceCache_New(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)

	if cache == nil {
		t.Fatal("cache should not be nil")
	}

	if cache.GetSize() != 0 {
		t.Errorf("expected size 0, got %d", cache.GetSize())
	}

	cache.Stop()
}

func TestFourDimensionResourceCache_GetSet(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	accountID := "account1"
	productID := "product1"
	regionID := "region1"
	resources := []string{"resource1", "resource2", "resource3"}

	// 缓存未命中
	result, found := cache.GetResourceIDs(accountID, productID, regionID)
	if found {
		t.Error("expected not found")
	}
	if result != nil {
		t.Error("expected nil result")
	}

	// 设置缓存
	cache.SetResourceIDs(accountID, productID, regionID, resources)

	// 缓存命中
	result, found = cache.GetResourceIDs(accountID, productID, regionID)
	if !found {
		t.Error("expected found")
	}
	if len(result) != 3 {
		t.Errorf("expected 3 resources, got %d", len(result))
	}
	if result[0] != "resource1" {
		t.Errorf("expected resource1, got %s", result[0])
	}
}

func TestFourDimensionResourceCache_TTL(t *testing.T) {
	logger := zap.NewNop()
	config := ResourceCacheConfig{
		TTL:             100 * time.Millisecond,
		MaxEntries:      5000,
		CleanupInterval: 5 * time.Minute,
	}
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	accountID := "account1"
	productID := "product1"
	regionID := "region1"
	resources := []string{"resource1", "resource2"}

	// 设置缓存
	cache.SetResourceIDs(accountID, productID, regionID, resources)

	// 立即查询，应该命中
	_, found := cache.GetResourceIDs(accountID, productID, regionID)
	if !found {
		t.Error("expected cache hit immediately after set")
	}

	// 检查是否过期（未过期）
	expired := cache.IsCacheExpired(accountID, productID, regionID)
	if expired {
		t.Error("expected cache not expired")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 查询，应该未命中
	result, found := cache.GetResourceIDs(accountID, productID, regionID)
	if found {
		t.Error("expected cache miss after TTL expired")
	}
	if result != nil {
		t.Error("expected nil result after TTL expired")
	}

	// 检查是否过期（已过期）
	expired = cache.IsCacheExpired(accountID, productID, regionID)
	if !expired {
		t.Error("expected cache expired")
	}
}

func TestFourDimensionResourceCache_Invalidate(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	accountID := "account1"
	productID := "product1"
	regionID := "region1"
	resources := []string{"resource1"}

	// 设置缓存
	cache.SetResourceIDs(accountID, productID, regionID, resources)

	// 验证缓存存在
	if cache.GetSize() != 1 {
		t.Errorf("expected size 1, got %d", cache.GetSize())
	}

	// 使缓存失效
	cache.Invalidate(accountID, productID, regionID)

	// 验证缓存已删除
	if cache.GetSize() != 0 {
		t.Errorf("expected size 0, got %d", cache.GetSize())
	}

	// 验证查询未命中
	_, found := cache.GetResourceIDs(accountID, productID, regionID)
	if found {
		t.Error("expected not found after invalidation")
	}
}

func TestFourDimensionResourceCache_InvalidateByAccount(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	resources := []string{"resource1"}

	// 设置多个账号的缓存
	cache.SetResourceIDs("account1", "product1", "region1", resources)
	cache.SetResourceIDs("account1", "product1", "region2", resources)
	cache.SetResourceIDs("account2", "product1", "region1", resources)

	// 验证缓存数量
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 使 account1 下所有缓存失效
	count := cache.InvalidateByAccount("account1")

	// 验证删除数量
	if count != 2 {
		t.Errorf("expected 2 entries deleted, got %d", count)
	}

	// 验证剩余缓存数量
	if cache.GetSize() != 1 {
		t.Errorf("expected size 1, got %d", cache.GetSize())
	}

	// 验证 account2 的缓存仍然存在
	_, found := cache.GetResourceIDs("account2", "product1", "region1")
	if !found {
		t.Error("expected account2 cache to exist")
	}
}

func TestFourDimensionResourceCache_InvalidateByProduct(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	resources := []string{"resource1"}

	// 设置多个产品的缓存
	cache.SetResourceIDs("account1", "product1", "region1", resources)
	cache.SetResourceIDs("account1", "product1", "region2", resources)
	cache.SetResourceIDs("account1", "product2", "region1", resources)

	// 验证缓存数量
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 使 product1 下所有缓存失效
	count := cache.InvalidateByProduct("account1", "product1")

	// 验证删除数量
	if count != 2 {
		t.Errorf("expected 2 entries deleted, got %d", count)
	}

	// 验证剩余缓存数量
	if cache.GetSize() != 1 {
		t.Errorf("expected size 1, got %d", cache.GetSize())
	}

	// 验证 product2 的缓存仍然存在
	_, found := cache.GetResourceIDs("account1", "product2", "region1")
	if !found {
		t.Error("expected product2 cache to exist")
	}
}

func TestFourDimensionResourceCache_InvalidateByRegion(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	resources := []string{"resource1"}

	// 设置多个区域的缓存
	cache.SetResourceIDs("account1", "product1", "region1", resources)
	cache.SetResourceIDs("account1", "product2", "region1", resources)
	cache.SetResourceIDs("account1", "product1", "region2", resources)

	// 验证缓存数量
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 使 region1 下所有缓存失效
	count := cache.InvalidateByRegion("account1", "product1", "region1")

	// 验证删除数量
	if count != 1 {
		t.Errorf("expected 1 entry deleted, got %d", count)
	}

	// 验证剩余缓存数量
	if cache.GetSize() != 2 {
		t.Errorf("expected size 2, got %d", cache.GetSize())
	}

	// 验证 region2 的缓存仍然存在
	_, found := cache.GetResourceIDs("account1", "product1", "region2")
	if !found {
		t.Error("expected region2 cache to exist")
	}
}

func TestFourDimensionResourceCache_Clear(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	resources := []string{"resource1"}

	// 设置多个缓存
	cache.SetResourceIDs("account1", "product1", "region1", resources)
	cache.SetResourceIDs("account2", "product1", "region1", resources)
	cache.SetResourceIDs("account3", "product1", "region1", resources)

	// 验证缓存数量
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 清空所有缓存
	cache.Clear()

	// 验证缓存已清空
	if cache.GetSize() != 0 {
		t.Errorf("expected size 0, got %d", cache.GetSize())
	}
}

func TestFourDimensionResourceCache_GetStats(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	// 设置缓存
	cache.SetResourceIDs("account1", "product1", "region1", []string{"r1", "r2", "r3"})
	cache.SetResourceIDs("account1", "product1", "region2", []string{"r4", "r5"})

	// 访问缓存多次以增加访问计数
	// 注意：SetResourceIDs 会初始化 accessCount=1
	for i := 0; i < 5; i++ {
		cache.GetResourceIDs("account1", "product1", "region1")
	}

	// 获取统计信息
	stats := cache.GetStats()

	// 验证统计信息
	if stats.TotalEntries != 2 {
		t.Errorf("expected 2 total entries, got %d", stats.TotalEntries)
	}
	if stats.TotalResources != 5 {
		t.Errorf("expected 5 total resources, got %d", stats.TotalResources)
	}
	// 总访问计数 = 条目1 (1 initial + 5 gets) + 条目2 (1 initial) = 7
	if stats.TotalAccessCount != 7 {
		t.Errorf("expected 7 total access count, got %d", stats.TotalAccessCount)
	}
}

func TestFourDimensionResourceCache_Concurrent(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	accountID := "account1"
	productID := "product1"
	regionID := "region1"

	// 并发写入
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(id int) {
			resources := []string{string(rune('a' + id)), string(rune('b' + id)), string(rune('c' + id))}
			cache.SetResourceIDs(accountID, productID, regionID, resources)
			done <- true
		}(i)
	}

	// 等待所有写入完成
	for i := 0; i < 100; i++ {
		<-done
	}

	// 验证缓存存在
	result, found := cache.GetResourceIDs(accountID, productID, regionID)
	if !found {
		t.Error("expected cache hit")
	}
	if len(result) != 3 {
		t.Errorf("expected 3 resources, got %d", len(result))
	}

	// 并发读取
	for i := 0; i < 100; i++ {
		go func() {
			cache.GetResourceIDs(accountID, productID, regionID)
			done <- true
		}()
	}

	// 等待所有读取完成
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestFourDimensionResourceCache_LRU(t *testing.T) {
	logger := zap.NewNop()
	config := ResourceCacheConfig{
		TTL:             30 * time.Minute,
		MaxEntries:      3,
		CleanupInterval: 5 * time.Minute,
	}
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	resources := []string{"resource1"}

	// 设置缓存（最大条目数为 3）
	cache.SetResourceIDs("account1", "product1", "region1", resources)
	cache.SetResourceIDs("account2", "product1", "region1", resources)
	cache.SetResourceIDs("account3", "product1", "region1", resources)

	// 验证缓存数量
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 访问前两个缓存以增加访问计数
	cache.GetResourceIDs("account1", "product1", "region1")
	cache.GetResourceIDs("account2", "product1", "region1")

	// 添加第 4 个缓存（应该触发 LRU 驱逐）
	cache.SetResourceIDs("account4", "product1", "region1", resources)

	// 验证缓存数量仍然是 3
	if cache.GetSize() != 3 {
		t.Errorf("expected size 3, got %d", cache.GetSize())
	}

	// 验证 account3 的缓存被驱逐（访问次数最少）
	_, found := cache.GetResourceIDs("account3", "product1", "region1")
	if found {
		t.Error("expected account3 cache to be evicted")
	}

	// 验证其他缓存仍然存在
	if _, found := cache.GetResourceIDs("account1", "product1", "region1"); !found {
		t.Error("expected account1 cache to exist")
	}
	if _, found := cache.GetResourceIDs("account2", "product1", "region1"); !found {
		t.Error("expected account2 cache to exist")
	}
	if _, found := cache.GetResourceIDs("account4", "product1", "region1"); !found {
		t.Error("expected account4 cache to exist")
	}
}

func TestFourDimensionResourceCache_GenerateCacheKey(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	key := cache.GenerateCacheKey("account1", "product1", "region1")
	expected := "account1:product1:region1"

	if key != expected {
		t.Errorf("expected key %s, got %s", expected, key)
	}
}

func BenchmarkFourDimensionResourceCache_GetSet(b *testing.B) {
	logger := zap.NewNop()
	config := DefaultResourceCacheConfig()
	cache := NewFourDimensionResourceCache(logger, config)
	defer cache.Stop()

	accountID := "account1"
	productID := "product1"
	regionID := "region1"
	resources := []string{"resource1", "resource2", "resource3"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetResourceIDs(accountID, productID, regionID, resources)
		cache.GetResourceIDs(accountID, productID, regionID)
	}
}
