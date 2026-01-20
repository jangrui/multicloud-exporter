package common

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestProductRegionIsolation_Scenario1 产品独立状态隔离测试
// 验证：SLB=Empty, CBWP=Active, OSS=Empty 时，只有 CBWP 调用 API
func TestProductRegionIsolation_Scenario1(t *testing.T) {
	// 1. 创建三个产品的 RegionManager
	slbRM := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3, // 连续 3 次空后跳过
		DiscoveryInterval: 1 * time.Hour,
	})

	cbwpmRM := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
	})

	ossRM := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
	})

	accountID := "test-account"
	region := "ap-northeast-1"

	// 2. 模拟 3 次采集
	t.Run("3 次采集，SLB 和 OSS 为空，CBWP 有资源", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			// SLB: 返回 0 个资源
			if slbRM.ShouldSkipRegion(accountID, region) {
				t.Logf("第 %d 次采集: SLB 被跳过", i)
			} else {
				t.Logf("第 %d 次采集: SLB 未跳过，执行采集", i)
				slbRM.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
			}

			// CBWP: 返回 5 个资源
			if cbwpmRM.ShouldSkipRegion(accountID, region) {
				t.Fatalf("第 %d 次采集: CBWP 不应被跳过（有资源）", i)
			} else {
				t.Logf("第 %d 次采集: CBWP 未跳过，执行采集", i)
				cbwpmRM.UpdateRegionStatus(accountID, region, 5, RegionStatusActive)
			}

			// OSS: 返回 0 个资源
			if ossRM.ShouldSkipRegion(accountID, region) {
				t.Logf("第 %d 次采集: OSS 被跳过", i)
			} else {
				t.Logf("第 %d 次采集: OSS 未跳过，执行采集", i)
				ossRM.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
			}
		}

		// 3. 验证：第 4 次采集时，SLB 和 OSS 应该被跳过，CBWP 继续采集
		t.Run("第 4 次采集验证", func(t *testing.T) {
			slbSkipped := slbRM.ShouldSkipRegion(accountID, region)
			cbwpmSkipped := cbwpmRM.ShouldSkipRegion(accountID, region)
			ossSkipped := ossRM.ShouldSkipRegion(accountID, region)

			assert.True(t, slbSkipped, "SLB 应该被跳过（EmptyCount=3）")
			assert.False(t, cbwpmSkipped, "CBWP 不应被跳过（Status=Active）")
			assert.True(t, ossSkipped, "OSS 应该被跳过（EmptyCount=3）")
		})

		// 4. 验证：SLB 和 OSS 的状态，CBWP 的状态
		t.Run("验证状态", func(t *testing.T) {
			slbInfo, _ := slbRM.GetRegionInfo(accountID, region)
			cbwpmInfo, _ := cbwpmRM.GetRegionInfo(accountID, region)
			ossInfo, _ := ossRM.GetRegionInfo(accountID, region)

			assert.Equal(t, RegionStatusEmpty, slbInfo.Status, "SLB 状态应为 Empty")
			assert.Equal(t, 3, slbInfo.EmptyCount, "SLB EmptyCount 应为 3")

			assert.Equal(t, RegionStatusActive, cbwpmInfo.Status, "CBWP 状态应为 Active")
			assert.Equal(t, 0, cbwpmInfo.EmptyCount, "CBWP EmptyCount 应为 0")

			assert.Equal(t, RegionStatusEmpty, ossInfo.Status, "OSS 状态应为 Empty")
			assert.Equal(t, 3, ossInfo.EmptyCount, "OSS EmptyCount 应为 3")
		})
	})
}

