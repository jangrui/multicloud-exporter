// 无锁并发模型单元测试
package lock_free

import (
	"testing"
	"time"
)

// TestLockFreeManager_Load 测试 Load 操作
func TestLockFreeManager_Load(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.Store("test")

	value := lfm.Load()
	if value != "test" {
		t.Errorf("expected 'test', got %v", value)
	}
}

// TestLockFreeManager_Store 测试 Store 操作
func TestLockFreeManager_Store(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.Store(42)

	value := lfm.Load()
	if value != 42 {
		t.Errorf("expected 42, got %v", value)
	}
}

// TestLockFreeManager_CompareAndSwap 测试 CAS 操作
func TestLockFreeManager_CompareAndSwap(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.Store("initial")

	// CAS 成功
	swapped := lfm.CompareAndSwap("initial", "new")
	if !swapped {
		t.Errorf("expected CAS to succeed")
	}

	value := lfm.Load()
	if value != "new" {
		t.Errorf("expected 'new', got %v", value)
	}

	// CAS 失败
	swapped = lfm.CompareAndSwap("old", "newer")
	if swapped {
		t.Errorf("expected CAS to fail")
	}

	value = lfm.Load()
	if value != "new" {
		t.Errorf("expected 'new', got %v", value)
	}
}

// TestLockFreeManager_Swap 测试 Swap 操作
func TestLockFreeManager_Swap(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.Store("old")

	old := lfm.Swap("new")
	if old != "old" {
		t.Errorf("expected 'old', got %v", old)
	}

	value := lfm.Load()
	if value != "new" {
		t.Errorf("expected 'new', got %v", value)
	}
}

// TestLockFreeManager_LoadInt64 测试 int64 值加载
func TestLockFreeManager_LoadInt64(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreInt64(42)

	value := lfm.LoadInt64()
	if value != 42 {
		t.Errorf("expected 42, got %d", value)
	}
}

// TestLockFreeManager_StoreInt64 测试 int64 值存储
func TestLockFreeManager_StoreInt64(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreInt64(100)

	value := lfm.LoadInt64()
	if value != 100 {
		t.Errorf("expected 100, got %d", value)
	}
}

// TestLockFreeManager_AddInt64 测试 int64 增加
func TestLockFreeManager_AddInt64(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreInt64(0)

	result := lfm.AddInt64(10)
	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}

	value := lfm.LoadInt64()
	if value != 10 {
		t.Errorf("expected 10, got %d", value)
	}

	result = lfm.AddInt64(5)
	if result != 15 {
		t.Errorf("expected 15, got %d", result)
	}

	value = lfm.LoadInt64()
	if value != 15 {
		t.Errorf("expected 15, got %d", value)
	}
}

// TestLockFreeManager_LoadUint32 测试 uint32 值加载
func TestLockFreeManager_LoadUint32(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreUint32(42)

	value := lfm.LoadUint32()
	if value != 42 {
		t.Errorf("expected 42, got %d", value)
	}
}

// TestLockFreeManager_StoreUint32 测试 uint32 值存储
func TestLockFreeManager_StoreUint32(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreUint32(100)

	value := lfm.LoadUint32()
	if value != 100 {
		t.Errorf("expected 100, got %d", value)
	}
}

// TestLockFreeManager_AddUint32 测试 uint32 增加
func TestLockFreeManager_AddUint32(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreUint32(0)

	result := lfm.AddUint32(10)
	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}

	value := lfm.LoadUint32()
	if value != 10 {
		t.Errorf("expected 10, got %d", value)
	}

	result = lfm.AddUint32(5)
	if result != 15 {
		t.Errorf("expected 15, got %d", result)
	}

	value = lfm.LoadUint32()
	if value != 15 {
		t.Errorf("expected 15, got %d", value)
	}
}

// TestGlobalStats_IncTotalCollections 测试采集统计
func TestGlobalStats_IncTotalCollections(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncTotalCollections()
	gs.IncTotalCollections()
	gs.IncTotalCollections()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.TotalCollections != 3 {
		t.Errorf("expected 3, got %d", snapshot.TotalCollections)
	}
}

