package degradation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"multicloud-exporter/internal/metrics"
)

// Dimension 维度类型
type Dimension string

const (
	DimensionAccount  Dimension = "account"
	DimensionProduct  Dimension = "product"
	DimensionRegion   Dimension = "region"
	DimensionResource Dimension = "resource"
)

// DegradationState 降级状态
type DegradationState string

const (
	DegradationStateActive   DegradationState = "active"   // 正常
	DegradationStateDisabled DegradationState = "disabled" // 已禁用
)

// DegradationInfo 降级信息
type DegradationInfo struct {
	mu               sync.RWMutex
	Status           DegradationState
	FailureCount     int
	FirstFailureTime time.Time
	LastFailureTime  time.Time
	LastSuccessTime  time.Time
	DisabledTime     time.Time
	DisabledReason   string
	RecoveryAttempts int
	LastRecoveryTime time.Time
}

// NewDegradationInfo 创建降级信息
func NewDegradationInfo() *DegradationInfo {
	return &DegradationInfo{
		Status:           DegradationStateActive,
		FirstFailureTime: time.Time{},
		LastSuccessTime:  time.Now(),
	}
}

// IsActive 检查是否活跃
func (d *DegradationInfo) IsActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Status == DegradationStateActive
}

// IsDisabled 检查是否已禁用
func (d *DegradationInfo) IsDisabled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Status == DegradationStateDisabled
}

// GetStatus 获取状态
func (d *DegradationInfo) GetStatus() DegradationState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Status
}

// RecordFailure 记录失败
func (d *DegradationInfo) RecordFailure(reason string) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.FailureCount++
	d.LastFailureTime = time.Now()

	if d.FirstFailureTime.IsZero() {
		d.FirstFailureTime = d.LastFailureTime
	}

	return d.FailureCount
}

// RecordSuccess 记录成功
func (d *DegradationInfo) RecordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.FailureCount = 0
	d.LastSuccessTime = time.Now()
	d.FirstFailureTime = time.Time{}
}

// Disable 禁用
func (d *DegradationInfo) Disable(reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Status != DegradationStateDisabled {
		d.Status = DegradationStateDisabled
		d.DisabledTime = time.Now()
		d.DisabledReason = reason
	}
}

// Enable 启用
func (d *DegradationInfo) Enable() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Status != DegradationStateActive {
		d.Status = DegradationStateActive
		d.DisabledTime = time.Time{}
		d.DisabledReason = ""
	}
}

// GetDisabledDuration 获取禁用持续时间
func (d *DegradationInfo) GetDisabledDuration() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.DisabledTime.IsZero() {
		return 0
	}
	return time.Since(d.DisabledTime)
}

// DegradationConfig 降级配置
type DegradationConfig struct {
	MaxFailures      int           // 最大失败次数，默认 3
	FailureWindow    time.Duration // 失败时间窗口，默认 5 分钟
	RecoveryInterval time.Duration // 自动恢复检查间隔，默认 10 分钟
	RecoveryTimeout  time.Duration // 恢复尝试超时时间，默认 30 秒
}

// DefaultDegradationConfig 默认降级配置
func DefaultDegradationConfig() DegradationConfig {
	return DegradationConfig{
		MaxFailures:      3,
		FailureWindow:    5 * time.Minute,
		RecoveryInterval: 10 * time.Minute,
		RecoveryTimeout:  30 * time.Second,
	}
}

// FourDimensionDegradationManager 四维降级管理器
type FourDimensionDegradationManager struct {
	mu     sync.RWMutex
	logger *zap.Logger
	config DegradationConfig
	ctx    context.Context
	cancel context.CancelFunc

	accountDegradations  map[string]*DegradationInfo // accountID -> DegradationInfo
	productDegradations  map[string]*DegradationInfo // accountID:productID -> DegradationInfo
	regionDegradations   map[string]*DegradationInfo // accountID:regionID -> DegradationInfo
	resourceDegradations map[string]*DegradationInfo // accountID:resourceID -> DegradationInfo
}

// NewFourDimensionDegradationManager 创建四维降级管理器
func NewFourDimensionDegradationManager(logger *zap.Logger, config DegradationConfig) *FourDimensionDegradationManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &FourDimensionDegradationManager{
		logger:               logger,
		config:               config,
		ctx:                  ctx,
		cancel:               cancel,
		accountDegradations:  make(map[string]*DegradationInfo),
		productDegradations:  make(map[string]*DegradationInfo),
		regionDegradations:   make(map[string]*DegradationInfo),
		resourceDegradations: make(map[string]*DegradationInfo),
	}
}

// StartRecoveryScheduler 启动自动恢复调度器
func (m *FourDimensionDegradationManager) StartRecoveryScheduler() {
	ticker := time.NewTicker(m.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("recovery scheduler stopped")
			return
		case <-ticker.C:
			m.recoverAllDegradedResources()
		}
	}
}

