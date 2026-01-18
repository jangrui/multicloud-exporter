// Package common 提供先进的智能区域发现和管理功能
// 特性：
// - 智能区域选择（优先活跃区域，跳过空区域）
// - 自动内存管理和清理
// - 集群状态同步
// - 并发安全
// - 性能监控
// - 优雅停止
package common

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
)

// RegionStatus 区域状态
type RegionStatus string

const (
	RegionStatusUnknown RegionStatus = "unknown" // 未知，首次运行
	RegionStatusActive  RegionStatus = "active"  // 有资源
	RegionStatusEmpty   RegionStatus = "empty"   // 无资源
)

// RegionInfo 区域信息
type RegionInfo struct {
	Status        RegionStatus `json:"status"`
	LastSeen      time.Time    `json:"last_seen"`      // 最后检查时间
	LastActive    time.Time    `json:"last_active"`    // 最后活跃时间
	EmptyCount    int          `json:"empty_count"`    // 连续空次数
	ResourceCount int          `json:"resource_count"` // 资源数量
	Priority      int          `json:"priority"`       // 优先级（用于排序）
}

// RegionDiscoveryConfig 区域发现配置
type RegionDiscoveryConfig struct {
	Enabled              bool          `json:"enabled"`                 // 是否启用
	DiscoveryInterval    time.Duration `json:"discovery_interval"`      // 重新发现周期
	EmptyThreshold       int           `json:"empty_threshold"`         // 空区域跳过阈值
	MaxAccounts          int           `json:"max_accounts"`            // 最大账号数（0=无限制）
	CleanupInterval      time.Duration `json:"cleanup_interval"`        // 清理间隔
	MaxRegionsPerAccount int           `json:"max_regions_per_account"` // 每账号最大区域数
}

// RegionManagerStats 统计信息
type RegionManagerStats struct {
	TotalAccounts       int       `json:"total_accounts"`
	TotalRegions        int       `json:"total_regions"`
	ActiveRegions       int       `json:"active_regions"`
	EmptyRegions        int       `json:"empty_regions"`
	UnknownRegions      int       `json:"unknown_regions"`
	SkippedRegions      int       `json:"skipped_regions"`
	LastCleanupTime     time.Time `json:"last_cleanup_time"`
	LastRediscoveryTime time.Time `json:"last_rediscovery_time"`
	UpdateCount         int64     `json:"update_count"`
	RediscoveryCount    int64     `json:"rediscovery_count"`
	RemoteUpdateCount   int64     `json:"remote_update_count"`
}

// Broadcaster 定义集群广播接口
type Broadcaster interface {
	BroadcastRegionStatus(provider, accountID, region, status string, resourceCount int)
}

// RegionManager 区域管理器接口
type RegionManager interface {
	// GetActiveRegions 获取活跃区域列表
	GetActiveRegions(accountID string, allRegions []string) []string

	// UpdateRegionStatus 更新区域状态（本地更新，触发广播）
	UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus)

	// UpdateFromPeer 更新区域状态（来自 Peer，不触发广播）
	UpdateFromPeer(accountID, region string, resourceCount int, status string)

	// SetBroadcaster 设置广播器
	SetBroadcaster(b Broadcaster, provider string)

	// MarkRegionForRediscovery 标记区域为需重新发现
	MarkRegionForRediscovery(accountID, region string)

	// GetRegionInfo 获取区域信息
	GetRegionInfo(accountID, region string) (RegionInfo, bool)

	// ShouldSkipRegion 判断是否跳过该区域
	ShouldSkipRegion(accountID, region string) bool

	// StartRediscoveryScheduler 启动调度器
	StartRediscoveryScheduler()

	// Stop 停止所有后台任务
	Stop()

	// GetStats 获取统计信息
	GetStats() RegionManagerStats

	// CleanupInactiveAccounts 清理不活跃账号
	CleanupInactiveAccounts(olderThan time.Duration) int
}

