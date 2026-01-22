package fault_tolerance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"multicloud-exporter/internal/metrics"
)

// IsolationStatus 隔离状态
type IsolationStatus int

const (
	// IsolationActive 正常状态，未隔离
	IsolationActive IsolationStatus = iota
	// IsolationIsolated 已隔离
	IsolationIsolated
	// IsolationRecovering 恢复中
	IsolationRecovering
)

func (s IsolationStatus) String() string {
	switch s {
	case IsolationActive:
		return "active"
	case IsolationIsolated:
		return "isolated"
	case IsolationRecovering:
		return "recovering"
	default:
		return "unknown"
	}
}

// IsolationStrategy 隔离策略
type IsolationStrategy int

const (
	// AccountIsolation 账号级隔离
	AccountIsolation IsolationStrategy = iota
	// ProductIsolation 产品级隔离
	ProductIsolation
	// RegionIsolation 区域级隔离
	RegionIsolation
	// ResourceIsolation 资源级隔离
	ResourceIsolation
)

func (s IsolationStrategy) String() string {
	switch s {
	case AccountIsolation:
		return "account"
	case ProductIsolation:
		return "product"
	case RegionIsolation:
		return "region"
	case ResourceIsolation:
		return "resource"
	default:
		return "unknown"
	}
}

// IsolationReason 隔离原因
type IsolationReason int

const (
	// ReasonAPIFailure API 调用失败
	ReasonAPIFailure IsolationReason = iota
	// ReasonTimeout 超时
	ReasonTimeout
	// ReasonRateLimit 限流
	ReasonRateLimit
	// ReasonAuthFailure 认证失败
	ReasonAuthFailure
	// ReasonUnknown 未知原因
	ReasonUnknown
)

func (r IsolationReason) String() string {
	switch r {
	case ReasonAPIFailure:
		return "api_failure"
	case ReasonTimeout:
		return "timeout"
	case ReasonRateLimit:
		return "rate_limit"
	case ReasonAuthFailure:
		return "auth_failure"
	case ReasonUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// IsolationInfo 隔离信息
type IsolationInfo struct {
	Status        IsolationStatus `json:"status"`
	Reason        IsolationReason `json:"reason"`
	FailureCount  int             `json:"failure_count"`
	IsolatedAt    time.Time       `json:"isolated_at"`
	LastFailure   time.Time       `json:"last_failure"`
	LastRecovered time.Time       `json:"last_recovered"`
	mu            sync.RWMutex    `json:"-"`
}

// IsolationInfoMethods 隔离信息方法
type IsolationInfoMethods interface {
	GetStatus() IsolationStatus
	GetReason() IsolationReason
	GetFailureCount() int
	GetIsolatedAt() time.Time
	GetLastFailure() time.Time
	GetLastRecovered() time.Time
	IsIsolated() bool
	IsRecovering() bool
	IncrementFailure()
	UpdateFailure(reason IsolationReason)
	MarkIsolated(reason IsolationReason)
	MarkRecovered()
	MarkRecovering()
}

// GetStatus 获取隔离状态
func (i *IsolationInfo) GetStatus() IsolationStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status
}

// GetReason 获取隔离原因
func (i *IsolationInfo) GetReason() IsolationReason {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Reason
}

// GetFailureCount 获取失败次数
func (i *IsolationInfo) GetFailureCount() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.FailureCount
}

// GetIsolatedAt 获取隔离时间
func (i *IsolationInfo) GetIsolatedAt() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.IsolatedAt
}

// GetLastFailure 获取最后失败时间
func (i *IsolationInfo) GetLastFailure() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.LastFailure
}

// GetLastRecovered 获取最后恢复时间
func (i *IsolationInfo) GetLastRecovered() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.LastRecovered
}

// IsIsolated 检查是否已隔离
func (i *IsolationInfo) IsIsolated() bool {
	return i.GetStatus() == IsolationIsolated
}

// IsRecovering 检查是否正在恢复
func (i *IsolationInfo) IsRecovering() bool {
	return i.GetStatus() == IsolationRecovering
}

// IncrementFailure 增加失败次数
func (i *IsolationInfo) IncrementFailure() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.FailureCount++
	i.LastFailure = time.Now()
}

// UpdateFailure 更新失败信息
func (i *IsolationInfo) UpdateFailure(reason IsolationReason) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.FailureCount++
	i.LastFailure = time.Now()
	i.Reason = reason
}

// MarkIsolated 标记为隔离
func (i *IsolationInfo) MarkIsolated(reason IsolationReason) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Status = IsolationIsolated
	i.Reason = reason
	i.IsolatedAt = time.Now()
	i.LastFailure = time.Now()
}

// MarkRecovered 标记为恢复
func (i *IsolationInfo) MarkRecovered() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Status = IsolationActive
	i.FailureCount = 0
	i.LastRecovered = time.Now()
}

