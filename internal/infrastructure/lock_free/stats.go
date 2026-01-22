// 全局统计：使用原子操作实现的性能统计
package lock_free

import (
	"sync/atomic"
	"time"
)

// GlobalStats 全局统计信息
type GlobalStats struct {
	// 采集统计
	totalCollections      atomic.Int64
	successfulCollections atomic.Int64
	failedCollections     atomic.Int64

	// 请求统计
	totalRequests         atomic.Int64
	successfulRequests    atomic.Int64
	failedRequests        atomic.Int64
	limitExceededRequests atomic.Int64

	// 并发统计
	currentConcurrency atomic.Int64
	maxConcurrency     atomic.Int64
	lockContentions    atomic.Int64

	// 性能统计
	totalDurationMs atomic.Int64
	totalRequestsMs atomic.Int64

	// 内存统计
	allocatedObjects atomic.Int64
	freedObjects     atomic.Int64

	// 集群同步统计
	syncOperations   atomic.Int64
	syncFailures     atomic.Int64
	syncOperationsMs atomic.Int64

	// 时间统计
	startTime      time.Time
	lastUpdateTime atomic.Value
}

// NewGlobalStats 创建全局统计
func NewGlobalStats() *GlobalStats {
	stats := &GlobalStats{}
	stats.startTime = time.Now()
	stats.lastUpdateTime.Store(time.Now())
	return stats
}

// IncTotalCollections 增加总采集次数
func (gs *GlobalStats) IncTotalCollections() {
	gs.totalCollections.Add(1)
	gs.updateLastUpdate()
}

// IncSuccessfulCollections 增加成功采集次数
func (gs *GlobalStats) IncSuccessfulCollections() {
	gs.successfulCollections.Add(1)
}

// IncFailedCollections 增加失败采集次数
func (gs *GlobalStats) IncFailedCollections() {
	gs.failedCollections.Add(1)
}

// IncTotalRequests 增加总请求数
func (gs *GlobalStats) IncTotalRequests() {
	gs.totalRequests.Add(1)
}

// IncSuccessfulRequests 增加成功请求数
func (gs *GlobalStats) IncSuccessfulRequests() {
	gs.successfulRequests.Add(1)
}

// IncFailedRequests 增加失败请求数
func (gs *GlobalStats) IncFailedRequests() {
	gs.failedRequests.Add(1)
}

// IncLimitExceededRequests 增加限流请求数
func (gs *GlobalStats) IncLimitExceededRequests() {
	gs.limitExceededRequests.Add(1)
}

// RecordCurrentConcurrency 记录当前并发度
func (gs *GlobalStats) RecordCurrentConcurrency(concurrency int64) {
	gs.currentConcurrency.Store(concurrency)

	// 更新最大并发度
	currentMax := gs.maxConcurrency.Load()
	if concurrency <= currentMax {
		return
	}
	gs.maxConcurrency.Store(concurrency)
}

// IncLockContentions 增加锁竞争次数
func (gs *GlobalStats) IncLockContentions() {
	gs.lockContentions.Add(1)
}

// RecordCollectionDuration 记录采集耗时（毫秒）
func (gs *GlobalStats) RecordCollectionDuration(duration time.Duration) {
	ms := int64(duration / time.Millisecond)
	gs.totalDurationMs.Add(ms)
}

// RecordRequestDuration 记录请求耗时（毫秒）
func (gs *GlobalStats) RecordRequestDuration(duration time.Duration) {
	ms := int64(duration / time.Millisecond)
	gs.totalRequestsMs.Add(ms)
}

// IncAllocatedObjects 增加已分配对象数
func (gs *GlobalStats) IncAllocatedObjects() {
	gs.allocatedObjects.Add(1)
}

// IncFreedObjects 增加已释放对象数
func (gs *GlobalStats) IncFreedObjects() {
	gs.freedObjects.Add(1)
}

// IncSyncOperations 增加集群同步操作次数
func (gs *GlobalStats) IncSyncOperations() {
	gs.syncOperations.Add(1)
}

// IncSyncFailures 增加集群同步失败次数
func (gs *GlobalStats) IncSyncFailures() {
	gs.syncFailures.Add(1)
}

// RecordSyncDuration 记录集群同步耗时（毫秒）
func (gs *GlobalStats) RecordSyncDuration(duration time.Duration) {
	ms := int64(duration / time.Millisecond)
	gs.syncOperationsMs.Add(ms)
}

// updateLastUpdate 更新最后更新时间
func (gs *GlobalStats) updateLastUpdate() {
	gs.lastUpdateTime.Store(time.Now())
}