// SmartRegionManager 智能区域管理器实现
type SmartRegionManager struct {
	mu            sync.RWMutex
	config        RegionDiscoveryConfig
	regionMap     map[string]map[string]RegionInfo
	stopChan      chan struct{}
	stopped       atomic.Bool
	schedulerOnce sync.Once

	broadcaster  Broadcaster
	providerName string

	// 统计信息
	stats   RegionManagerStats
	statsMu sync.RWMutex
}

// NewRegionManager 创建区域管理器
func NewRegionManager(config RegionDiscoveryConfig) RegionManager {
	// 设置默认值
	if config.DiscoveryInterval <= 0 {
		config.DiscoveryInterval = 1 * time.Hour
	}
	if config.EmptyThreshold <= 0 {
		config.EmptyThreshold = 10
	}
	if config.MaxAccounts < 0 {
		config.MaxAccounts = 0
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 1 * time.Hour
	}
	if config.MaxRegionsPerAccount <= 0 {
		config.MaxRegionsPerAccount = 1000
	}

	rm := &SmartRegionManager{
		config:    config,
		regionMap: make(map[string]map[string]RegionInfo),
		stopChan:  make(chan struct{}),
		stats: RegionManagerStats{
			LastCleanupTime:     time.Now(),
			LastRediscoveryTime: time.Now(),
		},
	}

	return rm
}

// SetBroadcaster 设置广播器
func (rm *SmartRegionManager) SetBroadcaster(b Broadcaster, provider string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.broadcaster = b
	rm.providerName = provider
}

// GetActiveRegions 获取活跃区域列表
func (rm *SmartRegionManager) GetActiveRegions(accountID string, allRegions []string) []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if !rm.config.Enabled {
		return allRegions
	}

	// 内存保护
	if rm.config.MaxAccounts > 0 && len(rm.regionMap) >= rm.config.MaxAccounts {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "RegionSelection", "account_id", accountID)
		ctxLog.Warnf("账号数达上限 %d，跳过智能区域选择", rm.config.MaxAccounts)
		return allRegions
	}

	accountRegions, ok := rm.regionMap[accountID]
	if !ok {
		return allRegions
	}

	// 按优先级排序：active > unknown > empty (未达阈值)
	activeRegions := make([]string, 0, len(allRegions)/2)
	unknownRegions := make([]string, 0, len(allRegions)/2)
	skippedCount := 0

	for _, region := range allRegions {
		info, exists := accountRegions[region]
		if !exists {
			unknownRegions = append(unknownRegions, region)
			continue
		}

		switch info.Status {
		case RegionStatusActive:
			activeRegions = append(activeRegions, region)
		case RegionStatusUnknown:
			unknownRegions = append(unknownRegions, region)
		case RegionStatusEmpty:
			if info.EmptyCount < rm.config.EmptyThreshold {
				unknownRegions = append(unknownRegions, region)
			} else {
				skippedCount++
			}
		}
	}

	result := append(activeRegions, unknownRegions...)

	if len(result) == 0 {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "RegionSelection", "account_id", accountID)
		ctxLog.Warnf("无可用区域，返回全部")
		return allRegions
	}

	ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "RegionSelection", "account_id", accountID)
	ctxLog.Infof("智能区域选择 总=%d 活跃=%d 未知=%d 跳过=%d",
		len(allRegions), len(activeRegions), len(unknownRegions), skippedCount)

	return result
}

// UpdateRegionStatus 更新区域状态（本地更新，触发广播）
func (rm *SmartRegionManager) UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus) {
	rm.updateRegionStatusInternal(accountID, region, resourceCount, status)

	// 触发广播
	rm.mu.RLock()
	b := rm.broadcaster
	p := rm.providerName
	rm.mu.RUnlock()

	if b != nil && p != "" {
		b.BroadcastRegionStatus(p, accountID, region, string(status), resourceCount)
	}
}