// TestProductRegionIsolation_Scenario2 集群同步隔离测试
// 验证：Pod 0 的 SLB Empty 状态不影响 Pod 1 的 CBWP Active 状态
func TestProductRegionIsolation_Scenario2(t *testing.T) {
	// 1. 创建 Mock 广播器
	mockBroadcaster := &MockBroadcaster{}

	// 2. Pod 0: 创建 SLB RegionManager
	slbRM_Pod0 := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
	})
	slbRM_Pod0.SetBroadcaster(mockBroadcaster, "aliyun", "slb")

	// 3. Pod 1: 创建 CBWP RegionManager
	cbwpRM_Pod1 := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    3,
		DiscoveryInterval: 1 * time.Hour,
	})
	cbwpRM_Pod1.SetBroadcaster(mockBroadcaster, "aliyun", "cbwp")

	accountID := "test-account"
	region := "ap-northeast-1"

	// 4. Pod 0: 模拟 SLB 采集（3 次，均为空）
	for i := 1; i <= 3; i++ {
		slbRM_Pod0.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
	}

	// 5. 验证：Pod 0 的 SLB RegionManager 状态
	slbInfo, _ := slbRM_Pod0.GetRegionInfo(accountID, region)
	assert.Equal(t, RegionStatusEmpty, slbInfo.Status)
	assert.Equal(t, 3, slbInfo.EmptyCount)

	// 6. 验证：广播消息中包含 SLB 的 Empty 状态
	messages := mockBroadcaster.GetMessages()
	assert.GreaterOrEqual(t, len(messages), 3, "应该至少有 3 条广播消息")

	slbMessages := []BroadcastMessage{}
	for _, msg := range messages {
		if msg.Provider == "aliyun" && msg.Product == "slb" {
			slbMessages = append(slbMessages, msg)
		}
	}
	assert.GreaterOrEqual(t, len(slbMessages), 3, "应该至少有 3 条 SLB 广播消息")

	// 7. 验证：Pod 1 的 CBWP RegionManager 状态不受影响
	cbwpRM_Pod1.UpdateRegionStatus(accountID, region, 5, RegionStatusActive)
	cbwpInfo, _ := cbwpRM_Pod1.GetRegionInfo(accountID, region)

	assert.Equal(t, RegionStatusActive, cbwpInfo.Status)
	assert.Equal(t, 0, cbwpInfo.EmptyCount)

	// 8. 验证：广播消息中包含 CBWP 的 Active 状态
	// 重新获取所有消息（包括 CBWP 的）
	allMessages := mockBroadcaster.GetMessages()
	cbwpMessages := []BroadcastMessage{}
	for _, msg := range allMessages {
		if msg.Provider == "aliyun" && msg.Product == "cbwp" {
			cbwpMessages = append(cbwpMessages, msg)
		}
	}
	assert.GreaterOrEqual(t, len(cbwpMessages), 1, "应该至少有 1 条 CBWP 广播消息")

	if len(cbwpMessages) > 0 {
		lastCBWPMessage := cbwpMessages[len(cbwpMessages)-1]
		assert.Equal(t, "active", lastCBWPMessage.Status)
		assert.Equal(t, 5, lastCBWPMessage.ResourceCount)
	}

	// 9. 验证：集群同步键格式正确（{provider}:{product}）
	for _, msg := range messages {
		assert.NotEmpty(t, msg.Provider, "广播消息应包含 provider 字段")
		assert.NotEmpty(t, msg.Product, "广播消息应包含 product 字段")
	}
}

