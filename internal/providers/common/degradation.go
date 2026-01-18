// Package common 提供云厂商通用的错误处理和重试逻辑
package common

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ResourceType 资源类型
type ResourceType string

const (
	// ResourceTypeAccount 账号类型
	ResourceTypeAccount ResourceType = "account"
	// ResourceTypeRegion 区域类型
	ResourceTypeRegion ResourceType = "region"
	// ResourceTypeProduct 产品类型
	ResourceTypeProduct ResourceType = "product"
)

// DegradationConfig 降级配置
type DegradationConfig struct {
	// MaxFailures 最大失败次数，达到后标记为禁用，默认 3
	MaxFailures int
	// FailureWindow 失败时间窗口，只计算窗口内的失败次数，默认 5 分钟
	FailureWindow time.Duration
	// RecoveryInterval 自动恢复检查间隔，默认 10 分钟
	RecoveryInterval time.Duration
	// RecoveryTimeout 恢复尝试超时时间，默认 30 秒
	RecoveryTimeout time.Duration
}

// DefaultDegradationConfig 返回默认降级配置
func DefaultDegradationConfig() DegradationConfig {
	return DegradationConfig{
		MaxFailures:      3,
		FailureWindow:    5 * time.Minute,
		RecoveryInterval: 10 * time.Minute,
		RecoveryTimeout:  30 * time.Second,
	}
}

// FailureRecord 失败记录
type FailureRecord struct {
	Timestamp time.Time
	Error     string
}

// ResourceState 资源状态
type ResourceState struct {
	Key            string
	Type           ResourceType
	Disabled       bool
	DisabledAt     time.Time
	FailureCount   int
	FailureRecords []FailureRecord
	mu             sync.RWMutex
}

// NewResourceState 创建新的资源状态
func NewResourceState(key string, rtype ResourceType) *ResourceState {
	return &ResourceState{
		Key:            key,
		Type:           rtype,
		Disabled:       false,
		FailureCount:   0,
		FailureRecords: make([]FailureRecord, 0),
	}
}

// RecordFailure 记录失败
func (rs *ResourceState) RecordFailure(err string, cfg DegradationConfig) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	now := time.Now()
	rs.FailureRecords = append(rs.FailureRecords, FailureRecord{
		Timestamp: now,
		Error:     err,
	})

	// 清理超出时间窗口的失败记录
	windowStart := now.Add(-cfg.FailureWindow)
	validRecords := make([]FailureRecord, 0)
	for _, record := range rs.FailureRecords {
		if record.Timestamp.After(windowStart) {
			validRecords = append(validRecords, record)
		}
	}
	rs.FailureRecords = validRecords
	rs.FailureCount = len(validRecords)

	// 检查是否达到禁用阈值
	if rs.FailureCount >= cfg.MaxFailures {
		rs.Disabled = true
		rs.DisabledAt = now
		return true
	}

	return false
}

// RecordSuccess 记录成功（用于恢复）
func (rs *ResourceState) RecordSuccess() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.FailureRecords = make([]FailureRecord, 0)
	rs.FailureCount = 0
	rs.Disabled = false
	rs.DisabledAt = time.Time{}
}

// IsDisabled 检查资源是否被禁用
func (rs *ResourceState) IsDisabled() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.Disabled
}

// GetFailureCount 获取失败次数
func (rs *ResourceState) GetFailureCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.FailureCount
}

// Manager 降级管理器
type Manager struct {
	resources map[string]*ResourceState
	cfg       DegradationConfig
	logger    *zap.Logger
	mu        sync.RWMutex
}

// NewManager 创建降级管理器
func NewManager(cfg DegradationConfig, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		resources: make(map[string]*ResourceState),
		cfg:       cfg,
		logger:    logger,
	}
}

// GetOrCreateResource 获取或创建资源状态
func (m *Manager) GetOrCreateResource(key string, rtype ResourceType) *ResourceState {
	m.mu.RLock()
	if rs, exists := m.resources[key]; exists {
		m.mu.RUnlock()
		return rs
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if rs, exists := m.resources[key]; exists {
		return rs
	}
	rs := NewResourceState(key, rtype)
	m.resources[key] = rs
	return rs
}

// RecordFailure 记录资源失败，返回是否被禁用
func (m *Manager) RecordFailure(key string, rtype ResourceType, err string) bool {
	rs := m.GetOrCreateResource(key, rtype)
	disabled := rs.RecordFailure(err, m.cfg)

	if disabled {
		m.logger.Warn("资源已被降级",
			zap.String("resource_key", key),
			zap.String("resource_type", string(rtype)),
			zap.Int("failure_count", rs.GetFailureCount()),
			zap.Int("max_failures", m.cfg.MaxFailures),
			zap.Duration("failure_window", m.cfg.FailureWindow),
		)
	}

	return disabled
}

// RecordSuccess 记录资源成功（用于恢复）
func (m *Manager) RecordSuccess(key string, rtype ResourceType) {
	rs := m.GetOrCreateResource(key, rtype)
	rs.RecordSuccess()

	m.logger.Info("资源已恢复",
		zap.String("resource_key", key),
		zap.String("resource_type", string(rtype)),
	)
}

// IsDisabled 检查资源是否被禁用
func (m *Manager) IsDisabled(key string, rtype ResourceType) bool {
	rs := m.GetOrCreateResource(key, rtype)
	return rs.IsDisabled()
}

// StartAutoRecovery 启动自动恢复协程
func (m *Manager) StartAutoRecovery(recoverFunc func(key string, rtype ResourceType) bool, shutdownCtx context.Context) {
	ticker := time.NewTicker(m.cfg.RecoveryInterval)
	defer ticker.Stop()

	m.logger.Info("自动恢复协程已启动",
		zap.Duration("interval", m.cfg.RecoveryInterval),
	)

	for {
		select {
		case <-shutdownCtx.Done():
			m.logger.Info("自动恢复协程已停止")
			return
		case <-ticker.C:
			m.tryRecoverResources(recoverFunc)
		}
	}
}

// tryRecoverResources 尝试恢复所有被禁用的资源
func (m *Manager) tryRecoverResources(recoverFunc func(key string, rtype ResourceType) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for key, rs := range m.resources {
		if rs.IsDisabled() {
			m.logger.Info("尝试恢复资源",
				zap.String("resource_key", key),
				zap.String("resource_type", string(rs.Type)),
			)

			success := recoverFunc(key, rs.Type)
			if success {
				m.RecordSuccess(key, rs.Type)
			} else {
				m.logger.Warn("资源恢复失败，保持禁用状态",
					zap.String("resource_key", key),
					zap.String("resource_type", string(rs.Type)),
				)
			}
		}
	}
}