// UpdateFromPeer 更新区域状态（来自 Peer，不触发广播）
func (rm *SmartRegionManager) UpdateFromPeer(accountID, region string, resourceCount int, statusStr string) {
	status := RegionStatus(statusStr)
	rm.updateRegionStatusInternal(accountID, region, resourceCount, status)
	atomic.AddInt64(&rm.stats.RemoteUpdateCount, 1)
}

// updateRegionStatusInternal 内部更新逻辑
func (rm *SmartRegionManager) updateRegionStatusInternal(accountID, region string, resourceCount int, status RegionStatus) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	atomic.AddInt64(&rm.stats.UpdateCount, 1)

	now := time.Now()

	if rm.regionMap[accountID] == nil {
		rm.regionMap[accountID] = make(map[string]RegionInfo)
	}

	info, exists := rm.regionMap[accountID][region]
	if !exists {
		info = RegionInfo{
			LastSeen: now,
			Priority: 0,
		}
	}

	// 如果状态没有变化且非 unknown，且已有记录，则不需要重置 EmptyCount
	// 但如果是 active，EmptyCount 总是 0
	// 如果是 empty，EmptyCount 增加
	// 这里的逻辑需要保持与原有一致

	info.Status = status
	info.LastSeen = now
	info.ResourceCount = resourceCount

	switch status {
	case RegionStatusActive:
		info.LastActive = now
		info.EmptyCount = 0
		info.Priority = 100 // 活跃区域优先级最高
	case RegionStatusEmpty:
		// 如果是 Peer 更新，我们可能不知道之前的 EmptyCount
		// 简单的做法是：如果是 Peer 告诉我们是 Empty，我们增加 EmptyCount？
		// 或者 Peer 传递的应该包含 EmptyCount？
		// 简化处理：每次 Empty 都 +1，如果 Peer 也是 Empty，说明它也没找到
		info.EmptyCount++
		info.Priority = 10 // 空区域优先级降低
	case RegionStatusUnknown:
		info.EmptyCount = 0
		info.Priority = 50 // 未知区域中等优先级
	}

	// 限制每账号区域数量
	if len(rm.regionMap[accountID]) >= rm.config.MaxRegionsPerAccount {
		// 在持写锁状态下找到要删除的区域列表
		var regionsToDelete []string
		accountRegions := rm.regionMap[accountID]
		for region, info := range accountRegions {
			if info.Status == RegionStatusEmpty && info.EmptyCount >= rm.config.EmptyThreshold {
				regionsToDelete = append(regionsToDelete, region)
			}
		}

		// 按时间排序，删除最旧的（直到满足数量限制）
		if len(regionsToDelete) > 0 {
			type regionTime struct {
				region string
				time   time.Time
			}
			// 构建时间列表
			var regionTimes []regionTime
			for _, region := range regionsToDelete {
				if info, ok := accountRegions[region]; ok {
					regionTimes = append(regionTimes, regionTime{region: region, time: info.LastSeen})
				}
			}
			// 按时间排序（最旧的在前）
			sort.Slice(regionTimes, func(i, j int) bool {
				return regionTimes[i].time.Before(regionTimes[j].time)
			})

			// 计算需要删除的数量
			currentCount := len(accountRegions)
			deleteCount := currentCount - rm.config.MaxRegionsPerAccount + 1
			if deleteCount > len(regionTimes) {
				deleteCount = len(regionTimes)
			}

			// 记录删除的区域（在锁内）
			for i := 0; i < deleteCount; i++ {
				region := regionTimes[i].region
				ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Eviction", "account_id", accountID, "region", region)
				ctxLog.Infof("驱逐旧区域（当前=%d, 最大=%d）", currentCount, rm.config.MaxRegionsPerAccount)

				// 在锁内删除
				delete(accountRegions, region)
			}
		}
	}

	rm.regionMap[accountID][region] = info
}