// MarkRecovering 标记为恢复中
func (i *IsolationInfo) MarkRecovering() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Status = IsolationRecovering
}

// IsolationConfig 隔离配置
type IsolationConfig struct {
	// MaxFailures 最大失败次数，超过则隔离
	MaxFailures int
	// FailureWindow 失败时间窗口
	FailureWindow time.Duration
	// RecoveryInterval 恢复检查间隔
	RecoveryInterval time.Duration
	// RecoveryTimeout 恢复检查超时
	RecoveryTimeout time.Duration
	// CascadePropagation 是否级联传播（资源 → 区域 → 产品 → 账号）
	CascadePropagation bool
}

// DefaultIsolationConfig 默认隔离配置
func DefaultIsolationConfig() IsolationConfig {
	return IsolationConfig{
		MaxFailures:        3,
		FailureWindow:      5 * time.Minute,
		RecoveryInterval:   10 * time.Minute,
		RecoveryTimeout:    30 * time.Second,
		CascadePropagation: true,
	}
}

// FourDimensionFaultTolerance 四维容错管理器
type FourDimensionFaultTolerance struct {
	// 账号级隔离
	accountIsolations map[string]*IsolationInfo
	// 产品级隔离
	productIsolations map[string]*IsolationInfo
	// 区域级隔离
	regionIsolations map[string]*IsolationInfo
	// 资源级隔离
	resourceIsolations map[string]*IsolationInfo

	config IsolationConfig
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 并发保护
	mu sync.RWMutex
}

// NewFourDimensionFaultTolerance 创建四维容错管理器
func NewFourDimensionFaultTolerance(config IsolationConfig) *FourDimensionFaultTolerance {
	ctx, cancel := context.WithCancel(context.Background())
	return &FourDimensionFaultTolerance{
		accountIsolations:  make(map[string]*IsolationInfo),
		productIsolations:  make(map[string]*IsolationInfo),
		regionIsolations:   make(map[string]*IsolationInfo),
		resourceIsolations: make(map[string]*IsolationInfo),
		config:             config,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// IsolateAccount 隔离账号
func (m *FourDimensionFaultTolerance) IsolateAccount(accountID string, reason IsolationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.accountIsolations[accountID]
	if !exists {
		info = &IsolationInfo{
			Status:       IsolationActive,
			Reason:       ReasonUnknown,
			FailureCount: 0,
		}
		m.accountIsolations[accountID] = info
	}

	// 更新失败信息
	info.UpdateFailure(reason)

	// 检查是否需要隔离
	if info.FailureCount >= m.config.MaxFailures {
		info.MarkIsolated(reason)
		metrics.IsolatedResourcesTotal.WithLabelValues(AccountIsolation.String(), reason.String()).Inc()
	}

	return nil
}

// IsolateProduct 隔离产品
func (m *FourDimensionFaultTolerance) IsolateProduct(accountID, productID string, reason IsolationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, productID)

	info, exists := m.productIsolations[key]
	if !exists {
		info = &IsolationInfo{
			Status:       IsolationActive,
			Reason:       ReasonUnknown,
			FailureCount: 0,
		}
		m.productIsolations[key] = info
	}

	// 更新失败信息
	info.UpdateFailure(reason)

	// 检查是否需要隔离
	if info.FailureCount >= m.config.MaxFailures {
		info.MarkIsolated(reason)
		metrics.IsolatedResourcesTotal.WithLabelValues(ProductIsolation.String(), reason.String()).Inc()

		// 级联传播：如果配置了级联传播，则隔离账号
		if m.config.CascadePropagation {
			accountInfo, accountExists := m.accountIsolations[accountID]
			if !accountExists {
				accountInfo = &IsolationInfo{
					Status:       IsolationActive,
					Reason:       ReasonUnknown,
					FailureCount: 0,
				}
				m.accountIsolations[accountID] = accountInfo
			}
			accountInfo.IncrementFailure()
		}
	}

	return nil
}

// IsolateRegion 隔离区域
func (m *FourDimensionFaultTolerance) IsolateRegion(accountID, regionID string, reason IsolationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, regionID)

	info, exists := m.regionIsolations[key]
	if !exists {
		info = &IsolationInfo{
			Status:       IsolationActive,
			Reason:       ReasonUnknown,
			FailureCount: 0,
		}
		m.regionIsolations[key] = info
	}

	// 更新失败信息
	info.UpdateFailure(reason)

	// 检查是否需要隔离
	if info.FailureCount >= m.config.MaxFailures {
		info.MarkIsolated(reason)
		metrics.IsolatedResourcesTotal.WithLabelValues(RegionIsolation.String(), reason.String()).Inc()

		// 级联传播：如果配置了级联传播，则隔离产品
		if m.config.CascadePropagation {
			productKey := fmt.Sprintf("%s:*", accountID)
			for k, v := range m.productIsolations {
				if len(k) > len(productKey) && k[:len(productKey)] == productKey {
					v.IncrementFailure()
				}
			}
			// 同时也影响账号
			accountInfo, accountExists := m.accountIsolations[accountID]
			if !accountExists {
				accountInfo = &IsolationInfo{
					Status:       IsolationActive,
					Reason:       ReasonUnknown,
					FailureCount: 0,
				}
				m.accountIsolations[accountID] = accountInfo
			}
			accountInfo.IncrementFailure()
		}
	}

	return nil
}

