// 采集器降级管理器集成测试
package collector

import (
	"testing"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	providerscommon "multicloud-exporter/internal/providers/common"
)

func TestCollector_DegradationIntegration(t *testing.T) {
	cfg := &config.Config{
		ServerConf: &config.ServerConf{},
	}

	mgr := discovery.NewManager(cfg)

	coll := NewCollector(cfg, mgr, nil)

	if coll.degradeMgr == nil {
		t.Fatal("降级管理器应为初始化")
	}

	accountKey := "test-account"

	for i := 0; i < 2; i++ {
		disabled := coll.degradeMgr.RecordFailure(accountKey, providerscommon.ResourceTypeAccount, "test error")
		if disabled {
			t.Errorf("第 %d 次失败后不应禁用", i+1)
		}
	}

	disabled := coll.degradeMgr.RecordFailure(accountKey, providerscommon.ResourceTypeAccount, "test error")
	if !disabled {
		t.Error("第三次失败后应被禁用")
	}

	if !coll.degradeMgr.IsDisabled(accountKey, providerscommon.ResourceTypeAccount) {
		t.Error("资源应被禁用")
	}

	coll.degradeMgr.RecordSuccess(accountKey, providerscommon.ResourceTypeAccount)

	if coll.degradeMgr.IsDisabled(accountKey, providerscommon.ResourceTypeAccount) {
		t.Error("记录成功后资源应恢复")
	}
}

func TestCollector_FailureWindow(t *testing.T) {
	cfg := providerscommon.DegradationConfig{
		MaxFailures:      3,
		FailureWindow:    1 * time.Second,
		RecoveryInterval: 10 * time.Minute,
		RecoveryTimeout:  30 * time.Second,
	}

	degradeMgr := providerscommon.NewManager(cfg, nil)

	accountKey := "test-account"

	degradeMgr.RecordFailure(accountKey, providerscommon.ResourceTypeAccount, "test error")
	degradeMgr.RecordFailure(accountKey, providerscommon.ResourceTypeAccount, "test error")

	time.Sleep(2 * time.Second)

	degradeMgr.RecordFailure(accountKey, providerscommon.ResourceTypeAccount, "test error")

	if degradeMgr.IsDisabled(accountKey, providerscommon.ResourceTypeAccount) {
		t.Error("超过时间窗口后第一次失败不应禁用")
	}

	if degradeMgr.GetOrCreateResource(accountKey, providerscommon.ResourceTypeAccount).GetFailureCount() != 1 {
		t.Error("超过时间窗口后失败计数应为 1")
	}
}