// MarkRegionForRediscovery 标记区域为需重新发现
func (rm *SmartRegionManager) MarkRegionForRediscovery(accountID, region string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.regionMap[accountID] == nil {
		return
	}

	if info, ok := rm.regionMap[accountID][region]; ok {
		info.Status = RegionStatusUnknown
		info.EmptyCount = 0
		info.Priority = 50
		rm.regionMap[accountID][region] = info
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Rediscovery", "account_id", accountID, "region", region)
		ctxLog.Infof("标记区域重新发现")
	}
}

// GetRegionInfo 获取区域信息
func (rm *SmartRegionManager) GetRegionInfo(accountID, region string) (RegionInfo, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.regionMap[accountID] == nil {
		return RegionInfo{}, false
	}

	info, ok := rm.regionMap[accountID][region]
	return info, ok
}

// ShouldSkipRegion 判断是否跳过该区域
func (rm *SmartRegionManager) ShouldSkipRegion(accountID, region string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if !rm.config.Enabled {
		return false
	}

	if rm.regionMap[accountID] == nil {
		return false
	}

	info, ok := rm.regionMap[accountID][region]
	if !ok {
		return false
	}

	return info.Status == RegionStatusEmpty && info.EmptyCount >= rm.config.EmptyThreshold
}

// StartRediscoveryScheduler 启动调度器（优化版本，合并任务减少锁竞争）
func (rm *SmartRegionManager) StartRediscoveryScheduler() {
	rm.schedulerOnce.Do(func() {
		if !rm.config.Enabled || rm.config.DiscoveryInterval <= 0 {
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Scheduler")
			ctxLog.Infof("区域调度器未启用")
			return
		}

		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Scheduler")
		ctxLog.Infof("启动区域调度器，周期=%v，清理间隔=%v",
			rm.config.DiscoveryInterval, rm.config.CleanupInterval)

		go func() {
			// 使用较小的间隔，统一执行所有任务
			tickInterval := rm.config.CleanupInterval
			if tickInterval > rm.config.DiscoveryInterval {
				tickInterval = rm.config.DiscoveryInterval
			}
			if tickInterval < time.Minute {
				tickInterval = time.Minute
			}

			ticker := time.NewTicker(tickInterval)
			defer ticker.Stop()

			for {
				select {
				case <-rm.stopChan:
					ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Scheduler")
					ctxLog.Infof("停止区域调度器")
					return
				case <-ticker.C:
					// 合并执行所有任务，减少锁竞争
					rm.performPeriodicTasks()
				}
			}
		}()
	})
}

// performPeriodicTasks 执行所有定期任务（优化版本，合并任务减少锁竞争）
func (rm *SmartRegionManager) performPeriodicTasks() {
	now := time.Now()

	// 1. 检查是否需要触发重新发现（从上次重新发现时间推算）
	rm.statsMu.RLock()
	lastRediscovery := rm.stats.LastRediscoveryTime
	lastCleanup := rm.stats.LastCleanupTime
	rm.statsMu.RUnlock()

	timeSinceRediscovery := now.Sub(lastRediscovery)
	if timeSinceRediscovery >= rm.config.DiscoveryInterval {
		rm.triggerRediscovery("periodic")
	}

	// 2. 检查是否需要清理不活跃账号
	timeSinceCleanup := now.Sub(lastCleanup)
	if timeSinceCleanup >= rm.config.CleanupInterval {
		cleaned := rm.CleanupInactiveAccounts(7 * 24 * time.Hour)
		if cleaned > 0 {
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Cleanup")
			ctxLog.Infof("定期清理完成，清理了 %d 个不活跃账号", cleaned)
		}
	}
}

// Stop 停止所有后台任务（优化版本，避免阻塞）
func (rm *SmartRegionManager) Stop() {
	if rm.stopped.Swap(true) {
		return
	}

	close(rm.stopChan)
	ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Shutdown")
	ctxLog.Infof("区域管理器已停止")
}

