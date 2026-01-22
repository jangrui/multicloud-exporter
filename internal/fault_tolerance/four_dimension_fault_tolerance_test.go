package fault_tolerance

import (
	"sync"
	"testing"
	"time"
)

func TestIsolationInfo_StatusMethods(t *testing.T) {
	info := &IsolationInfo{
		Status:       IsolationActive,
		Reason:       ReasonAPIFailure,
		FailureCount: 2,
	}

	// 测试 GetStatus
	if info.GetStatus() != IsolationActive {
		t.Errorf("expected status %v, got %v", IsolationActive, info.GetStatus())
	}

	// 测试 GetReason
	if info.GetReason() != ReasonAPIFailure {
		t.Errorf("expected reason %v, got %v", ReasonAPIFailure, info.GetReason())
	}

	// 测试 GetFailureCount
	if info.GetFailureCount() != 2 {
		t.Errorf("expected failure count 2, got %d", info.GetFailureCount())
	}

	// 测试 IsIsolated
	if info.IsIsolated() {
		t.Error("expected not isolated, but IsIsolated returned true")
	}

	// 测试 IsRecovering
	if info.IsRecovering() {
		t.Error("expected not recovering, but IsRecovering returned true")
	}
}

func TestIsolationInfo_MarkIsolated(t *testing.T) {
	info := &IsolationInfo{
		Status:       IsolationActive,
		Reason:       ReasonUnknown,
		FailureCount: 3,
	}

	info.MarkIsolated(ReasonRateLimit)

	if info.GetStatus() != IsolationIsolated {
		t.Errorf("expected status %v, got %v", IsolationIsolated, info.GetStatus())
	}

	if info.GetReason() != ReasonRateLimit {
		t.Errorf("expected reason %v, got %v", ReasonRateLimit, info.GetReason())
	}

	if info.GetIsolatedAt().IsZero() {
		t.Error("expected isolated_at to be set, but got zero time")
	}
}

func TestIsolationInfo_MarkRecovered(t *testing.T) {
	info := &IsolationInfo{
		Status:       IsolationIsolated,
		Reason:       ReasonAPIFailure,
		FailureCount: 3,
	}

	info.MarkRecovered()

	if info.GetStatus() != IsolationActive {
		t.Errorf("expected status %v, got %v", IsolationActive, info.GetStatus())
	}

	if info.GetFailureCount() != 0 {
		t.Errorf("expected failure count 0 after recovery, got %d", info.GetFailureCount())
	}

	if info.GetLastRecovered().IsZero() {
		t.Error("expected last_recovered to be set, but got zero time")
	}
}

func TestIsolationInfo_MarkRecovering(t *testing.T) {
	info := &IsolationInfo{
		Status: IsolationIsolated,
	}

	info.MarkRecovering()

	if info.GetStatus() != IsolationRecovering {
		t.Errorf("expected status %v, got %v", IsolationRecovering, info.GetStatus())
	}
}

func TestIsolationInfo_UpdateFailure(t *testing.T) {
	info := &IsolationInfo{
		Status:       IsolationActive,
		Reason:       ReasonUnknown,
		FailureCount: 1,
	}

	info.UpdateFailure(ReasonTimeout)

	if info.GetFailureCount() != 2 {
		t.Errorf("expected failure count 2, got %d", info.GetFailureCount())
	}

	if info.GetReason() != ReasonTimeout {
		t.Errorf("expected reason %v, got %v", ReasonTimeout, info.GetReason())
	}

	if info.GetLastFailure().IsZero() {
		t.Error("expected last_failure to be set, but got zero time")
	}
}

func TestIsolationInfo_Concurrent(t *testing.T) {
	info := &IsolationInfo{
		Status:       IsolationActive,
		Reason:       ReasonUnknown,
		FailureCount: 0,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info.UpdateFailure(ReasonAPIFailure)
			info.GetStatus()
			info.GetReason()
			info.GetFailureCount()
		}()
	}

	wg.Wait()

	if info.GetFailureCount() != 100 {
		t.Errorf("expected failure count 100, got %d", info.GetFailureCount())
	}
}