// Stop 停止降级管理器
func (m *FourDimensionDegradationManager) Stop() {
	m.cancel()
}

// recoverAllDegradedResources 恢复所有降级资源
func (m *FourDimensionDegradationManager) recoverAllDegradedResources() {
	m.logger.Debug("starting recovery for all degraded resources")

	// 恢复账号
	m.recoverDegradedAccounts()
	// 恢复产品
	m.recoverDegradedProducts()
	// 恢复区域
	m.recoverDegradedRegions()
	// 恢复资源
	m.recoverDegradedResources()
}

// ========== 账号级降级 ==========

// RecordAccountFailure 记录账号失败
func (m *FourDimensionDegradationManager) RecordAccountFailure(accountID, reason string) bool {
	m.mu.Lock()
	info, exists := m.accountDegradations[accountID]
	if !exists {
		info = NewDegradationInfo()
		m.accountDegradations[accountID] = info
	}
	m.mu.Unlock()

	failureCount := info.RecordFailure(reason)
	metrics.DegradationTotal.WithLabelValues(string(DimensionAccount), reason).Inc()

	// 检查是否超过最大失败次数
	if failureCount >= m.config.MaxFailures {
		info.Disable(fmt.Sprintf("exceeded max failures: %d", failureCount))
		m.logger.Warn("account degraded",
			zap.String("account_id", accountID),
			zap.Int("failure_count", failureCount),
			zap.String("reason", reason),
		)
		metrics.UpdateDegradedResources(string(DimensionAccount), 1)
		return true
	}

	return false
}

// RecordAccountSuccess 记录账号成功
func (m *FourDimensionDegradationManager) RecordAccountSuccess(accountID string) {
	m.mu.Lock()
	info, exists := m.accountDegradations[accountID]
	if !exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	info.RecordSuccess()
}

// IsAccountDisabled 检查账号是否已禁用
func (m *FourDimensionDegradationManager) IsAccountDisabled(accountID string) bool {
	m.mu.RLock()
	info, exists := m.accountDegradations[accountID]
	m.mu.RUnlock()

	return exists && info.IsDisabled()
}

// RecoverAccount 恢复账号
func (m *FourDimensionDegradationManager) RecoverAccount(accountID string) bool {
	m.mu.Lock()
	info, exists := m.accountDegradations[accountID]
	if !exists {
		m.mu.Unlock()
		return false
	}
	m.mu.Unlock()

	if !info.IsDisabled() {
		return false
	}

	info.Enable()
	info.RecoveryAttempts++
	info.LastRecoveryTime = time.Now()

	duration := info.GetDisabledDuration()
	metrics.DegradationRecoveredTotal.WithLabelValues(string(DimensionAccount)).Inc()
	metrics.DegradationDurationSeconds.WithLabelValues(string(DimensionAccount)).Observe(duration.Seconds())
	metrics.UpdateDegradedResources(string(DimensionAccount), -1)

	m.logger.Info("account recovered",
		zap.String("account_id", accountID),
		zap.Duration("disabled_duration", duration),
	)

	return true
}

// ========== 产品级降级 ==========

// RecordProductFailure 记录产品失败
func (m *FourDimensionDegradationManager) RecordProductFailure(accountID, productID, reason string) bool {
	key := fmt.Sprintf("%s:%s", accountID, productID)
	m.mu.Lock()
	info, exists := m.productDegradations[key]
	if !exists {
		info = NewDegradationInfo()
		m.productDegradations[key] = info
	}
	m.mu.Unlock()

	failureCount := info.RecordFailure(reason)
	metrics.DegradationTotal.WithLabelValues(string(DimensionProduct), reason).Inc()

	if failureCount >= m.config.MaxFailures {
		info.Disable(fmt.Sprintf("exceeded max failures: %d", failureCount))
		m.logger.Warn("product degraded",
			zap.String("account_id", accountID),
			zap.String("product_id", productID),
			zap.Int("failure_count", failureCount),
			zap.String("reason", reason),
		)
		metrics.UpdateDegradedResources(string(DimensionProduct), 1)
		return true
	}

	return false
}

// RecordProductSuccess 记录产品成功
func (m *FourDimensionDegradationManager) RecordProductSuccess(accountID, productID string) {
	key := fmt.Sprintf("%s:%s", accountID, productID)
	m.mu.Lock()
	info, exists := m.productDegradations[key]
	if !exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	info.RecordSuccess()
}

// IsProductDisabled 检查产品是否已禁用
func (m *FourDimensionDegradationManager) IsProductDisabled(accountID, productID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, productID)
	m.mu.RLock()
	info, exists := m.productDegradations[key]
	m.mu.RUnlock()

	return exists && info.IsDisabled()
}