// TestProductRegionIsolation_Scenario3 多区域多产品测试
// 验证：不同区域下不同产品的状态互不干扰
func TestProductRegionIsolation_Scenario3(t *testing.T) {
	// 1. 创建两个产品的 RegionManager
	slbRM := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    10, // 设置较高的阈值
		DiscoveryInterval: 1 * time.Hour,
	})

	cbwpRM := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    10,
		DiscoveryInterval: 1 * time.Hour,
	})

	accountID := "test-account"

	// 2. 配置 3 个区域的资源分布
	regions := map[string]struct {
		slbCount  int
		cbwpCount int
	}{
		"ap-northeast-1": {slbCount: 0, cbwpCount: 5}, // SLB=0, CBWP=5
		"ap-southeast-1": {slbCount: 3, cbwpCount: 0}, // SLB=3, CBWP=0
		"ap-south-1":     {slbCount: 0, cbwpCount: 0}, // SLB=0, CBWP=0
	}

	// 3. 运行 3 次采集
	for iteration := 1; iteration <= 3; iteration++ {
		t.Run("迭代 "+string(rune(iteration)), func(t *testing.T) {
			for region, counts := range regions {
				// SLB 采集
				if slbRM.ShouldSkipRegion(accountID, region) {
					t.Logf("SLB 区域 %s 已跳过", region)
				} else {
					slbRM.UpdateRegionStatus(accountID, region, counts.slbCount, func() RegionStatus {
						if counts.slbCount > 0 {
							return RegionStatusActive
						}
						return RegionStatusEmpty
					}())
				}

				// CBWP 采集
				if cbwpRM.ShouldSkipRegion(accountID, region) {
					t.Logf("CBWP 区域 %s 已跳过", region)
				} else {
					cbwpRM.UpdateRegionStatus(accountID, region, counts.cbwpCount, func() RegionStatus {
						if counts.cbwpCount > 0 {
							return RegionStatusActive
						}
						return RegionStatusEmpty
					}())
				}
			}
		})
	}

	// 4. 验证：ap-northeast-1 只采集 CBWP
	t.Run("ap-northeast-1 验证", func(t *testing.T) {
		slbInfo, _ := slbRM.GetRegionInfo(accountID, "ap-northeast-1")
		cbwpInfo, _ := cbwpRM.GetRegionInfo(accountID, "ap-northeast-1")

		assert.Equal(t, RegionStatusEmpty, slbInfo.Status)
		assert.Equal(t, 3, slbInfo.EmptyCount)

		assert.Equal(t, RegionStatusActive, cbwpInfo.Status)
		assert.Equal(t, 0, cbwpInfo.EmptyCount)
	})

	// 5. 验证：ap-southeast-1 只采集 SLB
	t.Run("ap-southeast-1 验证", func(t *testing.T) {
		slbInfo, _ := slbRM.GetRegionInfo(accountID, "ap-southeast-1")
		cbwpInfo, _ := cbwpRM.GetRegionInfo(accountID, "ap-southeast-1")

		assert.Equal(t, RegionStatusActive, slbInfo.Status)
		assert.Equal(t, 0, slbInfo.EmptyCount)

		assert.Equal(t, RegionStatusEmpty, cbwpInfo.Status)
		assert.Equal(t, 3, cbwpInfo.EmptyCount)
	})

	// 6. 验证：ap-south-1 所有产品都跳过（EmptyCount>=10，但只有 3 次迭代）
	t.Run("ap-south-1 验证", func(t *testing.T) {
		slbInfo, _ := slbRM.GetRegionInfo(accountID, "ap-south-1")
		cbwpInfo, _ := cbwpRM.GetRegionInfo(accountID, "ap-south-1")

		// 由于 EmptyThreshold=10，只有 3 次迭代，不会跳过
		assert.Equal(t, RegionStatusEmpty, slbInfo.Status)
		assert.Equal(t, 3, slbInfo.EmptyCount)
		assert.False(t, slbRM.ShouldSkipRegion(accountID, "ap-south-1"))

		assert.Equal(t, RegionStatusEmpty, cbwpInfo.Status)
		assert.Equal(t, 3, cbwpInfo.EmptyCount)
		assert.False(t, cbwpRM.ShouldSkipRegion(accountID, "ap-south-1"))
	})
}

// TestProductRegionIsolation_ConcurrentAccess 并发访问测试
// 验证：多个 goroutine 同时访问不同产品的 RegionManager，不会发生数据竞争
func TestProductRegionIsolation_ConcurrentAccess(t *testing.T) {
	// 1. 创建多个产品的 RegionManager
	managers := map[string]RegionManager{
		"slb":  NewRegionManager(RegionDiscoveryConfig{Enabled: true, EmptyThreshold: 3}),
		"cbwp": NewRegionManager(RegionDiscoveryConfig{Enabled: true, EmptyThreshold: 3}),
		"oss":  NewRegionManager(RegionDiscoveryConfig{Enabled: true, EmptyThreshold: 3}),
	}

	accountID := "test-account"
	region := "ap-northeast-1"

	// 2. 启动多个 goroutine 并发访问
	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// 随机选择一个产品
			productKeys := []string{"slb", "cbwp", "oss"}
			product := productKeys[idx%3]

			rm := managers[product]

			// 并发执行 ShouldSkipRegion 和 UpdateRegionStatus
			rm.ShouldSkipRegion(accountID, region)
			rm.UpdateRegionStatus(accountID, region, idx%5, func() RegionStatus {
				if idx%5 > 0 {
					return RegionStatusActive
				}
				return RegionStatusEmpty
			}())

			// 获取区域信息
			rm.GetRegionInfo(accountID, region)
		}(i)
	}

	// 3. 等待所有 goroutine 完成
	wg.Wait()

	// 4. 验证：所有 RegionManager 的状态都正确
	for product, rm := range managers {
		info, _ := rm.GetRegionInfo(accountID, region)
		assert.NotNil(t, info, "产品 %s 的区域信息不应为空", product)
		assert.NotNil(t, info.LastSeen, "产品 %s 的区域 LastSeen 不应为零值", product)
	}
}

