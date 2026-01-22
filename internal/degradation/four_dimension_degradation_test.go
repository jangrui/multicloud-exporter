package degradation

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"multicloud-exporter/internal/metrics"
)

func TestDegradationInfo_RecordFailure(t *testing.T) {
	info := NewDegradationInfo()
	reason := "timeout"

	// 测试失败计数
	count := info.RecordFailure(reason)
	if count != 1 {
		t.Fatalf("expected failure count 1, got %d", count)
	}

	// 测试多次失败
	count = info.RecordFailure(reason)
	if count != 2 {
		t.Fatalf("expected failure count 2, got %d", count)
	}

	// 测试首次失败时间
	if info.FirstFailureTime.IsZero() {
		t.Fatalf("expected first failure time to be set")
	}

	// 测试最后失败时间
	if info.LastFailureTime.IsZero() {
		t.Fatalf("expected last failure time to be set")
	}
}

func TestDegradationInfo_RecordSuccess(t *testing.T) {
	info := NewDegradationInfo()
	reason := "timeout"

	// 记录失败
	info.RecordFailure(reason)
	if info.FailureCount != 1 {
		t.Fatalf("expected failure count 1 after RecordFailure")
	}

	// 记录成功
	info.RecordSuccess()
	if info.FailureCount != 0 {
		t.Fatalf("expected failure count 0 after RecordSuccess")
	}

	// 验证首次失败时间被重置
	if !info.FirstFailureTime.IsZero() {
		t.Fatalf("expected first failure time to be reset after RecordSuccess")
	}
}

func TestDegradationInfo_Disable(t *testing.T) {
	info := NewDegradationInfo()

	if info.IsDisabled() {
		t.Fatalf("expected info to be active initially")
	}

	info.Disable("test reason")
	if !info.IsDisabled() {
		t.Fatalf("expected info to be disabled after Disable")
	}

	if info.DisabledReason != "test reason" {
		t.Fatalf("expected disabled reason 'test reason', got '%s'", info.DisabledReason)
	}
}

func TestDegradationInfo_Enable(t *testing.T) {
	info := NewDegradationInfo()
	info.Disable("test reason")

	if !info.IsDisabled() {
		t.Fatalf("expected info to be disabled")
	}

	info.Enable()
	if info.IsDisabled() {
		t.Fatalf("expected info to be active after Enable")
	}
}

func TestDegradationInfo_GetDisabledDuration(t *testing.T) {
	info := NewDegradationInfo()

	info.Disable("test")
	time.Sleep(10 * time.Millisecond)

	duration := info.GetDisabledDuration()
	if duration < 10*time.Millisecond {
		t.Fatalf("expected disabled duration >= 10ms, got %v", duration)
	}
}

func TestDegradationInfo_ConcurrentAccess(t *testing.T) {
	info := NewDegradationInfo()
	var wg sync.WaitGroup

	// 100 个并发 goroutine 记录失败和成功
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				info.RecordFailure("concurrent test")
			} else {
				info.RecordSuccess()
			}
		}(i)
	}

	wg.Wait()

	if info.FailureCount < 0 || info.FailureCount > 50 {
		t.Fatalf("unexpected failure count: %d", info.FailureCount)
	}
}

// ========== FourDimensionDegradationManager 测试 ==========