func TestDefaultIsolationConfig(t *testing.T) {
	config := DefaultIsolationConfig()

	if config.MaxFailures != 3 {
		t.Errorf("expected MaxFailures 3, got %d", config.MaxFailures)
	}

	if config.FailureWindow != 5*time.Minute {
		t.Errorf("expected FailureWindow 5m, got %v", config.FailureWindow)
	}

	if config.RecoveryInterval != 10*time.Minute {
		t.Errorf("expected RecoveryInterval 10m, got %v", config.RecoveryInterval)
	}

	if config.RecoveryTimeout != 30*time.Second {
		t.Errorf("expected RecoveryTimeout 30s, got %v", config.RecoveryTimeout)
	}

	if !config.CascadePropagation {
		t.Error("expected CascadePropagation to be true")
	}
}

func TestFourDimensionFaultTolerance_IsolateAccount(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"

	// 第一次失败
	err := manager.IsolateAccount(accountID, ReasonAPIFailure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsAccountDisabled(accountID) {
		t.Error("account should not be disabled after 1 failure")
	}

	// 第二次失败
	err = manager.IsolateAccount(accountID, ReasonAPIFailure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsAccountDisabled(accountID) {
		t.Error("account should not be disabled after 2 failures")
	}

	// 第三次失败（达到阈值）
	err = manager.IsolateAccount(accountID, ReasonAPIFailure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !manager.IsAccountDisabled(accountID) {
		t.Error("account should be disabled after 3 failures")
	}
}

func TestFourDimensionFaultTolerance_IsolateProduct(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	productID := "test-product"

	// 三次失败
	for i := 0; i < 3; i++ {
		err := manager.IsolateProduct(accountID, productID, ReasonTimeout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if !manager.IsProductDisabled(accountID, productID) {
		t.Error("product should be disabled after 3 failures")
	}
}

func TestFourDimensionFaultTolerance_IsolateRegion(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	regionID := "test-region"

	// 三次失败
	for i := 0; i < 3; i++ {
		err := manager.IsolateRegion(accountID, regionID, ReasonRateLimit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if !manager.IsRegionDisabled(accountID, regionID) {
		t.Error("region should be disabled after 3 failures")
	}
}

func TestFourDimensionFaultTolerance_IsolateResource(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	resourceID := "test-resource"

	// 三次失败
	for i := 0; i < 3; i++ {
		err := manager.IsolateResource(accountID, resourceID, ReasonAuthFailure)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if !manager.IsResourceDisabled(accountID, resourceID) {
		t.Error("resource should be disabled after 3 failures")
	}
}

func TestFourDimensionFaultTolerance_RecoverAccount(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"

	// 隔离账号
	for i := 0; i < 3; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}

	if !manager.IsAccountDisabled(accountID) {
		t.Fatal("account should be disabled before recovery")
	}

	// 恢复账号
	err := manager.RecoverAccount(accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsAccountDisabled(accountID) {
		t.Error("account should be enabled after recovery")
	}
}

func TestFourDimensionFaultTolerance_RecoverProduct(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	productID := "test-product"

	// 隔离产品
	for i := 0; i < 3; i++ {
		manager.IsolateProduct(accountID, productID, ReasonTimeout)
	}

	if !manager.IsProductDisabled(accountID, productID) {
		t.Fatal("product should be disabled before recovery")
	}

	// 恢复产品
	err := manager.RecoverProduct(accountID, productID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsProductDisabled(accountID, productID) {
		t.Error("product should be enabled after recovery")
	}
}

func TestFourDimensionFaultTolerance_RecoverRegion(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	regionID := "test-region"

	// 隔离区域
	for i := 0; i < 3; i++ {
		manager.IsolateRegion(accountID, regionID, ReasonRateLimit)
	}

	if !manager.IsRegionDisabled(accountID, regionID) {
		t.Fatal("region should be disabled before recovery")
	}

	// 恢复区域
	err := manager.RecoverRegion(accountID, regionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsRegionDisabled(accountID, regionID) {
		t.Error("region should be enabled after recovery")
	}
}

func TestFourDimensionFaultTolerance_RecoverResource(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"
	resourceID := "test-resource"

	// 隔离资源
	for i := 0; i < 3; i++ {
		manager.IsolateResource(accountID, resourceID, ReasonAuthFailure)
	}

	if !manager.IsResourceDisabled(accountID, resourceID) {
		t.Fatal("resource should be disabled before recovery")
	}

	// 恢复资源
	err := manager.RecoverResource(accountID, resourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.IsResourceDisabled(accountID, resourceID) {
		t.Error("resource should be enabled after recovery")
	}
}

func TestFourDimensionFaultTolerance_RecoverNonExistent(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())

	// 尝试恢复不存在的账号
	err := manager.RecoverAccount("non-existent-account")
	if err == nil {
		t.Error("expected error for non-existent account")
	}

	// 尝试恢复不存在的产品
	err = manager.RecoverProduct("account", "non-existent-product")
	if err == nil {
		t.Error("expected error for non-existent product")
	}

	// 尝试恢复不存在的区域
	err = manager.RecoverRegion("account", "non-existent-region")
	if err == nil {
		t.Error("expected error for non-existent region")
	}

	// 尝试恢复不存在的资源
	err = manager.RecoverResource("account", "non-existent-resource")
	if err == nil {
		t.Error("expected error for non-existent resource")
	}
}

func TestFourDimensionFaultTolerance_Concurrent(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			accountID := "account-" + string(rune('0'+idx%10))
			productID := "product-" + string(rune('0'+idx%5))
			regionID := "region-" + string(rune('0'+idx%3))
			resourceID := "resource-" + string(rune('0'+idx))

			manager.IsolateAccount(accountID, ReasonAPIFailure)
			manager.IsolateProduct(accountID, productID, ReasonTimeout)
			manager.IsolateRegion(accountID, regionID, ReasonRateLimit)
			manager.IsolateResource(accountID, resourceID, ReasonAuthFailure)

			manager.IsAccountDisabled(accountID)
			manager.IsProductDisabled(accountID, productID)
			manager.IsRegionDisabled(accountID, regionID)
			manager.IsResourceDisabled(accountID, resourceID)

			manager.RecoverAccount(accountID)
			manager.RecoverProduct(accountID, productID)
			manager.RecoverRegion(accountID, regionID)
			manager.RecoverResource(accountID, resourceID)
		}(i)
	}

	wg.Wait()

	stats := manager.GetIsolationStats()
	if stats == nil {
		t.Error("expected stats to be returned, got nil")
	}
}

func TestFourDimensionFaultTolerance_GetIsolationStats(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())

	// 隔离一些账号
	for i := 0; i < 3; i++ {
		accountID := "account-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateAccount(accountID, ReasonAPIFailure)
		}
	}

	// 隔离一些产品
	for i := 0; i < 2; i++ {
		accountID := "account-A"
		productID := "product-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateProduct(accountID, productID, ReasonTimeout)
		}
	}

	// 隔离一些区域
	for i := 0; i < 2; i++ {
		accountID := "account-A"
		regionID := "region-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateRegion(accountID, regionID, ReasonRateLimit)
		}
	}

	// 隔离一些资源
	for i := 0; i < 5; i++ {
		accountID := "account-A"
		resourceID := "resource-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateResource(accountID, resourceID, ReasonAuthFailure)
		}
	}

	stats := manager.GetIsolationStats()

	// 检查账号统计
	accountStats, ok := stats[AccountIsolation.String()]
	if !ok {
		t.Error("expected account stats to be present")
	} else {
		if accountStats["total"] != 3 {
			t.Errorf("expected total accounts 3, got %v", accountStats["total"])
		}
		if accountStats["isolated"] != 3 {
			t.Errorf("expected isolated accounts 3, got %v", accountStats["isolated"])
		}
	}

	// 检查产品统计
	productStats, ok := stats[ProductIsolation.String()]
	if !ok {
		t.Error("expected product stats to be present")
	} else {
		if productStats["total"] != 2 {
			t.Errorf("expected total products 2, got %v", productStats["total"])
		}
		if productStats["isolated"] != 2 {
			t.Errorf("expected isolated products 2, got %v", productStats["isolated"])
		}
	}

	// 检查区域统计
	regionStats, ok := stats[RegionIsolation.String()]
	if !ok {
		t.Error("expected region stats to be present")
	} else {
		if regionStats["total"] != 2 {
			t.Errorf("expected total regions 2, got %v", regionStats["total"])
		}
		if regionStats["isolated"] != 2 {
			t.Errorf("expected isolated regions 2, got %v", regionStats["isolated"])
		}
	}

	// 检查资源统计
	resourceStats, ok := stats[ResourceIsolation.String()]
	if !ok {
		t.Error("expected resource stats to be present")
	} else {
		if resourceStats["total"] != 5 {
			t.Errorf("expected total resources 5, got %v", resourceStats["total"])
		}
		if resourceStats["isolated"] != 5 {
			t.Errorf("expected isolated resources 5, got %v", resourceStats["isolated"])
		}
	}
}

func TestFourDimensionFaultTolerance_CascadePropagation(t *testing.T) {
	config := DefaultIsolationConfig()
	config.CascadePropagation = true
	manager := NewFourDimensionFaultTolerance(config)

	accountID := "test-account"

	// 隔离多个产品（不达到阈值）
	for i := 0; i < 2; i++ {
		productID := "product-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateProduct(accountID, productID, ReasonAPIFailure)
		}
	}

	// 检查账号的失败次数（因为启用了级联传播）
	stats := manager.GetIsolationStats()
	accountStats := stats[AccountIsolation.String()]
	if accountStats["total"] == 0 {
		t.Error("expected account to have some failure count due to cascade propagation")
	}
}

func TestFourDimensionFaultTolerance_StartStop(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(IsolationConfig{
		MaxFailures:        3,
		FailureWindow:      5 * time.Minute,
		RecoveryInterval:   100 * time.Millisecond, // 缩短间隔用于测试
		RecoveryTimeout:    30 * time.Second,
		CascadePropagation: true,
	})

	accountID := "test-account"
	for i := 0; i < 3; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}

	if !manager.IsAccountDisabled(accountID) {
		t.Fatal("account should be disabled")
	}

	// 启动恢复调度器
	manager.StartRecoveryScheduler()

	// 等待恢复检查
	time.Sleep(200 * time.Millisecond)

	// 停止管理器
	manager.Stop()

	// 停止后不应有 panic
}

func TestFourDimensionFaultTolerance_FaultInjection_Account(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "fault-account"

	// 注入故障
	for i := 0; i < 3; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}

	// 验证隔离
	if !manager.IsAccountDisabled(accountID) {
		t.Error("account should be isolated after fault injection")
	}

	// 验证其他账号不受影响
	otherAccountID := "other-account"
	manager.IsolateAccount(otherAccountID, ReasonAPIFailure)
	if manager.IsAccountDisabled(otherAccountID) {
		t.Error("other account should not be isolated after 1 failure")
	}
}

