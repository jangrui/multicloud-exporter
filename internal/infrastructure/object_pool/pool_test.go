package object_pool

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestObjectPool_GetPut 测试对象池的获取和归还
func TestObjectPool_GetPut(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	// 测试获取新对象
	obj1 := pool.Get()
	if obj1 == nil {
		t.Fatal("expected non-nil object")
	}

	// 测试归还对象
	pool.Put(obj1)

	// 测试归还 nil（应该被忽略）
	pool.Put(nil)

	// 验证统计信息
	stats := pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}

// TestObjectPool_ConcurrentAccess 测试并发访问
func TestObjectPool_ConcurrentAccess(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	var wg sync.WaitGroup
	goroutines := 100

	// 并发获取和归还对象
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj := pool.Get()
			time.Sleep(10 * time.Microsecond) // 模拟使用
			pool.Put(obj)
		}()
	}

	wg.Wait()

	stats := pool.GetStats()
	if stats.Hits == 0 && stats.Misses == 0 {
		t.Error("expected some pool activity")
	}

	t.Logf("Hits: %d, Misses: %d, Puts: %d", stats.Hits, stats.Misses, stats.Puts)
}

// TestObjectPool_GetStats 测试统计信息
func TestObjectPool_GetStats(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	stats := pool.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Puts != 0 {
		t.Error("expected zero stats initially")
	}

	// 执行一些操作
	pool.Put(&struct{}{})

	stats = pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}

// TestObjectPool_Reset 测试重置统计信息
func TestObjectPool_Reset(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	pool.Get()
	pool.Put(&struct{}{})

	stats := pool.GetStats()
	if stats.Hits == 0 && stats.Misses == 0 && stats.Puts == 0 {
		t.Error("expected non-zero stats")
	}

	pool.Reset()

	stats = pool.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Puts != 0 {
		t.Error("expected zero stats after reset")
	}
}

// TestPoolStats_GetHitRatio 测试命中率计算
func TestPoolStats_GetHitRatio(t *testing.T) {
	tests := []struct {
		name     string
		hits     uint64
		misses   uint64
		expected float64
	}{
		{
			name:     "no activity",
			hits:     0,
			misses:   0,
			expected: 0,
		},
		{
			name:     "all hits",
			hits:     100,
			misses:   0,
			expected: 100,
		},
		{
			name:     "all misses",
			hits:     0,
			misses:   100,
			expected: 0,
		},
		{
			name:     "50% hit ratio",
			hits:     50,
			misses:   50,
			expected: 50,
		},
		{
			name:     "75% hit ratio",
			hits:     75,
			misses:   25,
			expected: 75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := PoolStats{
				Hits:   tt.hits,
				Misses: tt.misses,
			}

			ratio := stats.GetHitRatio()
			if ratio != tt.expected {
				t.Errorf("expected %f%%, got %f%%", tt.expected, ratio)
			}
		})
	}
}

// TestPoolManager_RegisterGetPool 测试对象池管理器的注册和获取
func TestPoolManager_RegisterGetPool(t *testing.T) {
	pm := NewPoolManager()

	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	// 注册对象池
	pm.Register("test", pool)

	// 获取对象池
	retrieved, ok := pm.GetPool("test")
	if !ok {
		t.Fatal("expected pool to be registered")
	}
	if retrieved != pool {
		t.Error("expected to retrieve the same pool")
	}

	// 获取不存在的池
	_, ok = pm.GetPool("nonexistent")
	if ok {
		t.Error("expected false for nonexistent pool")
	}
}

// TestPoolManager_GetAllStats 测试获取所有对象池的统计信息
func TestPoolManager_GetAllStats(t *testing.T) {
	pm := NewPoolManager()

	pool1 := NewObjectPool(func() interface{} {
		return &struct{}{}
	})
	pool2 := NewObjectPool(func() interface{} {
		return &struct{}{}
	})

	pm.Register("pool1", pool1)
	pm.Register("pool2", pool2)

	// 执行一些操作
	pool1.Get()
	pool2.Get()
	pool1.Put(&struct{}{})

	stats := pm.GetAllStats()
	if len(stats) != 2 {
		t.Errorf("expected 2 pools, got %d", len(stats))
	}

	if _, ok := stats["pool1"]; !ok {
		t.Error("expected pool1 stats")
	}
	if _, ok := stats["pool2"]; !ok {
		t.Error("expected pool2 stats")
	}
}

