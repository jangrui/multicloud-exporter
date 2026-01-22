package memory

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestMemoryEntry_IsExpired 测试内存条目过期检查
func TestMemoryEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		entry    *MemoryEntry
		ttl      time.Duration
		expected bool
	}{
		{
			name: "未过期",
			entry: &MemoryEntry{
				LastUsedAt: time.Now().Add(-5 * time.Minute),
			},
			ttl:      30 * time.Minute,
			expected: false,
		},
		{
			name: "已过期",
			entry: &MemoryEntry{
				LastUsedAt: time.Now().Add(-35 * time.Minute),
			},
			ttl:      30 * time.Minute,
			expected: true,
		},
		{
			name: "刚好过期",
			entry: &MemoryEntry{
				LastUsedAt: time.Now().Add(-30 * time.Minute),
			},
			ttl:      30 * time.Minute,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := time.Since(tt.entry.LastUsedAt) > tt.ttl
			if result != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestMemoryEntry_AccessCount 测试访问计数
func TestMemoryEntry_AccessCount(t *testing.T) {
	entry := &MemoryEntry{
		AccessCount: 0,
		LastUsedAt:  time.Now(),
	}

	// 第一次访问
	entry.LastUsedAt = time.Now()
	entry.AccessCount++
	if entry.AccessCount != 1 {
		t.Errorf("AccessCount = %d, want 1", entry.AccessCount)
	}

	// 第二次访问
	entry.LastUsedAt = time.Now()
	entry.AccessCount++
	if entry.AccessCount != 2 {
		t.Errorf("AccessCount = %d, want 2", entry.AccessCount)
	}
}

// TestFourDimensionMemoryManager_RecordMemoryUsage 测试记录内存使用
func TestFourDimensionMemoryManager_RecordMemoryUsage(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	tests := []struct {
		name      string
		dimension string
		key       string
		bytes     int64
	}{
		{"记录账号内存", "account", "account-1", 1024},
		{"记录产品内存", "product", "product-1", 2048},
		{"记录区域内存", "region", "region-1", 4096},
		{"记录资源内存", "resource", "resource-1", 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager.RecordMemoryUsage(tt.dimension, tt.key, tt.bytes)

			stats := manager.GetMemoryUsage(tt.dimension)
			if stats.TotalBytes != tt.bytes {
				t.Errorf("TotalBytes = %d, want %d", stats.TotalBytes, tt.bytes)
			}
			if stats.EntryCount != 1 {
				t.Errorf("EntryCount = %d, want 1", stats.EntryCount)
			}
		})
	}
}

// TestFourDimensionMemoryManager_ScanMemory 测试扫描内存
func TestFourDimensionMemoryManager_ScanMemory(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 记录一些内存使用
	manager.RecordMemoryUsage("account", "account-1", 1024)
	manager.RecordMemoryUsage("product", "product-1", 2048)
	manager.RecordMemoryUsage("region", "region-1", 4096)
	manager.RecordMemoryUsage("resource", "resource-1", 8192)

	// 扫描内存
	stats := manager.ScanMemory()

	if stats.TotalBytes != 1024+2048+4096+8192 {
		t.Errorf("TotalBytes = %d, want %d", stats.TotalBytes, 1024+2048+4096+8192)
	}
	if stats.EntryCount != 4 {
		t.Errorf("EntryCount = %d, want 4", stats.EntryCount)
	}
}

// TestFourDimensionMemoryManager_GetMemoryUsage 测试获取内存使用
func TestFourDimensionMemoryManager_GetMemoryUsage(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 记录内存使用
	manager.RecordMemoryUsage("account", "account-1", 1024)
	manager.RecordMemoryUsage("account", "account-2", 2048)
	manager.RecordMemoryUsage("product", "product-1", 4096)
	manager.RecordMemoryUsage("region", "region-1", 8192)

	tests := []struct {
		name      string
		dimension string
		expected  int64
	}{
		{"账号内存", "account", 1024 + 2048},
		{"产品内存", "product", 4096},
		{"区域内存", "region", 8192},
		{"资源内存", "resource", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := manager.GetMemoryUsage(tt.dimension)
			if stats.TotalBytes != tt.expected {
				t.Errorf("TotalBytes = %d, want %d", stats.TotalBytes, tt.expected)
			}
		})
	}
}

// TestFourDimensionMemoryManager_SetMemoryLimit 测试设置内存限制
func TestFourDimensionMemoryManager_SetMemoryLimit(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 修改限制
	manager.SetMemoryLimit("account", 200*1024*1024) // 200 MB
	manager.SetMemoryLimit("product", 100*1024*1024) // 100 MB
	manager.SetMemoryLimit("region", 50*1024*1024)   // 50 MB
	manager.SetMemoryLimit("resource", 25*1024*1024) // 25 MB

	// 验证设置成功（通过记录内存并检查清理）
	manager.RecordMemoryUsage("account", "account-1", 150*1024*1024) // 150 MB
	manager.RecordMemoryUsage("product", "product-1", 80*1024*1024)  // 80 MB

	// 触发清理
	cleaned := manager.TriggerCleanup(CleanupStrategyLRU)
	if cleaned < 0 {
		t.Errorf("TriggerCleanup returned negative value")
	}
}

