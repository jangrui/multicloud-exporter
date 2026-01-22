package lru_cleanup

import (
	"sync"
	"testing"
	"time"
)

// TestGlobalLRUCache_TrackAccess 测试追踪访问
func TestGlobalLRUCache_TrackAccess(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	// 追踪访问
	cache.TrackAccess("key1", "value1")
	cache.TrackAccess("key2", "value2")

	// 验证获取
	value, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}

	stats := cache.GetStats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}
}

// TestGlobalLRUCache_GetAndUpdate 测试获取并更新访问时间
func TestGlobalLRUCache_GetAndUpdate(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	// 添加条目
	cache.TrackAccess("key1", "value1")

	// 获取并更新
	value, ok := cache.GetAndUpdate("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}

	// 验证被移动到前面（LRU）
	key, _, _ := cache.GetLeastRecentlyUsed()
	if key != "key1" {
		t.Errorf("expected key1 to be most recently used, got %s", key)
	}
}

// TestGlobalLRUCache_GetLeastRecentlyUsed 测试获取最少使用的条目
func TestGlobalLRUCache_GetLeastRecentlyUsed(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	cache.TrackAccess("key1", "value1")
	cache.TrackAccess("key2", "value2")

	// key1 应该是最少使用的（因为 key2 后添加，key1 被移到后面）
	key, value, ok := cache.GetLeastRecentlyUsed()
	if !ok {
		t.Fatal("expected least recently used entry")
	}
	if key != "key1" {
		t.Errorf("expected key1, got %s", key)
	}
	if value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}
}

// TestGlobalLRUCache_Evict 测试驱除指定条目
func TestGlobalLRUCache_Evict(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	cache.TrackAccess("key1", "value1")
	cache.TrackAccess("key2", "value2")

	// 驱逐 key1
	if !cache.Evict("key1") {
		t.Error("expected key1 to be evicted")
	}

	// 验证 key1 已被驱除
	if _, ok := cache.Get("key1"); ok {
		t.Error("expected key1 to be removed")
	}

	// 验证 key2 仍然存在
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to exist")
	}

	stats := cache.GetStats()
	if stats.EvictedTotal != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.EvictedTotal)
	}
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
}

// TestGlobalLRUCache_CapacityLimit 测试容量限制
func TestGlobalLRUCache_CapacityLimit(t *testing.T) {
	cache := NewGlobalLRUCache(3, time.Hour)

	// 添加 5 个条目（超过容量 3）
	for i := 1; i <= 5; i++ {
		cache.TrackAccess(string(rune('0'+i)), i)
	}

	stats := cache.GetStats()
	if stats.Size != 3 {
		t.Errorf("expected size 3, got %d", stats.Size)
	}

	if stats.EvictedTotal != 2 {
		t.Errorf("expected 2 evictions, got %d", stats.EvictedTotal)
	}

	// 验证最旧的 2 个条目被驱除
	if _, ok := cache.Get("1"); ok {
		t.Error("expected key1 to be evicted")
	}
	if _, ok := cache.Get("2"); ok {
		t.Error("expected key2 to be evicted")
	}

	// 验证最新的 3 个条目仍然存在
	for i := 3; i <= 5; i++ {
		if _, ok := cache.Get(string(rune('0' + i))); !ok {
			t.Errorf("expected key%d to exist", i)
		}
	}
}

// TestGlobalLRUCache_CleanupTTL 测试 TTL 清理
func TestGlobalLRUCache_CleanupTTL(t *testing.T) {
	cache := NewGlobalLRUCache(100, 100*time.Millisecond)

	// 添加条目
	cache.TrackAccess("key1", "value1")
	cache.TrackAccess("key2", "value2")

	stats := cache.GetStats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}

	// 等待 TTL 过期
	time.Sleep(150 * time.Millisecond)

	// 启动清理循环并等待
	stopChan := make(chan struct{})
	go cache.CleanupLoop(50*time.Millisecond, stopChan)
	defer close(stopChan)

	// 等待清理完成
	time.Sleep(200 * time.Millisecond)

	// 验证旧条目被清理
	if _, ok := cache.Get("key1"); ok {
		t.Error("expected key1 to be evicted by TTL")
	}
	if _, ok := cache.Get("key2"); ok {
		t.Error("expected key2 to be evicted by TTL")
	}

	stats = cache.GetStats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
	if stats.CleanupTotal == 0 {
		t.Error("expected cleanup to be triggered")
	}
}