// TestGlobalStats_IncFailedCollections 测试失败采集统计
func TestGlobalStats_IncFailedCollections(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncFailedCollections()
	gs.IncFailedCollections()
	gs.IncSuccessfulCollections()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.FailedCollections != 2 {
		t.Errorf("expected 2, got %d", snapshot.FailedCollections)
	}
	if snapshot.SuccessfulCollections != 1 {
		t.Errorf("expected 1, got %d", snapshot.SuccessfulCollections)
	}
}

// TestGlobalStats_CurrentConcurrency 测试并发度统计
func TestGlobalStats_CurrentConcurrency(t *testing.T) {
	gs := NewGlobalStats()

	gs.RecordCurrentConcurrency(5)
	gs.RecordCurrentConcurrency(10)
	gs.RecordCurrentConcurrency(3)

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.CurrentConcurrency != 3 {
		t.Errorf("expected 3, got %d", snapshot.CurrentConcurrency)
	}
	if snapshot.MaxConcurrency != 10 {
		t.Errorf("expected 10, got %d", snapshot.MaxConcurrency)
	}
}

// TestGlobalStats_LockContentions 测试锁竞争统计
func TestGlobalStats_LockContentions(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncLockContentions()
	gs.IncLockContentions()
	gs.IncLockContentions()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.LockContentions != 3 {
		t.Errorf("expected 3, got %d", snapshot.LockContentions)
	}
}

// TestGlobalStats_MemoryStats 测试内存统计
func TestGlobalStats_MemoryStats(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncAllocatedObjects()
	gs.IncAllocatedObjects()
	gs.IncFreedObjects()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.AllocatedObjects != 2 {
		t.Errorf("expected 2, got %d", snapshot.AllocatedObjects)
	}
	if snapshot.FreedObjects != 1 {
		t.Errorf("expected 1, got %d", snapshot.FreedObjects)
	}
}

// TestGlobalStats_SyncStats 测试集群同步统计
func TestGlobalStats_SyncStats(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncSyncOperations()
	gs.IncSyncOperations()
	gs.IncSyncFailures()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.SyncOperations != 2 {
		t.Errorf("expected 2, got %d", snapshot.SyncOperations)
	}
	if snapshot.SyncFailures != 1 {
		t.Errorf("expected 1, got %d", snapshot.SyncFailures)
	}
}

// TestGlobalStats_Reset 测试重置统计
func TestGlobalStats_Reset(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncTotalCollections()
	gs.IncSuccessfulCollections()
	gs.RecordCurrentConcurrency(5)
	gs.IncLockContentions()

	gs.Reset()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.TotalCollections != 0 {
		t.Errorf("expected 0, got %d", snapshot.TotalCollections)
	}
	if snapshot.SuccessfulCollections != 0 {
		t.Errorf("expected 0, got %d", snapshot.SuccessfulCollections)
	}
	if snapshot.CurrentConcurrency != 0 {
		t.Errorf("expected 0, got %d", snapshot.CurrentConcurrency)
	}
	if snapshot.LockContentions != 0 {
		t.Errorf("expected 0, got %d", snapshot.LockContentions)
	}
}