// RecoverProduct 恢复产品
func (m *FourDimensionDegradationManager) RecoverProduct(accountID, productID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, productID)
	m.mu.Lock()
	info, exists := m.productDegradations[key]
	if !exists {
		m.mu.Unlock()
		return false
	}
	m.mu.Unlock()

	if !info.IsDisabled() {
		return false
	}

	info.Enable()
	info.RecoveryAttempts++
	info.LastRecoveryTime = time.Now()

	duration := info.GetDisabledDuration()
	metrics.DegradationRecoveredTotal.WithLabelValues(string(DimensionProduct)).Inc()
	metrics.DegradationDurationSeconds.WithLabelValues(string(DimensionProduct)).Observe(duration.Seconds())
	metrics.UpdateDegradedResources(string(DimensionProduct), -1)

	m.logger.Info("product recovered",
		zap.String("account_id", accountID),
		zap.String("product_id", productID),
		zap.Duration("disabled_duration", duration),
	)

	return true
}

// ========== 区域级降级 ==========

// RecordRegionFailure 记录区域失败
func (m *FourDimensionDegradationManager) RecordRegionFailure(accountID, regionID, reason string) bool {
	key := fmt.Sprintf("%s:%s", accountID, regionID)
	m.mu.Lock()
	info, exists := m.regionDegradations[key]
	if !exists {
		info = NewDegradationInfo()
		m.regionDegradations[key] = info
	}
	m.mu.Unlock()

	failureCount := info.RecordFailure(reason)
	metrics.DegradationTotal.WithLabelValues(string(DimensionRegion), reason).Inc()

	if failureCount >= m.config.MaxFailures {
		info.Disable(fmt.Sprintf("exceeded max failures: %d", failureCount))
		m.logger.Warn("region degraded",
			zap.String("account_id", accountID),
			zap.String("region_id", regionID),
			zap.Int("failure_count", failureCount),
			zap.String("reason", reason),
		)
		metrics.UpdateDegradedResources(string(DimensionRegion), 1)
		return true
	}

	return false
}

// RecordRegionSuccess 记录区域成功
func (m *FourDimensionDegradationManager) RecordRegionSuccess(accountID, regionID string) {
	key := fmt.Sprintf("%s:%s", accountID, regionID)
	m.mu.Lock()
	info, exists := m.regionDegradations[key]
	if !exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	info.RecordSuccess()
}

// IsRegionDisabled 检查区域是否已禁用
func (m *FourDimensionDegradationManager) IsRegionDisabled(accountID, regionID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, regionID)
	m.mu.RLock()
	info, exists := m.regionDegradations[key]
	m.mu.RUnlock()

	return exists && info.IsDisabled()
}

// RecoverRegion 恢复区域
func (m *FourDimensionDegradationManager) RecoverRegion(accountID, regionID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, regionID)
	m.mu.Lock()
	info, exists := m.regionDegradations[key]
	if !exists {
		m.mu.Unlock()
		return false
	}
	m.mu.Unlock()

	if !info.IsDisabled() {
		return false
	}

	info.Enable()
	info.RecoveryAttempts++
	info.LastRecoveryTime = time.Now()

	duration := info.GetDisabledDuration()
	metrics.DegradationRecoveredTotal.WithLabelValues(string(DimensionRegion)).Inc()
	metrics.DegradationDurationSeconds.WithLabelValues(string(DimensionRegion)).Observe(duration.Seconds())
	metrics.UpdateDegradedResources(string(DimensionRegion), -1)

	m.logger.Info("region recovered",
		zap.String("account_id", accountID),
		zap.String("region_id", regionID),
		zap.Duration("disabled_duration", duration),
	)

	return true
}

// ========== 资源级降级 ==========

// RecordResourceFailure 记录资源失败
func (m *FourDimensionDegradationManager) RecordResourceFailure(accountID, resourceID, reason string) bool {
	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	m.mu.Lock()
	info, exists := m.resourceDegradations[key]
	if !exists {
		info = NewDegradationInfo()
		m.resourceDegradations[key] = info
	}
	m.mu.Unlock()

	failureCount := info.RecordFailure(reason)
	metrics.DegradationTotal.WithLabelValues(string(DimensionResource), reason).Inc()

	if failureCount >= m.config.MaxFailures {
		info.Disable(fmt.Sprintf("exceeded max failures: %d", failureCount))
		m.logger.Warn("resource degraded",
			zap.String("account_id", accountID),
			zap.String("resource_id", resourceID),
			zap.Int("failure_count", failureCount),
			zap.String("reason", reason),
		)
		metrics.UpdateDegradedResources(string(DimensionResource), 1)
		return true
	}

	return false
}