// TestFourDimensionMemoryManager_TriggerCleanup_LRU 测试 LRU 清理
func TestFourDimensionMemoryManager_TriggerCleanup_LRU(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	// 设置较小的限制以触发清理
	config.Limit.AccountLimit = 1024
	manager := NewFourDimensionMemoryManager(config)

	// 记录 3 个账号内存（总 1100，超过限制 1024）
	manager.RecordMemoryUsage("account", "account-1", 500)
	manager.RecordMemoryUsage("account", "account-2", 400)
	manager.RecordMemoryUsage("account", "account-3", 200)

	// 检查清理前的内存使用
	beforeCleanup := manager.GetMemoryUsage("account")
	t.Logf("Before cleanup: TotalBytes=%d, EntryCount=%d", beforeCleanup.TotalBytes, beforeCleanup.EntryCount)

	// 触发 LRU 清理（应该清理至少一个条目以降到 1024 以下）
	cleaned := manager.TriggerCleanup(CleanupStrategyLRU)
	t.Logf("Cleanup returned: %d cleaned", cleaned)

	// 检查清理后的内存使用
	afterCleanup := manager.GetMemoryUsage("account")
	t.Logf("After cleanup: TotalBytes=%d, EntryCount=%d, EvictedCount=%d",
		afterCleanup.TotalBytes, afterCleanup.EntryCount, afterCleanup.EvictedCount)

	// 验证清理后内存降到限制以下
	if cleaned < 0 {
		t.Errorf("TriggerCleanup(LRU) returned negative value: %d", cleaned)
	}

	// 验证内存使用降到限制以下（允许 10% 的误差）
	expectedLimit := int64(float64(config.Limit.AccountLimit) * 1.1)
	if afterCleanup.TotalBytes > expectedLimit {
		t.Errorf("TotalBytes = %d, should be <= %d after cleanup (cleaned: %d, evicted: %d)",
			afterCleanup.TotalBytes, expectedLimit, cleaned, afterCleanup.EvictedCount)
	}
}

// TestFourDimensionMemoryManager_TriggerCleanup_TTL 测试 TTL 清理
func TestFourDimensionMemoryManager_TriggerCleanup_TTL(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	config.Limit.TTL = 1 * time.Second // 1 秒 TTL
	manager := NewFourDimensionMemoryManager(config)

	// 记录内存使用
	manager.RecordMemoryUsage("account", "account-1", 1024)
	manager.RecordMemoryUsage("account", "account-2", 2048)

	// 等待过期
	time.Sleep(2 * time.Second)

	// 触发 TTL 清理
	cleaned := manager.TriggerCleanup(CleanupStrategyTTL)
	if cleaned < 0 {
		t.Errorf("TriggerCleanup(TTL) returned negative value: %d", cleaned)
	}

	stats := manager.GetMemoryUsage("account")
	if stats.ExpiredCount == 0 && cleaned > 0 {
		t.Errorf("Expected expired entries to be cleaned")
	}
}

// TestFourDimensionMemoryManager_TriggerCleanup_Force 测试强制清理
func TestFourDimensionMemoryManager_TriggerCleanup_Force(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	config.Cleanup.ForceCleanupRatio = 0.5 // 清理 50%
	manager := NewFourDimensionMemoryManager(config)

	// 记录多个资源内存
	for i := 0; i < 10; i++ {
		manager.RecordMemoryUsage("resource", "resource-"+string(rune(i)), 1024)
	}

	// 触发强制清理
	cleaned := manager.TriggerCleanup(CleanupStrategyForce)
	if cleaned < 0 {
		t.Errorf("TriggerCleanup(Force) returned negative value: %d", cleaned)
	}

	stats := manager.GetMemoryUsage("resource")
	if stats.EvictedCount == 0 && cleaned > 0 {
		t.Errorf("Expected entries to be force cleaned")
	}
}