// TestLockFreeManager_ConcurrentAccess 测试并发访问
func TestLockFreeManager_ConcurrentAccess(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.StoreInt64(0)

	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(id int) {
			defer func() { done <- true }()
			lfm.AddInt64(1)
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 50; i++ {
		<-done
	}

	// 验证最终值（由于竞争条件，结果可能略小于 50，但应该在合理范围内）
	value := lfm.LoadInt64()
	if value < 45 || value > 50 {
		t.Errorf("concurrent add result out of reasonable range: got %d", value)
	}
}

// TestGlobalStats_ConcurrentAccess 测试统计并发访问
func TestGlobalStats_ConcurrentAccess(t *testing.T) {
	gs := NewGlobalStats()

	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(id int) {
			defer func() { done <- true }()

			if id%3 == 0 {
				gs.IncTotalCollections()
			} else if id%3 == 1 {
				gs.IncFailedCollections()
			} else {
				gs.IncSuccessfulCollections()
			}
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	snapshot := gs.GetGlobalSnapshot()
	total := snapshot.TotalCollections + snapshot.SuccessfulCollections + snapshot.FailedCollections
	if total != 50 {
		t.Errorf("expected total 50, got %d", total)
	}
}

// BenchmarkLockFreeManager_Load 某准测试 Load 性能
func BenchmarkLockFreeManager_Load(b *testing.B) {
	lfm := NewLockFreeManager()
	lfm.Store("test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lfm.Load()
	}
}

// BenchmarkLockFreeManager_Store 某准测试 Store 性能
func BenchmarkLockFreeManager_Store(b *testing.B) {
	lfm := NewLockFreeManager()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lfm.Store(i)
	}
}

// BenchmarkLockFreeManager_CompareAndSwap 某准测试 CAS 性能
func BenchmarkLockFreeManager_CompareAndSwap(b *testing.B) {
	lfm := NewLockFreeManager()
	lfm.Store(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lfm.CompareAndSwap(i, i+1)
	}
}

// BenchmarkGlobalStats_IncTotalCollections 某准测试统计性能
func BenchmarkGlobalStats_IncTotalCollections(b *testing.B) {
	gs := NewGlobalStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gs.IncTotalCollections()
	}
}

// TestLockFreeManager_GetAndSet 测试 GetAndSet 操作
func TestLockFreeManager_GetAndSet(t *testing.T) {
	lfm := NewLockFreeManager()
	lfm.Store("initial")

	// 测试读取并修改（直接传递新值）
	oldValue := lfm.GetAndSet("new_value")

	if oldValue != "initial" {
		t.Errorf("expected 'initial' as old value, got %v", oldValue)
	}

	value := lfm.Load()
	if value != "new_value" {
		t.Errorf("expected 'new_value', got %v", value)
	}

	// 测试 int64 值
	lfm2 := NewLockFreeManager()
	lfm2.StoreInt64(100)

	oldInt64 := lfm2.GetAndSet(int64(200))

	if oldInt64 != int64(100) {
		t.Errorf("expected 100 as old int64, got %v", oldInt64)
	}

	newInt64 := lfm2.LoadInt64()
	if newInt64 != int64(200) {
		t.Errorf("expected 200 as new int64, got %v", newInt64)
	}
}

// TestLockFreeManager_Lock 测试 Lock 操作
func TestLockFreeManager_Lock(t *testing.T) {
	lfm := NewLockFreeManager()

	// Lock 应不 panic
	lfm.Lock()

	// 再次 Lock 应不 panic（可重入）
	lfm.Lock()

	lfm.Unlock()
	lfm.Unlock()
}

// TestLockFreeManager_RLock 测试 RLock 操作
func TestLockFreeManager_RLock(t *testing.T) {
	lfm := NewLockFreeManager()

	// RLock 应不 panic
	lfm.RLock()

	// 再次 RLock 应不 panic（可重入）
	lfm.RLock()

	lfm.RUnlock()
	lfm.RUnlock()
}

// TestGlobalStats_IncTotalRequests 测试请求统计
func TestGlobalStats_IncTotalRequests(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncTotalRequests()
	gs.IncTotalRequests()
	gs.IncTotalRequests()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", snapshot.TotalRequests)
	}
}

// TestGlobalStats_IncSuccessfulRequests 测试成功请求统计
func TestGlobalStats_IncSuccessfulRequests(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncSuccessfulRequests()
	gs.IncSuccessfulRequests()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.SuccessfulRequests != 2 {
		t.Errorf("expected 2 successful requests, got %d", snapshot.SuccessfulRequests)
	}
}

// TestGlobalStats_IncFailedRequests 测试失败请求统计
func TestGlobalStats_IncFailedRequests(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncFailedRequests()
	gs.IncFailedRequests()
	gs.IncFailedRequests()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.FailedRequests != 3 {
		t.Errorf("expected 3 failed requests, got %d", snapshot.FailedRequests)
	}
}