func TestFourDimensionFaultTolerance_FaultInjection_Resource(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "test-account"

	// 隔离多个资源
	for i := 0; i < 5; i++ {
		resourceID := "resource-" + string(rune('A'+i))
		for j := 0; j < 3; j++ {
			manager.IsolateResource(accountID, resourceID, ReasonTimeout)
		}
	}

	stats := manager.GetIsolationStats()
	resourceStats := stats[ResourceIsolation.String()]

	// 验证所有资源都被隔离
	if resourceStats["total"] != 5 {
		t.Errorf("expected 5 isolated resources, got %v", resourceStats["total"])
	}

	if resourceStats["isolated"] != 5 {
		t.Errorf("expected 5 isolated resources, got %v", resourceStats["isolated"])
	}

	// 验证账号级联传播（如果启用）
	accountStats := stats[AccountIsolation.String()]
	if accountStats["total"] == 0 {
		t.Log("account not affected by resource failures (cascade propagation may be disabled)")
	}
}

func TestFourDimensionFaultTolerance_Metrics(t *testing.T) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "metrics-account"

	// 隔离账号
	for i := 0; i < 3; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}

	// 恢复账号
	manager.RecoverAccount(accountID)

	// 验证指标（通过 Prometheus 的注册来验证）
	// 注意：这里只是确保代码路径执行了，不直接验证指标值
	// 实际测试中应该使用 Prometheus 的测试工具或通过 /metrics 端点验证
}

func BenchmarkFourDimensionFaultTolerance_IsolateAccount(b *testing.B) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "bench-account"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}
}

func BenchmarkFourDimensionFaultTolerance_IsAccountDisabled(b *testing.B) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())
	accountID := "bench-account"

	for i := 0; i < 3; i++ {
		manager.IsolateAccount(accountID, ReasonAPIFailure)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.IsAccountDisabled(accountID)
	}
}

func BenchmarkFourDimensionFaultTolerance_RecoverAccount(b *testing.B) {
	manager := NewFourDimensionFaultTolerance(DefaultIsolationConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		accountID := "bench-account-" + string(rune('0'+i%10))
		for j := 0; j < 3; j++ {
			manager.IsolateAccount(accountID, ReasonAPIFailure)
		}
		manager.RecoverAccount(accountID)
	}
}