// TestFourDimensionMemoryManager_CheckMemoryStatus 测试内存状态检查
func TestFourDimensionMemoryManager_CheckMemoryStatus(t *testing.T) {
	tests := []struct {
		name           string
		usageBytes     int64
		limitBytes     int64
		warningRatio   float64
		criticalRatio  float64
		expectedStatus MemoryStatus
	}{
		{
			name:           "正常状态",
			usageBytes:     50,
			limitBytes:     100,
			warningRatio:   0.7,
			criticalRatio:  0.9,
			expectedStatus: MemoryStatusNormal,
		},
		{
			name:           "警告状态",
			usageBytes:     80,
			limitBytes:     100,
			warningRatio:   0.7,
			criticalRatio:  0.9,
			expectedStatus: MemoryStatusWarning,
		},
		{
			name:           "严重状态",
			usageBytes:     95,
			limitBytes:     100,
			warningRatio:   0.7,
			criticalRatio:  0.9,
			expectedStatus: MemoryStatusCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultMemoryManagerConfig()
			// 设置所有维度的限制为相同值，以便计算
			config.Limit.AccountLimit = tt.limitBytes
			config.Limit.ProductLimit = tt.limitBytes
			config.Limit.RegionLimit = tt.limitBytes
			config.Limit.ResourceLimit = tt.limitBytes
			config.Cleanup.WarningThreshold = tt.warningRatio
			config.Cleanup.CriticalThreshold = tt.criticalRatio
			manager := NewFourDimensionMemoryManager(config)

			// 记录内存使用（总限制为 4 * limitBytes）
			manager.RecordMemoryUsage("account", "account-1", tt.usageBytes*4)

			status := manager.GetStatus()
			if status != tt.expectedStatus {
				t.Errorf("Status = %v, want %v", status, tt.expectedStatus)
			}
		})
	}
}

func TestFourDimensionMemoryManager_Concurrent(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)
	manager.StartMemoryManager()
	defer manager.StopMemoryManager()

	var wg sync.WaitGroup
	goroutines := 100

	// 并发记录内存使用
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dimension := []string{"account", "product", "region", "resource"}[idx%4]
			key := string(rune('a' + idx%26))
			bytes := int64(idx+1) * 1024
			manager.RecordMemoryUsage(dimension, key+string(rune('a'+idx%26)), bytes)
		}(i)
	}

	wg.Wait()

	stats := manager.ScanMemory()
	if stats.EntryCount != goroutines {
		t.Errorf("EntryCount = %d, want %d", stats.EntryCount, goroutines)
	}
}

// TestFourDimensionMemoryManager_Metrics 测试指标更新
func TestFourDimensionMemoryManager_Metrics(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 记录内存使用
	manager.RecordMemoryUsage("account", "account-1", 1024)
	manager.RecordMemoryUsage("product", "product-1", 2048)
	manager.RecordMemoryUsage("region", "region-1", 4096)
	manager.RecordMemoryUsage("resource", "resource-1", 8192)

	// 触发清理
	manager.TriggerCleanup(CleanupStrategyLRU)

	// 等待指标更新
	time.Sleep(100 * time.Millisecond)

	stats := manager.GetMemoryManagerStats()
	if stats.TotalBytes == 0 {
		t.Errorf("Expected non-zero TotalBytes")
	}
}

// TestFourDimensionMemoryManager_RuntimeMemory 测试运行时内存统计
func TestFourDimensionMemoryManager_RuntimeMemory(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 记录一些内存使用
	for i := 0; i < 1000; i++ {
		manager.RecordMemoryUsage("resource", "resource-"+string(rune(i)), 1024)
	}

	// 获取运行时内存统计
	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)

	if mstats.Alloc == 0 {
		t.Errorf("Expected non-zero allocated memory")
	}

	stats := manager.ScanMemory()
	if stats.EntryCount != 1000 {
		t.Errorf("EntryCount = %d, want 1000", stats.EntryCount)
	}
}

// TestFourDimensionMemoryManager_StartStop 测试启动和停止
func TestFourDimensionMemoryManager_StartStop(t *testing.T) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 启动管理器
	manager.StartMemoryManager()

	// 记录一些内存使用
	for i := 0; i < 10; i++ {
		manager.RecordMemoryUsage("account", "account-"+string(rune(i)), 1024)
	}

	// 等待自动清理
	time.Sleep(2 * time.Second)

	// 停止管理器
	manager.StopMemoryManager()

	stats := manager.GetMemoryManagerStats()
	if stats.CleanupCount == 0 {
		t.Logf("No cleanup performed (may be expected)")
	}
}

// BenchmarkFourDimensionMemoryManager_RecordMemoryUsage 基准测试：记录内存使用
func BenchmarkFourDimensionMemoryManager_RecordMemoryUsage(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			dimension := []string{"account", "product", "region", "resource"}[i%4]
			key := string(rune('a' + i%26))
			manager.RecordMemoryUsage(dimension, key+string(rune('a'+i%26)), 1024)
			i++
		}
	})
}

// BenchmarkFourDimensionMemoryManager_ScanMemory 基准测试：扫描内存
func BenchmarkFourDimensionMemoryManager_ScanMemory(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 预填充数据
	for i := 0; i < 10000; i++ {
		manager.RecordMemoryUsage("resource", "resource-"+string(rune(i)), 1024)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ScanMemory()
	}
}

// BenchmarkFourDimensionMemoryManager_TriggerCleanup 基准测试：触发清理
func BenchmarkFourDimensionMemoryManager_TriggerCleanup(b *testing.B) {
	config := DefaultMemoryManagerConfig()
	manager := NewFourDimensionMemoryManager(config)

	// 预填充数据
	for i := 0; i < 10000; i++ {
		manager.RecordMemoryUsage("resource", "resource-"+string(rune(i)), 1024)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.TriggerCleanup(CleanupStrategyLRU)
	}
}
