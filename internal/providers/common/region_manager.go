// Package common 提供智能区域发现功能
// 用于管理云厂商区域状态，智能选择有资源的区域，避免重复访问空区域
package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"multicloud-exporter/internal/logger"
)

// RegionStatus 定义区域状态
type RegionStatus string

const (
	RegionStatusUnknown RegionStatus = "unknown" // 未知，首次运行
	RegionStatusActive  RegionStatus = "active"   // 有资源
	RegionStatusEmpty   RegionStatus = "empty"    // 无资源
)

// RegionInfo 定义区域信息
type RegionInfo struct {
	Status         RegionStatus `json:"status"`
	LastSeen       time.Time   `json:"last_seen"`       // 最后一次检查时间
	LastActive     time.Time   `json:"last_active"`     // 最后一次发现资源的时间
	EmptyCount     int        `json:"empty_count"`      // 连续为空的次数
	ResourceCount  int        `json:"resource_count"`   // 最后一次发现的资源数量
}

// RegionDiscoveryConfig 定义区域发现配置
type RegionDiscoveryConfig struct {
	Enabled          bool          `json:"enabled"`           // 是否启用智能发现
	DiscoveryInterval time.Duration `json:"discovery_interval"` // 重新发现周期
	EmptyThreshold   int           `json:"empty_threshold"`    // 空区域跳过阈值（连续空次数）
	PersistFile      string        `json:"persist_file"`      // 持久化文件路径
}

// RegionManager 区域管理器接口
type RegionManager interface {
	// GetActiveRegions 获取活跃的区域列表（优先返回 active 状态的区域）
	GetActiveRegions(accountID string, allRegions []string) []string

	// UpdateRegionStatus 更新区域状态
	UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus)

	// MarkRegionForRediscovery 标记区域为需要重新发现
	MarkRegionForRediscovery(accountID, region string)

	// GetRegionInfo 获取区域信息
	GetRegionInfo(accountID, region string) (RegionInfo, bool)

	// ShouldSkipRegion 判断是否应该跳过该区域
	ShouldSkipRegion(accountID, region string) bool

	// Load 加载持久化的区域状态
	Load() error

	// Save 保存区域状态到文件
	Save() error

	// StartRediscoveryScheduler 启动定期重新发现调度器
	StartRediscoveryScheduler()

	// StopRediscoveryScheduler 停止定期重新发现调度器
	StopRediscoveryScheduler()
}

// DefaultRegionManager 默认的区域管理器实现
type DefaultRegionManager struct {
	mu            sync.RWMutex
	config         RegionDiscoveryConfig
	regionMap     map[string]map[string]RegionInfo // accountID -> region -> RegionInfo
	stopChan      chan struct{}
	schedulerOnce sync.Once
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
	if config.PersistFile == "" {
		config.PersistFile = "./data/region_status.json"
	}

	rm := &DefaultRegionManager{
		config:      config,
		regionMap:   make(map[string]map[string]RegionInfo),
		stopChan:    make(chan struct{}),
	}

	// 确保持久化目录存在
	if err := os.MkdirAll(filepath.Dir(config.PersistFile), 0755); err != nil {
		logger.Log.Warnf("创建区域状态持久化目录失败: %v", err)
	}

	return rm
}