// TestProductRegionIsolation_KeyFormat 集群同步键格式测试
// 验证：集群同步键格式为 {provider}:{product}
func TestProductRegionIsolation_KeyFormat(t *testing.T) {
	// 1. 创建 Mock 广播器
	mockBroadcaster := &MockBroadcaster{}

	// 2. 创建多个产品的 RegionManager
	providers := []string{"aliyun", "tencent", "huawei"}
	products := []string{"slb", "cbwp", "oss"}

	for _, provider := range providers {
		for _, product := range products {
			rm := NewRegionManager(RegionDiscoveryConfig{Enabled: true})
			rm.SetBroadcaster(mockBroadcaster, provider, product)

			rm.UpdateRegionStatus("test-account", "test-region", 0, RegionStatusEmpty)
		}
	}

	// 3. 验证：所有广播消息的键格式正确
	messages := mockBroadcaster.GetMessages()
	assert.Equal(t, len(providers)*len(products), len(messages), "广播消息数量应匹配产品数量")

	for _, msg := range messages {
		assert.NotEmpty(t, msg.Provider, "广播消息应包含 provider 字段")
		assert.NotEmpty(t, msg.Product, "广播消息应包含 product 字段")
		assert.Contains(t, providers, msg.Provider, "Provider 应为支持的云厂商之一")
		assert.Contains(t, products, msg.Product, "Product 应为支持的产品之一")
	}
}

// TestProductRegionIsolation_EmptyThresholdAccumulation EmptyThreshold 累积测试
// 验证：EmptyCount 正确累积，达到阈值后跳过
func TestProductRegionIsolation_EmptyThresholdAccumulation(t *testing.T) {
	rm := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		EmptyThreshold:    5, // 阈值为 5
		DiscoveryInterval: 1 * time.Hour,
	})

	accountID := "test-account"
	region := "ap-northeast-1"

	// 1. 累积 4 次空结果
	for i := 1; i <= 4; i++ {
		assert.False(t, rm.ShouldSkipRegion(accountID, region), "第 %d 次，不应跳过", i)
		rm.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)

		info, _ := rm.GetRegionInfo(accountID, region)
		assert.Equal(t, i, info.EmptyCount, "第 %d 次，EmptyCount 应为 %d", i, i)
	}

	// 2. 第 5 次空结果，达到阈值
	assert.False(t, rm.ShouldSkipRegion(accountID, region), "第 5 次，不应跳过（累积 5 次）")
	rm.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)

	// 3. 第 6 次，应该跳过
	assert.True(t, rm.ShouldSkipRegion(accountID, region), "第 6 次，应该跳过（累积 5 次）")

	info, _ := rm.GetRegionInfo(accountID, region)
	assert.Equal(t, 5, info.EmptyCount, "EmptyCount 应为 5（达到阈值）")
	assert.Equal(t, RegionStatusEmpty, info.Status)

	// 4. 模拟重新发现，重置状态
	rm.MarkRegionForRediscovery(accountID, region)
	info2, _ := rm.GetRegionInfo(accountID, region)
	assert.Equal(t, 0, info2.EmptyCount, "重置后，EmptyCount 应为 0")
	assert.False(t, rm.ShouldSkipRegion(accountID, region), "重置后，不应跳过")
}