// TestGlobalLRUCache_CleanupLoop 测试清理循环
func TestGlobalLRUCache_CleanupLoop(t *testing.T) {
	cache := NewGlobalLRUCache(100, 50*time.Millisecond)

	// 添加条目
	for i := 1; i <= 5; i++ {
		cache.TrackAccess(string(rune('0'+i)), i)
	}

	stopChan := make(chan struct{})
	go cache.CleanupLoop(100*time.Millisecond, stopChan)

	// 等待清理
	time.Sleep(200 * time.Millisecond)

	// 停止清理循环
	close(stopChan)

	stats := cache.GetStats()
	t.Logf("Stats: Size=%d, EvictedTotal=%d, CleanupTotal=%d",
		stats.Size, stats.EvictedTotal, stats.CleanupTotal)
}

// TestGlobalLRUCache_ConcurrentAccess 测试并发访问
func TestGlobalLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	var wg sync.WaitGroup
	goroutines := 100

	// 并发访问
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := string(rune('0' + (id % 10)))
			cache.TrackAccess(key, id)
			cache.GetAndUpdate(key)
		}(i)
	}

	wg.Wait()

	stats := cache.GetStats()
	t.Logf("Concurrent access stats: Size=%d, EvictedTotal=%d",
		stats.Size, stats.EvictedTotal)

	if stats.Size == 0 {
		t.Error("expected some entries after concurrent access")
	}
}

// TestGlobalLRUCache_GetStats 测试获取统计信息
func TestGlobalLRUCache_GetStats(t *testing.T) {
	cache := NewGlobalLRUCache(10, time.Hour)

	// 初始统计
	stats := cache.GetStats()
	if stats.Size != 0 || stats.EvictedTotal != 0 || stats.CleanupTotal != 0 {
		t.Error("expected zero stats initially")
	}
	if stats.Capacity != 10 {
		t.Errorf("expected capacity 10, got %d", stats.Capacity)
	}

	// 添加条目
	cache.TrackAccess("key1", "value1")
	cache.TrackAccess("key2", "value2")

	stats = cache.GetStats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}

	// 驱逐条目
	cache.Evict("key1")

	stats = cache.GetStats()
	if stats.EvictedTotal != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.EvictedTotal)
	}
}

// TestGlobalLRUCache_EvictNonExistent 测试驱除不存在的条目
func TestGlobalLRUCache_EvictNonExistent(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	// 尝试驱除不存在的条目
	if cache.Evict("nonexistent") {
		t.Error("expected false for nonexistent key")
	}

	stats := cache.GetStats()
	if stats.EvictedTotal != 0 {
		t.Errorf("expected 0 evictions, got %d", stats.EvictedTotal)
	}
}

// TestGlobalLRUCache_UpdateExisting 测试更新现有条目
func TestGlobalLRUCache_UpdateExisting(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	// 添加条目
	cache.TrackAccess("key1", "value1")

	// 更新条目
	cache.TrackAccess("key1", "value2")

	value, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if value != "value2" {
		t.Errorf("expected value2, got %v", value)
	}

	stats := cache.GetStats()
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
}

// TestGlobalLRUCache_MultipleUpdates 测试多次更新
func TestGlobalLRUCache_MultipleUpdates(t *testing.T) {
	cache := NewGlobalLRUCache(100, time.Hour)

	// 添加条目
	cache.TrackAccess("key1", "value1")

	// 多次更新
	for i := 2; i <= 10; i++ {
		cache.TrackAccess("key1", i)
	}

	value, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if value != 10 {
		t.Errorf("expected 10, got %v", value)
	}

	stats := cache.GetStats()
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
}

// TestGlobalLRUCache_LRUOrder 测试 LRU 顺序
func TestGlobalLRUCache_LRUOrder(t *testing.T) {
	cache := NewGlobalLRUCache(10, time.Hour)

	// 添加条目
	cache.TrackAccess("key1", 1)
	cache.TrackAccess("key2", 2)
	cache.TrackAccess("key3", 3)

	// 访问 key1（应该移动到前面）
	cache.GetAndUpdate("key1")

	// key2 应该是最少使用的
	key, _, _ := cache.GetLeastRecentlyUsed()
	if key != "key2" {
		t.Errorf("expected key2 to be least recently used, got %s", key)
	}

	// 再次访问 key3
	cache.GetAndUpdate("key3")

	// key2 仍然应该是最少使用的
	key, _, _ = cache.GetLeastRecentlyUsed()
	if key != "key2" {
		t.Errorf("expected key2 to be least recently used, got %s", key)
	}
}

// TestGlobalLRUCache_CapacityZero 测试容量为 0
func TestGlobalLRUCache_CapacityZero(t *testing.T) {
	cache := NewGlobalLRUCache(0, time.Hour)

	// 添加条目（应该被立即驱除）
	cache.TrackAccess("key1", "value1")

	stats := cache.GetStats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
	if stats.EvictedTotal != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.EvictedTotal)
	}
}
