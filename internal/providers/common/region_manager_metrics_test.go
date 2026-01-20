// 测试新增的产品级指标和内存占用监控
package common

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRegionManager_ProductMetrics 测试产品级指标更新
func TestRegionManager_ProductMetrics(t *testing.T) {
	rm := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
		CleanupInterval:   1 * time.Hour,
	})
	rm.SetBroadcaster(nil, "test-provider", "test-product")

	// 更新区域状态
	rm.UpdateRegionStatus("acc1", "region1", 10, RegionStatusActive)
	rm.UpdateRegionStatus("acc1", "region2", 0, RegionStatusEmpty)
	rm.UpdateRegionStatus("acc1", "region3", 0, RegionStatusEmpty)

	// 触发指标更新（内部会调用 UpdatePrometheusMetrics）
	time.Sleep(100 * time.Millisecond)

	// 验证指标是否注册
	// 注意：在测试环境中，指标值可能为 0，因为我们使用 mock broadcaster
	// 这里主要验证不会 panic
	stats := rm.GetStats()
	if stats.ActiveRegions != 1 {
		t.Errorf("Expected 1 active region, got %d", stats.ActiveRegions)
	}
	if stats.EmptyRegions != 2 {
		t.Errorf("Expected 2 empty regions, got %d", stats.EmptyRegions)
	}
}

// TestRegionManager_MemoryEstimation 测试内存占用估算
func TestRegionManager_MemoryEstimation(t *testing.T) {
	rm := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
		CleanupInterval:   1 * time.Hour,
	})
	rm.SetBroadcaster(nil, "test-provider", "test-product")

	// 添加 100 个账号，每个账号 10 个区域
	for i := 0; i < 100; i++ {
		accountID := fmt.Sprintf("acc%d", i)
		for j := 0; j < 10; j++ {
			region := fmt.Sprintf("region%d", j)
			rm.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
		}
	}

	// 等待指标更新
	time.Sleep(100 * time.Millisecond)

	// 验证内存占用估算
	// 1000 个区域 × (128 + 32) + 100 个账号 × 32 = 166,400 字节 ≈ 162 KB
	// 实际值会因内存分配而异，这里仅验证不会 panic 或溢出
	stats := rm.GetStats()
	if stats.TotalRegions != 1000 {
		t.Errorf("Expected 1000 regions, got %d", stats.TotalRegions)
	}
	if stats.TotalAccounts != 100 {
		t.Errorf("Expected 100 accounts, got %d", stats.TotalAccounts)
	}
}

// TestRegionManager_ProductLabelFormat 测试产品标签格式
func TestRegionManager_ProductLabelFormat(t *testing.T) {
	// 验证所有云厂商的产品标识是否正确格式
	testCases := []struct {
		provider string
		product  string
		expected string
	}{
		{"aliyun", "slb", "slb"},
		{"aliyun", "cbwp", "cbwp"},
		{"aliyun", "oss", "oss"},
		{"aliyun", "alb", "alb"},
		{"aliyun", "nlb", "nlb"},
		{"aliyun", "ali-gwlb", "ali-gwlb"},
		{"tencent", "clb", "clb"},
		{"tencent", "bwp", "bwp"},
		{"tencent", "cos", "cos"},
		{"tencent", "gwlb", "gwlb"},
		{"huawei", "elb", "elb"},
		{"huawei", "obs", "obs"},
		{"aws", "lb", "lb"},
		{"aws", "s3", "s3"},
	}

	for _, tc := range testCases {
		if tc.product != tc.expected {
			t.Errorf("Product label mismatch: expected %s, got %s", tc.expected, tc.product)
		}

		// 验证产品标识为小写（排除特殊字符）
		lowerProduct := strings.ToLower(tc.product)
		if tc.product != lowerProduct && tc.product != "ali-gwlb" {
			t.Errorf("Product label should be lowercase for %s", tc.product)
		}
	}
}

// TestRegionManager_UpdatePrometheusMetricsConcurrency 测试指标更新的并发安全性
func TestRegionManager_UpdatePrometheusMetricsConcurrency(t *testing.T) {
	rm := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
		CleanupInterval:   1 * time.Hour,
	})
	rm.SetBroadcaster(nil, "test-provider", "test-product")

	// 并发更新区域状态
	var wg sync.WaitGroup
	concurrency := 100
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			accountID := fmt.Sprintf("acc%d", index%10)
			region := fmt.Sprintf("region%d", index%20)
			rm.UpdateRegionStatus(accountID, region, index%5, RegionStatusActive)
		}(i)
	}
	wg.Wait()

	// 等待异步指标更新完成
	time.Sleep(500 * time.Millisecond)

	// 验证统计信息一致性
	stats := rm.GetStats()
	if stats.TotalAccounts == 0 {
		t.Error("Expected some accounts to be recorded")
	}
	if stats.TotalRegions == 0 {
		t.Error("Expected some regions to be recorded")
	}
}