// UpdateStats 更新统计信息
func (gs *GlobalStats) UpdateStats(snapshot GlobalStatsSnapshot) {
	gs.totalCollections.Store(snapshot.TotalCollections)
	gs.successfulCollections.Store(snapshot.SuccessfulCollections)
	gs.failedCollections.Store(snapshot.FailedCollections)

	gs.totalRequests.Store(snapshot.TotalRequests)
	gs.successfulRequests.Store(snapshot.SuccessfulRequests)
	gs.failedRequests.Store(snapshot.FailedRequests)
	gs.limitExceededRequests.Store(snapshot.LimitExceededRequests)

	gs.currentConcurrency.Store(snapshot.CurrentConcurrency)
	gs.maxConcurrency.Store(snapshot.MaxConcurrency)
	gs.lockContentions.Store(snapshot.LockContentions)

	gs.totalDurationMs.Store(snapshot.TotalDurationMs)
	gs.totalRequestsMs.Store(snapshot.TotalRequestsMs)

	gs.allocatedObjects.Store(snapshot.AllocatedObjects)
	gs.freedObjects.Store(snapshot.FreedObjects)

	gs.syncOperations.Store(snapshot.SyncOperations)
	gs.syncFailures.Store(snapshot.SyncFailures)
	gs.syncOperationsMs.Store(snapshot.SyncOperationsMs)

	gs.updateLastUpdate()
}

// GetGlobalSnapshot 获取全局统计快照
func (gs *GlobalStats) GetGlobalSnapshot() GlobalStatsSnapshot {
	return GlobalStatsSnapshot{
		TotalCollections:      gs.totalCollections.Load(),
		SuccessfulCollections: gs.successfulCollections.Load(),
		FailedCollections:     gs.failedCollections.Load(),

		TotalRequests:         gs.totalRequests.Load(),
		SuccessfulRequests:    gs.successfulRequests.Load(),
		FailedRequests:        gs.failedRequests.Load(),
		LimitExceededRequests: gs.limitExceededRequests.Load(),

		CurrentConcurrency: gs.currentConcurrency.Load(),
		MaxConcurrency:     gs.maxConcurrency.Load(),
		LockContentions:    gs.lockContentions.Load(),

		TotalDurationMs: gs.totalDurationMs.Load(),
		TotalRequestsMs: gs.totalRequestsMs.Load(),

		AllocatedObjects: gs.allocatedObjects.Load(),
		FreedObjects:     gs.freedObjects.Load(),

		SyncOperations:   gs.syncOperations.Load(),
		SyncFailures:     gs.syncFailures.Load(),
		SyncOperationsMs: gs.syncOperationsMs.Load(),

		StartTime:      gs.startTime,
		LastUpdateTime: gs.lastUpdateTime.Load().(time.Time),
	}
}

// GlobalStatsSnapshot 全局统计快照
type GlobalStatsSnapshot struct {
	// 采集统计
	TotalCollections      int64 `json:"total_collections"`
	SuccessfulCollections int64 `json:"successful_collections"`
	FailedCollections     int64 `json:"failed_collections"`

	// 请求统计
	TotalRequests         int64 `json:"total_requests"`
	SuccessfulRequests    int64 `json:"successful_requests"`
	FailedRequests        int64 `json:"failed_requests"`
	LimitExceededRequests int64 `json:"limit_exceeded_requests"`

	// 并发统计
	CurrentConcurrency int64 `json:"current_concurrency"`
	MaxConcurrency     int64 `json:"max_concurrency"`
	LockContentions    int64 `json:"lock_contentions"`

	// 性能统计
	TotalDurationMs int64 `json:"total_duration_ms"`
	TotalRequestsMs int64 `json:"total_requests_ms"`

	// 内存统计
	AllocatedObjects int64 `json:"allocated_objects"`
	FreedObjects     int64 `json:"freed_objects"`

	// 集群同步统计
	SyncOperations   int64 `json:"sync_operations"`
	SyncFailures     int64 `json:"sync_failures"`
	SyncOperationsMs int64 `json:"sync_operations_ms"`

	// 时间统计
	StartTime      time.Time `json:"start_time"`
	LastUpdateTime time.Time `json:"last_update_time"`
}

// Reset 重置所有统计
func (gs *GlobalStats) Reset() {
	gs.totalCollections.Store(0)
	gs.successfulCollections.Store(0)
	gs.failedCollections.Store(0)

	gs.totalRequests.Store(0)
	gs.successfulRequests.Store(0)
	gs.failedRequests.Store(0)
	gs.limitExceededRequests.Store(0)

	gs.currentConcurrency.Store(0)
	gs.maxConcurrency.Store(0)
	gs.lockContentions.Store(0)

	gs.totalDurationMs.Store(0)
	gs.totalRequestsMs.Store(0)

	gs.allocatedObjects.Store(0)
	gs.freedObjects.Store(0)

	gs.syncOperations.Store(0)
	gs.syncFailures.Store(0)
	gs.syncOperationsMs.Store(0)

	gs.startTime = time.Now()
	gs.lastUpdateTime.Store(time.Now())
}
