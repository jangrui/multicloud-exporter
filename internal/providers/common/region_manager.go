// Package common 提供先进的智能区域发现和管理功能
// 特性：
// - 智能区域选择（优先活跃区域，跳过空区域）
// - 自动内存管理和清理
// - 持久化支持
// - 并发安全
// - 性能监控
// - 优雅停止
package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"multicloud-exporter/internal/logger"
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
	DataDir              string        `json:"data_dir"`                // 数据目录
	PersistFile          string        `json:"persist_file"`            // 持久化文件
	MaxAccounts          int           `json:"max_accounts"`            // 最大账号数（0=无限制）
	CleanupInterval      time.Duration `json:"cleanup_interval"`        // 清理间隔
	MaxRegionsPerAccount int           `json:"max_regions_per_account"` // 每账号最大区域数
}

// RegionManagerStats 统计信息
type RegionManagerStats struct {
	TotalAccounts   int       `json:"total_accounts"`
	TotalRegions    int       `json:"total_regions"`
	ActiveRegions   int       `json:"active_regions"`
	EmptyRegions    int       `json:"empty_regions"`
	UnknownRegions  int       `json:"unknown_regions"`
	SkippedRegions  int       `json:"skipped_regions"`
	LastCleanupTime time.Time `json:"last_cleanup_time"`
	LastSaveTime    time.Time `json:"last_save_time"`
	LastLoadTime    time.Time `json:"last_load_time"`
	SaveCount       int64     `json:"save_count"`
	LoadCount       int64     `json:"load_count"`
	UpdateCount     int64     `json:"update_count"`
}

// RegionManager 区域管理器接口
type RegionManager interface {
	// GetActiveRegions 获取活跃区域列表
	GetActiveRegions(accountID string, allRegions []string) []string

	// UpdateRegionStatus 更新区域状态
	UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus)

	// MarkRegionForRediscovery 标记区域为需重新发现
	MarkRegionForRediscovery(accountID, region string)

	// GetRegionInfo 获取区域信息
	GetRegionInfo(accountID, region string) (RegionInfo, bool)

	// ShouldSkipRegion 判断是否跳过该区域
	ShouldSkipRegion(accountID, region string) bool

	// Load 加载持久化状态
	Load() error

	// Save 保存状态
	Save() error

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

	// 统计信息
	stats   RegionManagerStats
	statsMu sync.RWMutex
}

// NewRegionManager 创建区域管理器
func NewRegionManager(config RegionDiscoveryConfig) RegionManager {
	// 设置默认值
	if config.DiscoveryInterval <= 0 {
		config.DiscoveryInterval = 24 * time.Hour
	}
	if config.EmptyThreshold <= 0 {
		config.EmptyThreshold = 3
	}
	if config.DataDir == "" {
		config.DataDir = "/app/data"
	}
	if config.PersistFile == "" {
		config.PersistFile = "region_status.json"
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
			LastCleanupTime: time.Now(),
		},
	}

	// 创建数据目录
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Initialization")
		ctxLog.Warnf("创建区域状态目录失败: %v", err)
	}

	return rm
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

// UpdateRegionStatus 更新区域状态
func (rm *SmartRegionManager) UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus) {
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

	info.Status = status
	info.LastSeen = now
	info.ResourceCount = resourceCount

	switch status {
	case RegionStatusActive:
		info.LastActive = now
		info.EmptyCount = 0
		info.Priority = 100 // 活跃区域优先级最高
	case RegionStatusEmpty:
		info.EmptyCount++
		info.Priority = 10 // 空区域优先级降低
	case RegionStatusUnknown:
		info.EmptyCount = 0
		info.Priority = 50 // 未知区域中等优先级
	}

	// 限制每账号区域数量
	if len(rm.regionMap[accountID]) >= rm.config.MaxRegionsPerAccount {
		// 清理最旧的低优先级区域
		// 注意：此方法在持锁状态下调用，不会导致锁重入
		rm.evictLowPriorityRegionsLocked(accountID)
	}

	rm.regionMap[accountID][region] = info
}

// evictLowPriorityRegionsLocked 驱逐低优先级区域（必须在持锁状态下调用）
func (rm *SmartRegionManager) evictLowPriorityRegionsLocked(accountID string) {
	regions := rm.regionMap[accountID]
	if len(regions) <= rm.config.MaxRegionsPerAccount {
		return
	}

	// 找出最旧的空区域
	var oldestRegion string
	var oldestTime time.Time
	for region, info := range regions {
		if info.Status == RegionStatusEmpty && info.EmptyCount >= rm.config.EmptyThreshold {
			if oldestRegion == "" || info.LastSeen.Before(oldestTime) {
				oldestRegion = region
				oldestTime = info.LastSeen
			}
		}
	}

	if oldestRegion != "" {
		delete(regions, oldestRegion)
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Eviction", "account_id", accountID, "region", oldestRegion)
		ctxLog.Infof("驱逐旧区域")
	}
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

// Load 加载持久化状态
func (rm *SmartRegionManager) Load() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	atomic.AddInt64(&rm.stats.LoadCount, 1)

	persistPath := filepath.Join(rm.config.DataDir, rm.config.PersistFile)

	data, err := os.ReadFile(persistPath)
	if err != nil {
		if !os.IsNotExist(err) {
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
			ctxLog.Warnf("加载区域状态失败: %v", err)
			return err
		}
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Infof("区域状态文件不存在: %s", persistPath)
		return nil
	}

	var persisted struct {
		RegionMap map[string]map[string]RegionInfo `json:"region_map"`
	}

	if err := json.Unmarshal(data, &persisted); err != nil {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Errorf("解析区域状态失败: %v", err)
		return err
	}

	if persisted.RegionMap != nil {
		rm.regionMap = persisted.RegionMap
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Infof("成功加载区域状态，账号数=%d", len(rm.regionMap))
	}

	rm.statsMu.Lock()
	rm.stats.LastLoadTime = time.Now()
	rm.statsMu.Unlock()

	return nil
}