// GetActiveRegions 获取活跃的区域列表
// 优先返回 active 状态的区域，然后是 unknown 状态的区域
// 跳过超过阈值的 empty 状态的区域
func (rm *DefaultRegionManager) GetActiveRegions(accountID string, allRegions []string) []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if !rm.config.Enabled {
		// 如果未启用智能发现，返回所有区域
		return allRegions
	}

	accountRegions, ok := rm.regionMap[accountID]
	if !ok {
		// 账号不存在，返回所有区域（首次运行）
		return allRegions
	}

	var activeRegions []string
	var unknownRegions []string

	for _, region := range allRegions {
		info, ok := accountRegions[region]
		if !ok {
			// 区域不存在，作为 unknown 处理
			unknownRegions = append(unknownRegions, region)
			continue
		}

		switch info.Status {
		case RegionStatusActive:
			// 活跃区域，优先返回
			activeRegions = append(activeRegions, region)
		case RegionStatusUnknown:
			// 未知区域，也返回（可能是新区域）
			unknownRegions = append(unknownRegions, region)
		case RegionStatusEmpty:
			// 空区域，检查是否超过阈值
			if info.EmptyCount < rm.config.EmptyThreshold {
				// 未超过阈值，仍然返回
				unknownRegions = append(unknownRegions, region)
			} else {
				// 超过阈值，跳过
				logger.Log.Debugf("跳过空区域 account=%s region=%s empty_count=%d threshold=%d",
					accountID, region, info.EmptyCount, rm.config.EmptyThreshold)
			}
		}
	}

	// 优先返回活跃区域，然后返回未知区域
	result := append(activeRegions, unknownRegions...)

	if len(result) == 0 {
		// 如果没有任何区域返回，返回所有区域（兜底）
		logger.Log.Warnf("没有可用区域，返回全部区域 account=%s", accountID)
		return allRegions
	}

	logger.Log.Infof("智能区域选择 account=%s 总区域=%d 活跃=%d 未知=%d 跳过=%d",
		accountID, len(allRegions), len(activeRegions), len(unknownRegions), len(allRegions)-len(result))

	return result
}

// UpdateRegionStatus 更新区域状态
func (rm *DefaultRegionManager) UpdateRegionStatus(accountID, region string, resourceCount int, status RegionStatus) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()

	// 初始化账号的区域映射
	if rm.regionMap[accountID] == nil {
		rm.regionMap[accountID] = make(map[string]RegionInfo)
	}

	// 获取现有信息
	info, ok := rm.regionMap[accountID][region]
	if !ok {
		// 新区域
		info = RegionInfo{
			LastSeen:  now,
			EmptyCount: 0,
		}
	}

	// 更新状态
	info.Status = status
	info.LastSeen = now
	info.ResourceCount = resourceCount

	// 根据状态更新其他字段
	switch status {
	case RegionStatusActive:
		info.LastActive = now
		info.EmptyCount = 0
		logger.Log.Debugf("区域状态更新为 active account=%s region=%s resource_count=%d",
			accountID, region, resourceCount)
	case RegionStatusEmpty:
		info.EmptyCount++
		logger.Log.Debugf("区域状态更新为 empty account=%s region=%s empty_count=%d",
			accountID, region, info.EmptyCount)
	case RegionStatusUnknown:
		info.EmptyCount = 0
		logger.Log.Debugf("区域状态更新为 unknown account=%s region=%s", accountID, region)
	}

	// 保存更新后的信息
	rm.regionMap[accountID][region] = info
}

// MarkRegionForRediscovery 标记区域为需要重新发现
func (rm *DefaultRegionManager) MarkRegionForRediscovery(accountID, region string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.regionMap[accountID] == nil {
		return
	}

	if info, ok := rm.regionMap[accountID][region]; ok {
		info.Status = RegionStatusUnknown
		info.EmptyCount = 0
		rm.regionMap[accountID][region] = info

		logger.Log.Infof("标记区域为重新发现 account=%s region=%s", accountID, region)
	}
}

// GetRegionInfo 获取区域信息
func (rm *DefaultRegionManager) GetRegionInfo(accountID, region string) (RegionInfo, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.regionMap[accountID] == nil {
		return RegionInfo{}, false
	}

	info, ok := rm.regionMap[accountID][region]
	return info, ok
}

// ShouldSkipRegion 判断是否应该跳过该区域
func (rm *DefaultRegionManager) ShouldSkipRegion(accountID, region string) bool {
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

	// 如果区域状态为 empty 且连续空次数超过阈值，则跳过
	return info.Status == RegionStatusEmpty && info.EmptyCount >= rm.config.EmptyThreshold
}

// Load 加载持久化的区域状态
func (rm *DefaultRegionManager) Load() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	data, err := os.ReadFile(rm.config.PersistFile)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Log.Warnf("加载区域状态失败: %v", err)
			return err
		}
		// 文件不存在是正常的（首次运行）
		logger.Log.Infof("区域状态文件不存在，将使用空状态: %s", rm.config.PersistFile)
		return nil
	}

	var persisted struct {
		RegionMap map[string]map[string]RegionInfo `json:"region_map"`
	}

	if err := json.Unmarshal(data, &persisted); err != nil {
		logger.Log.Errorf("解析区域状态失败: %v", err)
		return err
	}

	if persisted.RegionMap != nil {
		rm.regionMap = persisted.RegionMap
		logger.Log.Infof("成功加载区域状态，账号数=%d", len(rm.regionMap))
	}

	return nil
}