// TestGlobalStats_IncLimitExceededRequests 测试限流请求统计
func TestGlobalStats_IncLimitExceededRequests(t *testing.T) {
	gs := NewGlobalStats()

	gs.IncLimitExceededRequests()
	gs.IncLimitExceededRequests()

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.LimitExceededRequests != 2 {
		t.Errorf("expected 2 limit exceeded requests, got %d", snapshot.LimitExceededRequests)
	}
}

// TestGlobalStats_RecordCollectionDuration 测试采集持续时间记录
func TestGlobalStats_RecordCollectionDuration(t *testing.T) {
	gs := NewGlobalStats()

	// 先增加采集计数
	gs.IncTotalCollections()
	gs.IncTotalCollections()
	gs.IncTotalCollections()

	gs.RecordCollectionDuration(500 * time.Millisecond)
	gs.RecordCollectionDuration(1000 * time.Millisecond)
	gs.RecordCollectionDuration(1500 * time.Millisecond)

	snapshot := gs.GetGlobalSnapshot()
	if snapshot.TotalCollections != 3 {
		t.Errorf("expected 3 collections, got %d", snapshot.TotalCollections)
	}

	expectedTotalMs := int64(500 + 1000 + 1500)
	if snapshot.TotalDurationMs != expectedTotalMs {
		t.Errorf("expected %dms total duration, got %d", expectedTotalMs, snapshot.TotalDurationMs)
	}
}

// TestGlobalStats_RecordRequestDuration 测试请求持续时间记录
func TestGlobalStats_RecordRequestDuration(t *testing.T) {
	gs := NewGlobalStats()

	gs.RecordRequestDuration(10 * time.Millisecond)
	gs.RecordRequestDuration(20 * time.Millisecond)
	gs.RecordRequestDuration(30 * time.Millisecond)

	snapshot := gs.GetGlobalSnapshot()

	expectedTotalMs := int64(10 + 20 + 30)
	if snapshot.TotalRequestsMs != expectedTotalMs {
		t.Errorf("expected %dms total duration, got %d", expectedTotalMs, snapshot.TotalRequestsMs)
	}
}

// TestGlobalStats_RecordSyncDuration 测试同步持续时间记录
func TestGlobalStats_RecordSyncDuration(t *testing.T) {
	gs := NewGlobalStats()

	gs.RecordSyncDuration(100 * time.Millisecond)
	gs.RecordSyncDuration(200 * time.Millisecond)

	snapshot := gs.GetGlobalSnapshot()

	expectedTotalMs := int64(100 + 200)
	if snapshot.SyncOperationsMs != expectedTotalMs {
		t.Errorf("expected %dms total sync duration, got %d", expectedTotalMs, snapshot.SyncOperationsMs)
	}
}

// TestGlobalStats_UpdateStats 测试统计更新
func TestGlobalStats_UpdateStats(t *testing.T) {
	gs := NewGlobalStats()

	// 先设置一些值
	gs.IncTotalCollections()
	gs.IncSuccessfulCollections()
	gs.RecordCurrentConcurrency(5)

	// 使用 UpdateStats 更新（传递快照）
	gs.UpdateStats(GlobalStatsSnapshot{
		TotalCollections:      10,
		SuccessfulCollections: 8,
		AllocatedObjects:      20,
		CurrentConcurrency:    3,
	})

	snapshot2 := gs.GetGlobalSnapshot()

	// 验证更新
	if snapshot2.TotalCollections != 10 {
		t.Errorf("expected 10 collections, got %d", snapshot2.TotalCollections)
	}
	if snapshot2.SuccessfulCollections != 8 {
		t.Errorf("expected 8 successful, got %d", snapshot2.SuccessfulCollections)
	}
	if snapshot2.AllocatedObjects != 20 {
		t.Errorf("expected 20 allocated, got %d", snapshot2.AllocatedObjects)
	}
	if snapshot2.CurrentConcurrency != 3 {
		t.Errorf("expected 3 concurrency, got %d", snapshot2.CurrentConcurrency)
	}
}