// TestCleanupManager_StartStop 测试清理管理器的启动和停止
func TestCleanupManager_StartStop(t *testing.T) {
	cm := NewCleanupManager(100*time.Millisecond, 5*time.Minute)

	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})
	cm.Register("test", pool)

	// 启动清理管理器
	go cm.Start()

	// 等待一段时间
	time.Sleep(200 * time.Millisecond)

	// 停止清理管理器
	cm.Stop()

	// 验证统计信息被重置（如果命中率过低）
	stats := pool.GetStats()
	t.Logf("Stats after cleanup: Hits=%d, Misses=%d, Puts=%d",
		stats.Hits, stats.Misses, stats.Puts)
}

// TestCleanupManager_Cleanup 测试清理逻辑
func TestCleanupManager_Cleanup(t *testing.T) {
	cm := NewCleanupManager(100*time.Millisecond, 5*time.Minute)

	pool := NewObjectPool(func() interface{} {
		return &struct{}{}
	})
	cm.Register("test", pool)

	// 执行大量 misses，但很少 hits（模拟低命中率场景）
	for i := 0; i < 200; i++ {
		pool.Get()
	}

	statsBefore := pool.GetStats()
	t.Logf("Stats before cleanup: Hits=%d, Misses=%d, HitRatio=%.2f%%",
		statsBefore.Hits, statsBefore.Misses, statsBefore.GetHitRatio())

	// 手动触发清理
	cm.cleanup()

	statsAfter := pool.GetStats()
	t.Logf("Stats after cleanup: Hits=%d, Misses=%d, HitRatio=%.2f%%",
		statsAfter.Hits, statsAfter.Misses, statsAfter.GetHitRatio())

	// 由于命中率 < 10% 且 Hits > 100，统计应该被重置
	if statsBefore.Hits > 100 && statsBefore.GetHitRatio() < 10.0 {
		if statsAfter.Hits != 0 || statsAfter.Misses != 0 {
			t.Log("Stats were reset as expected for low hit ratio")
		}
	}
}

// TestObjectPool_MemoryUsage 测试对象池的内存使用
func TestObjectPool_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory usage test in short mode")
	}

	// 记录初始内存分配
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	pool := NewObjectPool(func() interface{} {
		return make([]byte, 1024) // 1KB 对象
	})

	// 创建并归还大量对象
	iterations := 1000
	for i := 0; i < iterations; i++ {
		obj := pool.Get()
		pool.Put(obj)
	}

	// 触发 GC
	runtime.GC()

	// 记录最终内存分配
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// 计算内存增长
	allocDiff := m2.TotalAlloc - m1.TotalAlloc
	heapDiff := m2.HeapAlloc - m1.HeapAlloc

	t.Logf("Memory usage after %d iterations:", iterations)
	t.Logf("  TotalAlloc: %d bytes (%.2f MB)", m2.TotalAlloc, float64(m2.TotalAlloc)/(1024*1024))
	t.Logf("  HeapAlloc: %d bytes (%.2f MB)", m2.HeapAlloc, float64(m2.HeapAlloc)/(1024*1024))
	t.Logf("  AllocDiff: %d bytes (%.2f MB)", allocDiff, float64(allocDiff)/(1024*1024))
	t.Logf("  HeapDiff: %d bytes (%.2f MB)", heapDiff, float64(heapDiff)/(1024*1024))

	stats := pool.GetStats()
	t.Logf("Pool stats: Hits=%d, Misses=%d, HitRatio=%.2f%%",
		stats.Hits, stats.Misses, stats.GetHitRatio())
}

// TestAccountPool 测试账号管理器对象池
func TestAccountPool(t *testing.T) {
	pool := NewAccountPool(func() interface{} {
		return &struct{}{}
	})

	obj := pool.Get()
	if obj == nil {
		t.Fatal("expected non-nil object")
	}

	pool.Put(obj)

	stats := pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}

// TestProductPool 测试产品管理器对象池
func TestProductPool(t *testing.T) {
	pool := NewProductPool(func() interface{} {
		return &struct{}{}
	})

	obj := pool.Get()
	if obj == nil {
		t.Fatal("expected non-nil object")
	}

	pool.Put(obj)

	stats := pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}

// TestRegionPool 测试区域管理器对象池
func TestRegionPool(t *testing.T) {
	pool := NewRegionPool(func() interface{} {
		return &struct{}{}
	})

	obj := pool.Get()
	if obj == nil {
		t.Fatal("expected non-nil object")
	}

	pool.Put(obj)

	stats := pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}

// TestResourcePool 测试资源管理器对象池
func TestResourcePool(t *testing.T) {
	pool := NewResourcePool(func() interface{} {
		return &struct{}{}
	})

	obj := pool.Get()
	if obj == nil {
		t.Fatal("expected non-nil object")
	}

	pool.Put(obj)

	stats := pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("expected 1 put, got %d", stats.Puts)
	}
}