// Save 保存状态（优化版本，带重试机制）
func (rm *SmartRegionManager) Save() error {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := rm.trySave()
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt < maxRetries-1 {
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
			ctxLog.Warnf("保存区域状态失败（第 %d 次重试，%v 后重试）: %v", attempt+1, backoff, err)
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("保存失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// trySave 尝试保存状态（内部方法）
func (rm *SmartRegionManager) trySave() error {
	// 创建快照避免长时间持锁
	rm.mu.RLock()
	snapshot := rm.createSnapshot()
	rm.mu.RUnlock()

	if rm.config.PersistFile == "" || rm.config.DataDir == "" {
		return nil
	}

	persistPath := filepath.Join(rm.config.DataDir, rm.config.PersistFile)

	data, err := json.MarshalIndent(struct {
		RegionMap map[string]map[string]RegionInfo `json:"region_map"`
		UpdatedAt time.Time                        `json:"updated_at"`
	}{
		RegionMap: snapshot,
		UpdatedAt: time.Now(),
	}, "", "  ")

	if err != nil {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Errorf("序列化区域状态失败: %v", err)
		return err
	}

	if err := os.MkdirAll(rm.config.DataDir, 0755); err != nil {
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Errorf("创建持久化目录失败: %v", err)
		return err
	}

	// 原子写入
	tmpFile := persistPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		// 清理失败的临时文件
		_ = os.Remove(tmpFile)
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Errorf("写入临时文件失败: %v", err)
		return err
	}

	if err := os.Rename(tmpFile, persistPath); err != nil {
		// 清理临时文件
		_ = os.Remove(tmpFile)
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
		ctxLog.Errorf("重命名文件失败: %v", err)
		return err
	}

	atomic.AddInt64(&rm.stats.SaveCount, 1)
	rm.statsMu.Lock()
	rm.stats.LastSaveTime = time.Now()
	rm.statsMu.Unlock()

	ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
	ctxLog.Debugf("成功保存区域状态，账号数=%d", len(snapshot))
	return nil
}

// createSnapshot 创建快照（优化版本，限制大小防止性能问题）
func (rm *SmartRegionManager) createSnapshot() map[string]map[string]RegionInfo {
	const maxSnapshotSize = 5000 // 快照上限，防止 OOM 和性能问题

	snapshot := make(map[string]map[string]RegionInfo, len(rm.regionMap))
	count := 0

	for accountID, regions := range rm.regionMap {
		if count >= maxSnapshotSize {
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Snapshot")
			ctxLog.Warnf("快照达到上限 %d，停止拷贝", maxSnapshotSize)
			break
		}

		snapshot[accountID] = make(map[string]RegionInfo, len(regions))
		for region, info := range regions {
			snapshot[accountID][region] = info
			count++
		}
	}

	return snapshot
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

	// 1. 检查是否需要触发重新发现（从上次清理时间推算）
	timeSinceCleanup := now.Sub(rm.stats.LastCleanupTime)
	if timeSinceCleanup >= rm.config.DiscoveryInterval {
		rm.triggerRediscovery()
	}

	// 2. 检查是否需要清理不活跃账号
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

	// 异步保存，避免阻塞程序退出
	done := make(chan struct{})
	go func() {
		if err := rm.Save(); err != nil {
			ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Persistence")
			ctxLog.Errorf("停止时保存区域状态失败: %v", err)
		}
		close(done)
	}()

	// 等待保存完成或超时（1秒）
	select {
	case <-done:
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Shutdown")
		ctxLog.Infof("区域管理器已停止，状态已保存")
	case <-time.After(1 * time.Second):
		ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Shutdown")
		ctxLog.Warnf("区域管理器停止超时，放弃保存状态")
	}
}

// triggerRediscovery 触发重新发现
func (rm *SmartRegionManager) triggerRediscovery() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	totalMarked := 0

	for accountID, regions := range rm.regionMap {
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

	ctxLog := logger.NewContextLogger("RegionManager", "resource_type", "Rediscovery")
	ctxLog.Infof("区域重新发现完成，标记 %d 个区域为 unknown", totalMarked)
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
		TotalAccounts:   len(rm.regionMap),
		TotalRegions:    activeCount + emptyCount + unknownCount,
		ActiveRegions:   activeCount,
		EmptyRegions:    emptyCount,
		UnknownRegions:  unknownCount,
		SkippedRegions:  skippedCount, // 实时计算，不累积
		LastCleanupTime: rm.stats.LastCleanupTime,
		LastSaveTime:    rm.stats.LastSaveTime,
		LastLoadTime:    rm.stats.LastLoadTime,
		SaveCount:       atomic.LoadInt64(&rm.stats.SaveCount),
		LoadCount:       atomic.LoadInt64(&rm.stats.LoadCount),
		UpdateCount:     atomic.LoadInt64(&rm.stats.UpdateCount),
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