// IsolateResource 隔离资源
func (m *FourDimensionFaultTolerance) IsolateResource(accountID, resourceID string, reason IsolationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, resourceID)

	info, exists := m.resourceIsolations[key]
	if !exists {
		info = &IsolationInfo{
			Status:       IsolationActive,
			Reason:       ReasonUnknown,
			FailureCount: 0,
		}
		m.resourceIsolations[key] = info
	}

	// 更新失败信息
	info.UpdateFailure(reason)

	// 检查是否需要隔离
	if info.FailureCount >= m.config.MaxFailures {
		info.MarkIsolated(reason)
		metrics.IsolatedResourcesTotal.WithLabelValues(ResourceIsolation.String(), reason.String()).Inc()

		// 级联传播：如果配置了级联传播，则隔离区域
		if m.config.CascadePropagation {
			regionKey := fmt.Sprintf("%s:*", accountID)
			for k, v := range m.regionIsolations {
				if len(k) > len(regionKey) && k[:len(regionKey)] == regionKey {
					v.IncrementFailure()
				}
			}
			// 同时也影响账号
			accountInfo, accountExists := m.accountIsolations[accountID]
			if !accountExists {
				accountInfo = &IsolationInfo{
					Status:       IsolationActive,
					Reason:       ReasonUnknown,
					FailureCount: 0,
				}
				m.accountIsolations[accountID] = accountInfo
			}
			accountInfo.IncrementFailure()
		}
	}

	return nil
}

// IsAccountDisabled 检查账号是否已隔离
func (m *FourDimensionFaultTolerance) IsAccountDisabled(accountID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.accountIsolations[accountID]
	if !exists {
		return false
	}
	return info.IsIsolated()
}

// IsProductDisabled 检查产品是否已隔离
func (m *FourDimensionFaultTolerance) IsProductDisabled(accountID, productID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", accountID, productID)
	info, exists := m.productIsolations[key]
	if !exists {
		return false
	}
	return info.IsIsolated()
}

// IsRegionDisabled 检查区域是否已隔离
func (m *FourDimensionFaultTolerance) IsRegionDisabled(accountID, regionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", accountID, regionID)
	info, exists := m.regionIsolations[key]
	if !exists {
		return false
	}
	return info.IsIsolated()
}

// IsResourceDisabled 检查资源是否已隔离
func (m *FourDimensionFaultTolerance) IsResourceDisabled(accountID, resourceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	info, exists := m.resourceIsolations[key]
	if !exists {
		return false
	}
	return info.IsIsolated()
}

// RecoverAccount 恢复账号
func (m *FourDimensionFaultTolerance) RecoverAccount(accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.accountIsolations[accountID]
	if !exists {
		return fmt.Errorf("account %s not found", accountID)
	}

	if info.IsIsolated() {
		info.MarkRecovered()
		metrics.RecoveredResourcesTotal.WithLabelValues(AccountIsolation.String()).Inc()
	}

	return nil
}

// RecoverProduct 恢复产品
func (m *FourDimensionFaultTolerance) RecoverProduct(accountID, productID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, productID)
	info, exists := m.productIsolations[key]
	if !exists {
		return fmt.Errorf("product %s:%s not found", accountID, productID)
	}

	if info.IsIsolated() {
		info.MarkRecovered()
		metrics.RecoveredResourcesTotal.WithLabelValues(ProductIsolation.String()).Inc()
	}

	return nil
}

// RecoverRegion 恢复区域
func (m *FourDimensionFaultTolerance) RecoverRegion(accountID, regionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, regionID)
	info, exists := m.regionIsolations[key]
	if !exists {
		return fmt.Errorf("region %s:%s not found", accountID, regionID)
	}

	if info.IsIsolated() {
		info.MarkRecovered()
		metrics.RecoveredResourcesTotal.WithLabelValues(RegionIsolation.String()).Inc()
	}

	return nil
}