func TestFourDimensionDegradation_Account(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 2

	mgr := NewFourDimensionDegradationManager(logger, config)

	accountID := "test-account"

	// 测试记录失败
	// 第一次失败，不应该禁用
	disabled := mgr.RecordAccountFailure(accountID, "timeout")
	if disabled {
		t.Fatalf("expected account not disabled after 1 failure")
	}

	// 第二次失败，应该禁用
	disabled = mgr.RecordAccountFailure(accountID, "timeout")
	if !disabled {
		t.Fatalf("expected account disabled after 2 failures")
	}

	// 验证账号已被禁用
	if !mgr.IsAccountDisabled(accountID) {
		t.Fatalf("expected account to be disabled")
	}

	// 测试记录成功
	mgr.RecordAccountSuccess(accountID)

	// 记录成功后，账号应该还是禁用的（需要手动恢复）
	if !mgr.IsAccountDisabled(accountID) {
		t.Fatalf("expected account to remain disabled after success")
	}

	// 测试恢复
	recovered := mgr.RecoverAccount(accountID)
	if !recovered {
		t.Fatalf("expected account to be recovered")
	}

	// 验证账号已恢复
	if mgr.IsAccountDisabled(accountID) {
		t.Fatalf("expected account to be active after recovery")
	}
}

func TestFourDimensionDegradation_Product(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 3

	mgr := NewFourDimensionDegradationManager(logger, config)

	accountID := "test-account"
	productID := "test-product"

	// 测试记录失败
	for i := 0; i < 2; i++ {
		disabled := mgr.RecordProductFailure(accountID, productID, "timeout")
		if disabled {
			t.Fatalf("expected product not disabled after %d failures", i+1)
		}
	}

	// 第三次失败，应该禁用
	disabled := mgr.RecordProductFailure(accountID, productID, "timeout")
	if !disabled {
		t.Fatalf("expected product disabled after 3 failures")
	}

	// 验证产品已被禁用
	if !mgr.IsProductDisabled(accountID, productID) {
		t.Fatalf("expected product to be disabled")
	}

	// 测试恢复
	recovered := mgr.RecoverProduct(accountID, productID)
	if !recovered {
		t.Fatalf("expected product to be recovered")
	}

	// 验证产品已恢复
	if mgr.IsProductDisabled(accountID, productID) {
		t.Fatalf("expected product to be active after recovery")
	}
}

func TestFourDimensionDegradation_Region(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 2

	mgr := NewFourDimensionDegradationManager(logger, config)

	accountID := "test-account"
	regionID := "cn-hangzhou"

	// 第一次失败，不应该禁用
	disabled := mgr.RecordRegionFailure(accountID, regionID, "rate limit")
	if disabled {
		t.Fatalf("expected region not disabled after 1 failure")
	}

	// 第二次失败，应该禁用
	disabled = mgr.RecordRegionFailure(accountID, regionID, "rate limit")
	if !disabled {
		t.Fatalf("expected region disabled after 2 failures")
	}

	if !mgr.IsRegionDisabled(accountID, regionID) {
		t.Fatalf("expected region to be disabled")
	}

	// 测试恢复
	recovered := mgr.RecoverRegion(accountID, regionID)
	if !recovered {
		t.Fatalf("expected region to be recovered")
	}

	if mgr.IsRegionDisabled(accountID, regionID) {
		t.Fatalf("expected region to be active after recovery")
	}
}

func TestFourDimensionDegradation_Resource(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 2

	mgr := NewFourDimensionDegradationManager(logger, config)

	accountID := "test-account"
	resourceID := "resource-123"

	// 第一次失败，不应该禁用
	disabled := mgr.RecordResourceFailure(accountID, resourceID, "auth error")
	if disabled {
		t.Fatalf("expected resource not disabled after 1 failure")
	}

	// 第二次失败，应该禁用
	disabled = mgr.RecordResourceFailure(accountID, resourceID, "auth error")
	if !disabled {
		t.Fatalf("expected resource disabled after 2 failures")
	}

	if !mgr.IsResourceDisabled(accountID, resourceID) {
		t.Fatalf("expected resource to be disabled")
	}

	// 测试恢复
	recovered := mgr.RecoverResource(accountID, resourceID)
	if !recovered {
		t.Fatalf("expected resource to be recovered")
	}

	if mgr.IsResourceDisabled(accountID, resourceID) {
		t.Fatalf("expected resource to be active after recovery")
	}
}