// RecordResourceSuccess 记录资源成功
func (m *FourDimensionDegradationManager) RecordResourceSuccess(accountID, resourceID string) {
	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	m.mu.Lock()
	info, exists := m.resourceDegradations[key]
	if !exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	info.RecordSuccess()
}

// IsResourceDisabled 检查资源是否已禁用
func (m *FourDimensionDegradationManager) IsResourceDisabled(accountID, resourceID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	m.mu.RLock()
	info, exists := m.resourceDegradations[key]
	m.mu.RUnlock()

	return exists && info.IsDisabled()
}

// RecoverResource 恢复资源
func (m *FourDimensionDegradationManager) RecoverResource(accountID, resourceID string) bool {
	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	m.mu.Lock()
	info, exists := m.resourceDegradations[key]
	if !exists {
		m.mu.Unlock()
		return false
	}
	m.mu.Unlock()

	if !info.IsDisabled() {
		return false
	}

	info.Enable()
	info.RecoveryAttempts++
	info.LastRecoveryTime = time.Now()

	duration := info.GetDisabledDuration()
	metrics.DegradationRecoveredTotal.WithLabelValues(string(DimensionResource)).Inc()
	metrics.DegradationDurationSeconds.WithLabelValues(string(DimensionResource)).Observe(duration.Seconds())
	metrics.UpdateDegradedResources(string(DimensionResource), -1)

	m.logger.Info("resource recovered",
		zap.String("account_id", accountID),
		zap.String("resource_id", resourceID),
		zap.Duration("disabled_duration", duration),
	)

	return true
}

// ========== 自动恢复 ==========

// recoverDegradedAccounts 恢复所有降级的账号
func (m *FourDimensionDegradationManager) recoverDegradedAccounts() {
	m.mu.RLock()
	accountIDs := make([]string, 0, len(m.accountDegradations))
	for accountID := range m.accountDegradations {
		accountIDs = append(accountIDs, accountID)
	}
	m.mu.RUnlock()

	for _, accountID := range accountIDs {
		if m.IsAccountDisabled(accountID) {
			m.RecoverAccount(accountID)
		}
	}
}

// recoverDegradedProducts 恢复所有降级的产品
func (m *FourDimensionDegradationManager) recoverDegradedProducts() {
	m.mu.RLock()
	keys := make([]string, 0, len(m.productDegradations))
	for key := range m.productDegradations {
		keys = append(keys, key)
	}
	m.mu.RUnlock()

	for _, key := range keys {
		parts := splitKey(key)
		if len(parts) == 2 {
			accountID, productID := parts[0], parts[1]
			if m.IsProductDisabled(accountID, productID) {
				m.RecoverProduct(accountID, productID)
			}
		}
	}
}

// recoverDegradedRegions 恢复所有降级的区域
func (m *FourDimensionDegradationManager) recoverDegradedRegions() {
	m.mu.RLock()
	keys := make([]string, 0, len(m.regionDegradations))
	for key := range m.regionDegradations {
		keys = append(keys, key)
	}
	m.mu.RUnlock()

	for _, key := range keys {
		parts := splitKey(key)
		if len(parts) == 2 {
			accountID, regionID := parts[0], parts[1]
			if m.IsRegionDisabled(accountID, regionID) {
				m.RecoverRegion(accountID, regionID)
			}
		}
	}
}

// recoverDegradedResources 恢复所有降级的资源
func (m *FourDimensionDegradationManager) recoverDegradedResources() {
	m.mu.RLock()
	keys := make([]string, 0, len(m.resourceDegradations))
	for key := range m.resourceDegradations {
		keys = append(keys, key)
	}
	m.mu.RUnlock()

	for _, key := range keys {
		parts := splitKey(key)
		if len(parts) == 2 {
			accountID, resourceID := parts[0], parts[1]
			if m.IsResourceDisabled(accountID, resourceID) {
				m.RecoverResource(accountID, resourceID)
			}
		}
	}
}

// splitKey 分割键
func splitKey(key string) []string {
	parts := make([]string, 0, 2)
	start := 0
	for i, r := range key {
		if r == ':' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}

// GetDegradationStats 获取降级统计信息
func (m *FourDimensionDegradationManager) GetDegradationStats() map[Dimension]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[Dimension]int)

	for _, info := range m.accountDegradations {
		if info.IsDisabled() {
			stats[DimensionAccount]++
		}
	}

	for _, info := range m.productDegradations {
		if info.IsDisabled() {
			stats[DimensionProduct]++
		}
	}

	for _, info := range m.regionDegradations {
		if info.IsDisabled() {
			stats[DimensionRegion]++
		}
	}

	for _, info := range m.resourceDegradations {
		if info.IsDisabled() {
			stats[DimensionResource]++
		}
	}

	return stats
}