// triggerRediscovery 触发重新发现
func (rm *SmartRegionManager) triggerRediscovery(reason string) {
	startTime := time.Now()

	rm.mu.Lock()
	defer rm.mu.Unlock()

	totalMarked := 0
	accountCount := 0

	for accountID, regions := range rm.regionMap {
		accountCount++
		for region, info := range regions {
			if info.Status == RegionStatusActive || info.Status == RegionStatusEmpty {
				info.Status = RegionStatusUnknown
				info.EmptyCount = 0
				info.Priority = 50
				regions[region] = info
				totalMarked++
			}
		}
		rm.regionMap[accountID] = regions
	}

	// 更新统计信息
	now := time.Now()
	rm.statsMu.Lock()
	rm.stats.LastRediscoveryTime = now
	atomic.AddInt64(&rm.stats.RediscoveryCount, 1)
	rm.statsMu.Unlock()

	// 记录 Prometheus 指标
	duration := time.Since(startTime).Seconds()
	metrics.RegionRediscoveryTotal.WithLabelValues(reason).Inc()
	metrics.RegionRediscoveryDuration.Observe(duration)
	metrics.RegionRediscoveryMarkedTotal.Set(float64(totalMarked))

	ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Rediscovery")
	ctxLog.Infof("区域重新发现完成 原因=%s 账号数=%d 标记区域数=%d 耗时=%.3fs",
		reason, accountCount, totalMarked, duration)
}

// GetStats 获取统计信息（优化版本，实时计算所有统计数据）
func (rm *SmartRegionManager) GetStats() RegionManagerStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	activeCount := 0
	emptyCount := 0
	unknownCount := 0
	skippedCount := 0 // 实时计算，不累积

	for _, regions := range rm.regionMap {
		for _, info := range regions {
			switch info.Status {
			case RegionStatusActive:
				activeCount++
			case RegionStatusEmpty:
				emptyCount++
				// 实时计算是否会被跳过
				if info.EmptyCount >= rm.config.EmptyThreshold {
					skippedCount++
				}
			case RegionStatusUnknown:
				unknownCount++
			}
		}
	}

	return RegionManagerStats{
		TotalAccounts:       len(rm.regionMap),
		TotalRegions:        activeCount + emptyCount + unknownCount,
		ActiveRegions:       activeCount,
		EmptyRegions:        emptyCount,
		UnknownRegions:      unknownCount,
		SkippedRegions:      skippedCount,
		LastCleanupTime:     rm.stats.LastCleanupTime,
		LastRediscoveryTime: rm.stats.LastRediscoveryTime,
		UpdateCount:         atomic.LoadInt64(&rm.stats.UpdateCount),
		RediscoveryCount:    atomic.LoadInt64(&rm.stats.RediscoveryCount),
		RemoteUpdateCount:   atomic.LoadInt64(&rm.stats.RemoteUpdateCount),
	}
}

// CleanupInactiveAccounts 清理不活跃账号
func (rm *SmartRegionManager) CleanupInactiveAccounts(olderThan time.Duration) int {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if olderThan <= 0 {
		return 0
	}

	now := time.Now()
	toDelete := make([]string, 0)

	for accountID, regions := range rm.regionMap {
		hasRecentActivity := false
		for _, info := range regions {
			if now.Sub(info.LastSeen) < olderThan {
				hasRecentActivity = true
				break
			}
		}

		if !hasRecentActivity && len(regions) > 0 {
			toDelete = append(toDelete, accountID)
		}
	}

	for _, accountID := range toDelete {
		delete(rm.regionMap, accountID)
	}

	rm.statsMu.Lock()
	rm.stats.LastCleanupTime = now
	rm.statsMu.Unlock()

	if len(toDelete) > 0 {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Cleanup")
		ctxLog.Infof("清理了 %d 个不活跃账号（超过 %v 未活跃）", len(toDelete), olderThan)
	}

	return len(toDelete)
}