func TestFourDimensionDegradation_Concurrent(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 2

	mgr := NewFourDimensionDegradationManager(logger, config)

	var wg sync.WaitGroup

	// 100 个并发 goroutine 同时降级和恢复不同的账号、产品、区域、资源
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			accountID := string(rune('a' + (n % 26)))
			productID := string(rune('A' + (n % 26)))
			regionID := string(rune('0' + (n % 10)))
			resourceID := string(rune('0' + (n % 10)))

			// 记录 2 次失败，触发降级
			for j := 0; j < 2; j++ {
				switch n % 4 {
				case 0:
					mgr.RecordAccountFailure(accountID, "timeout")
				case 1:
					mgr.RecordProductFailure(accountID, productID, "timeout")
				case 2:
					mgr.RecordRegionFailure(accountID, regionID, "timeout")
				case 3:
					mgr.RecordResourceFailure(accountID, resourceID, "timeout")
				}
			}

			// 不恢复资源，保持降级状态用于统计
		}(i)
	}

	wg.Wait()

	stats := mgr.GetDegradationStats()
	if len(stats) == 0 {
		t.Fatalf("expected some degradation stats, got none")
	}

	// 验证每个维度都有降级
	if stats[DimensionAccount] == 0 && stats[DimensionProduct] == 0 &&
		stats[DimensionRegion] == 0 && stats[DimensionResource] == 0 {
		t.Fatalf("expected at least one dimension with degraded resources")
	}
}

func TestFourDimensionDegradation_GetDegradationStats(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 1

	mgr := NewFourDimensionDegradationManager(logger, config)

	// 禁用一些资源
	mgr.RecordAccountFailure("account1", "timeout")
	mgr.RecordProductFailure("account1", "product1", "timeout")
	mgr.RecordRegionFailure("account1", "region1", "timeout")
	mgr.RecordResourceFailure("account1", "resource1", "timeout")

	stats := mgr.GetDegradationStats()

	// 验证统计数据
	if stats[DimensionAccount] != 1 {
		t.Fatalf("expected 1 degraded account, got %d", stats[DimensionAccount])
	}

	if stats[DimensionProduct] != 1 {
		t.Fatalf("expected 1 degraded product, got %d", stats[DimensionProduct])
	}

	if stats[DimensionRegion] != 1 {
		t.Fatalf("expected 1 degraded region, got %d", stats[DimensionRegion])
	}

	if stats[DimensionResource] != 1 {
		t.Fatalf("expected 1 degraded resource, got %d", stats[DimensionResource])
	}
}

func TestFourDimensionDegradation_AutoRecovery(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 1
	config.RecoveryInterval = 100 * time.Millisecond

	mgr := NewFourDimensionDegradationManager(logger, config)

	// 禁用一个账号
	mgr.RecordAccountFailure("test-account", "timeout")

	if !mgr.IsAccountDisabled("test-account") {
		t.Fatalf("expected account to be disabled")
	}

	// 启动自动恢复调度器
	go mgr.StartRecoveryScheduler()

	// 等待恢复
	time.Sleep(200 * time.Millisecond)

	// 停止调度器
	mgr.Stop()

	// 验证账号已恢复
	if mgr.IsAccountDisabled("test-account") {
		t.Fatalf("expected account to be auto-recovered")
	}
}

// Benchmark 测试

func BenchmarkDegradationInfo_RecordFailure(b *testing.B) {
	info := NewDegradationInfo()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info.RecordFailure("timeout")
	}
}

func BenchmarkDegradationManager_RecordAccountFailure(b *testing.B) {
	logger := zap.NewNop()
	config := DefaultDegradationConfig()
	config.MaxFailures = 3

	mgr := NewFourDimensionDegradationManager(logger, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		accountID := string(rune('a' + (i % 26)))
		mgr.RecordAccountFailure(accountID, "timeout")
	}
}

func init() {
	// 初始化指标
	metrics.RegisterNamespacePrefix("test", "test_")
}
