// Package common 测试降级管理器
package common

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestResourceState_RecordFailure(t *testing.T) {
	cfg := DefaultDegradationConfig()
	cfg.MaxFailures = 3
	cfg.FailureWindow = 5 * time.Minute

	rs := NewResourceState("test-account", ResourceTypeAccount)

	// 记录前两次失败，不应该被禁用
	for i := 0; i < 2; i++ {
		disabled := rs.RecordFailure("test error", cfg)
		if disabled {
			t.Errorf("第 %d 次失败后不应禁用", i+1)
		}
		if rs.GetFailureCount() != i+1 {
			t.Errorf("第 %d 次失败后失败计数应为 %d，实际为 %d", i+1, i+1, rs.GetFailureCount())
		}
	}

	// 记录第三次失败，应该被禁用
	disabled := rs.RecordFailure("test error", cfg)
	if !disabled {
		t.Error("第三次失败后应被禁用")
	}
	if !rs.IsDisabled() {
		t.Error("资源应被禁用")
	}
	if rs.GetFailureCount() != 3 {
		t.Errorf("第三次失败后失败计数应为 3，实际为 %d", rs.GetFailureCount())
	}
}

func TestResourceState_RecordSuccess(t *testing.T) {
	cfg := DefaultDegradationConfig()
	cfg.MaxFailures = 3

	rs := NewResourceState("test-account", ResourceTypeAccount)

	// 记录三次失败
	for i := 0; i < 3; i++ {
		rs.RecordFailure("test error", cfg)
	}

	// 验证资源被禁用
	if !rs.IsDisabled() {
		t.Error("三次失败后资源应被禁用")
	}

	// 记录成功，应该恢复
	rs.RecordSuccess()

	if rs.IsDisabled() {
		t.Error("记录成功后资源应恢复")
	}
	if rs.GetFailureCount() != 0 {
		t.Errorf("记录成功后失败计数应为 0，实际为 %d", rs.GetFailureCount())
	}
}

func TestResourceState_FailureWindow(t *testing.T) {
	cfg := DefaultDegradationConfig()
	cfg.MaxFailures = 3
	cfg.FailureWindow = 1 * time.Second

	rs := NewResourceState("test-account", ResourceTypeAccount)

	// 记录两次失败
	for i := 0; i < 2; i++ {
		rs.RecordFailure("test error", cfg)
	}

	// 等待超过时间窗口
	time.Sleep(2 * time.Second)

	// 记录第三次失败，应该重新计数
	disabled := rs.RecordFailure("test error", cfg)
	if disabled {
		t.Error("超过时间窗口后的第一次失败不应禁用")
	}
	if rs.GetFailureCount() != 1 {
		t.Errorf("超过时间窗口后失败计数应为 1，实际为 %d", rs.GetFailureCount())
	}
}

func TestManager_RecordFailure(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(DefaultDegradationConfig(), logger)

	// 记录两次失败
	for i := 0; i < 2; i++ {
		disabled := mgr.RecordFailure("test-account", ResourceTypeAccount, "test error")
		if disabled {
			t.Errorf("第 %d 次失败后不应禁用", i+1)
		}
		if mgr.IsDisabled("test-account", ResourceTypeAccount) {
			t.Errorf("第 %d 次失败后资源不应禁用", i+1)
		}
	}

	// 记录第三次失败，应该被禁用
	disabled := mgr.RecordFailure("test-account", ResourceTypeAccount, "test error")
	if !disabled {
		t.Error("第三次失败后应被禁用")
	}
	if !mgr.IsDisabled("test-account", ResourceTypeAccount) {
		t.Error("第三次失败后资源应被禁用")
	}
}

func TestManager_RecordSuccess(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(DefaultDegradationConfig(), logger)

	cfg := DefaultDegradationConfig()
	cfg.MaxFailures = 3

	// 记录三次失败
	for i := 0; i < 3; i++ {
		mgr.RecordFailure("test-account", ResourceTypeAccount, "test error")
	}

	// 验证资源被禁用
	if !mgr.IsDisabled("test-account", ResourceTypeAccount) {
		t.Error("三次失败后资源应被禁用")
	}

	// 记录成功，应该恢复
	mgr.RecordSuccess("test-account", ResourceTypeAccount)

	if mgr.IsDisabled("test-account", ResourceTypeAccount) {
		t.Error("记录成功后资源应恢复")
	}
}

func TestManager_StartAutoRecovery(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultDegradationConfig()
	cfg.MaxFailures = 1
	cfg.RecoveryInterval = 100 * time.Millisecond
	mgr := NewManager(cfg, logger)

	// 记录失败，禁用资源
	mgr.RecordFailure("test-account", ResourceTypeAccount, "test error")
	if !mgr.IsDisabled("test-account", ResourceTypeAccount) {
		t.Error("资源应被禁用")
	}

	// 启动自动恢复
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	recoverCalled := make(chan bool, 1)
	recoverFunc := func(key string, rtype ResourceType) bool {
		recoverCalled <- true
		return true
	}

	go mgr.StartAutoRecovery(recoverFunc, shutdownCtx)

	// 等待自动恢复执行
	select {
	case <-recoverCalled:
		// 恢复函数被调用
	case <-time.After(500 * time.Millisecond):
		t.Error("自动恢复函数应被调用")
	}

	// 验证资源已恢复
	if mgr.IsDisabled("test-account", ResourceTypeAccount) {
		t.Error("自动恢复后资源应已恢复")
	}
}