// RecoverResource 恢复资源
func (m *FourDimensionFaultTolerance) RecoverResource(accountID, resourceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, resourceID)
	info, exists := m.resourceIsolations[key]
	if !exists {
		return fmt.Errorf("resource %s:%s not found", accountID, resourceID)
	}

	if info.IsIsolated() {
		info.MarkRecovered()
		metrics.RecoveredResourcesTotal.WithLabelValues(ResourceIsolation.String()).Inc()
	}

	return nil
}

// StartRecoveryScheduler 启动恢复调度器
func (m *FourDimensionFaultTolerance) StartRecoveryScheduler() {
	m.wg.Add(1)
	go m.recoveryLoop()
}

// recoveryLoop 恢复循环
func (m *FourDimensionFaultTolerance) recoveryLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAndRecoverAll()
		}
	}
}

// checkAndRecoverAll 检查并恢复所有隔离资源
func (m *FourDimensionFaultTolerance) checkAndRecoverAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 恢复账号
	for accountID, info := range m.accountIsolations {
		if info.IsIsolated() {
			m.tryRecover(AccountIsolation, accountID, "")
		}
	}

	// 恢复产品
	for key, info := range m.productIsolations {
		if info.IsIsolated() {
			accountID, productID := m.parseProductKey(key)
			m.tryRecover(ProductIsolation, accountID, productID)
		}
	}

	// 恢复区域
	for key, info := range m.regionIsolations {
		if info.IsIsolated() {
			accountID, regionID := m.parseRegionKey(key)
			m.tryRecover(RegionIsolation, accountID, regionID)
		}
	}

	// 恢复资源
	for key, info := range m.resourceIsolations {
		if info.IsIsolated() {
			accountID, resourceID := m.parseResourceKey(key)
			m.tryRecover(ResourceIsolation, accountID, resourceID)
		}
	}
}

// tryRecover 尝试恢复（需要调用方实现具体的健康检查逻辑）
func (m *FourDimensionFaultTolerance) tryRecover(strategy IsolationStrategy, id1, id2 string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var info *IsolationInfo
	var key string

	switch strategy {
	case AccountIsolation:
		key = id1
		info = m.accountIsolations[key]
	case ProductIsolation:
		key = fmt.Sprintf("%s:%s", id1, id2)
		info = m.productIsolations[key]
	case RegionIsolation:
		key = fmt.Sprintf("%s:%s", id1, id2)
		info = m.regionIsolations[key]
	case ResourceIsolation:
		key = fmt.Sprintf("%s:%s", id1, id2)
		info = m.resourceIsolations[key]
	}

	if info != nil && info.IsIsolated() {
		// 标记为恢复中
		info.MarkRecovering()
	}
}

// parseProductKey 解析产品键
func (m *FourDimensionFaultTolerance) parseProductKey(key string) (string, string) {
	parts := splitKey(key, 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// parseRegionKey 解析区域键
func (m *FourDimensionFaultTolerance) parseRegionKey(key string) (string, string) {
	parts := splitKey(key, 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// parseResourceKey 解析资源键
func (m *FourDimensionFaultTolerance) parseResourceKey(key string) (string, string) {
	parts := splitKey(key, 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// splitKey 分割键
func splitKey(key string, max int) []string {
	parts := make([]string, 0, max)
	start := 0
	for i, c := range key {
		if c == ':' {
			parts = append(parts, key[start:i])
			start = i + 1
			if len(parts) == max-1 {
				break
			}
		}
	}
	parts = append(parts, key[start:])
	return parts
}

// GetIsolationStats 获取隔离统计信息
func (m *FourDimensionFaultTolerance) GetIsolationStats() map[string]map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]map[string]any)

	// 账号统计
	accountStats := make(map[string]any)
	accountStats["total"] = len(m.accountIsolations)
	accountStats["isolated"] = countIsolated(m.accountIsolations)
	stats[AccountIsolation.String()] = accountStats

	// 产品统计
	productStats := make(map[string]any)
	productStats["total"] = len(m.productIsolations)
	productStats["isolated"] = countIsolated(m.productIsolations)
	stats[ProductIsolation.String()] = productStats

	// 区域统计
	regionStats := make(map[string]any)
	regionStats["total"] = len(m.regionIsolations)
	regionStats["isolated"] = countIsolated(m.regionIsolations)
	stats[RegionIsolation.String()] = regionStats

	// 资源统计
	resourceStats := make(map[string]any)
	resourceStats["total"] = len(m.resourceIsolations)
	resourceStats["isolated"] = countIsolated(m.resourceIsolations)
	stats[ResourceIsolation.String()] = resourceStats

	return stats
}

// countIsolated 统计隔离数量
func countIsolated(isolations map[string]*IsolationInfo) int {
	count := 0
	for _, info := range isolations {
		if info.IsIsolated() {
			count++
		}
	}
	return count
}

// Stop 停止容错管理器
func (m *FourDimensionFaultTolerance) Stop() {
	m.cancel()
	m.wg.Wait()
}