// Save 保存区域状态到文件
func (rm *DefaultRegionManager) Save() error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.config.PersistFile == "" {
		return nil
	}

	data, err := json.MarshalIndent(struct {
		RegionMap map[string]map[string]RegionInfo `json:"region_map"`
		UpdatedAt  time.Time                        `json:"updated_at"`
	}{
		RegionMap: rm.regionMap,
		UpdatedAt:  time.Now(),
	}, "", "  ")

	if err != nil {
		logger.Log.Errorf("序列化区域状态失败: %v", err)
		return err
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(rm.config.PersistFile), 0755); err != nil {
		logger.Log.Errorf("创建持久化目录失败: %v", err)
		return err
	}

	// 写入临时文件
	tmpFile := rm.config.PersistFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		logger.Log.Errorf("写入区域状态临时文件失败: %v", err)
		return err
	}

	// 原子性重命名
	if err := os.Rename(tmpFile, rm.config.PersistFile); err != nil {
		logger.Log.Errorf("重命名区域状态文件失败: %v", err)
		return err
	}

	logger.Log.Debugf("成功保存区域状态，账号数=%d", len(rm.regionMap))
	return nil
}

// StartRediscoveryScheduler 启动定期重新发现调度器
func (rm *DefaultRegionManager) StartRediscoveryScheduler() {
	rm.schedulerOnce.Do(func() {
		if !rm.config.Enabled || rm.config.DiscoveryInterval <= 0 {
			logger.Log.Infof("区域重新发现调度器未启用")
			return
		}

		logger.Log.Infof("启动区域重新发现调度器，周期=%v", rm.config.DiscoveryInterval)

		go func() {
			ticker := time.NewTicker(rm.config.DiscoveryInterval)
			defer ticker.Stop()

			for {
				select {
				case <-rm.stopChan:
					logger.Log.Infof("停止区域重新发现调度器")
					return
				case <-ticker.C:
					rm.triggerRediscovery()
				}
			}
		}()
	})
}

// StopRediscoveryScheduler 停止定期重新发现调度器
func (rm *DefaultRegionManager) StopRediscoveryScheduler() {
	select {
	case rm.stopChan <- struct{}{}:
		logger.Log.Infof("发送停止信号到区域重新发现调度器")
	default:
		logger.Log.Debug("区域重新发现调度器已停止")
	}
}

// triggerRediscovery 触发重新发现
// 将所有 active 和 empty 状态的区域标记为 unknown
// 这样下一轮采集会重新探测这些区域
func (rm *DefaultRegionManager) triggerRediscovery() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	totalMarked := 0

	for accountID, regions := range rm.regionMap {
		for region, info := range regions {
			if info.Status == RegionStatusActive || info.Status == RegionStatusEmpty {
				info.Status = RegionStatusUnknown
				info.EmptyCount = 0
				regions[region] = info
				totalMarked++
			}
		}
		rm.regionMap[accountID] = regions
	}

	// 保存更新后的状态
	if err := rm.Save(); err != nil {
		logger.Log.Errorf("重新发现后保存区域状态失败: %v", err)
	} else {
		logger.Log.Infof("区域重新发现完成，标记了 %d 个区域为 unknown", totalMarked)
	}
}

// GetStats 获取区域统计信息（用于监控）
func (rm *DefaultRegionManager) GetStats() map[string]int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := map[string]int{
		"active":  0,
		"empty":   0,
		"unknown": 0,
		"total":   0,
	}

	for _, regions := range rm.regionMap {
		for _, info := range regions {
			stats["total"]++
			switch info.Status {
			case RegionStatusActive:
				stats["active"]++
			case RegionStatusEmpty:
				stats["empty"]++
			case RegionStatusUnknown:
				stats["unknown"]++
			}
		}
	}

	return stats
}

