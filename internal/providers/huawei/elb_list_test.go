package huawei

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"multicloud-exporter/internal/config"
	providerscommon "multicloud-exporter/internal/providers/common"

	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
)

func TestListELBInstances_Success(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	// 设置 mock 客户端
	mockELB := &mockELBClient{
		listLoadBalancersFunc: func(req *elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error) {
			id1 := "elb-001"
			name1 := "test-elb-1"
			id2 := "elb-002"
			name2 := "test-elb-2"
			return &elbmodel.ListLoadBalancersResponse{
				Loadbalancers: &[]elbmodel.LoadBalancer{
					{Id: id1, Name: name1},
					{Id: id2, Name: name2},
				},
			}, nil
		},
	}

	mockFactory := &mockClientFactory{
		newELBClientFunc: func(region, ak, sk string) (ELBClient, error) {
			return mockELB, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	elbs := c.listELBInstances(account, "cn-north-4")

	assert.Len(t, elbs, 2)
	assert.Equal(t, "elb-001", elbs[0].ID)
	assert.Equal(t, "test-elb-1", elbs[0].Name)
	assert.Equal(t, "elb-002", elbs[1].ID)
	assert.Equal(t, "test-elb-2", elbs[1].Name)

	// 验证缓存已设置
	key := c.cacheKey(account, "cn-north-4", "SYS.ELB", "elb")
	entry, ok := c.resCache[key]
	assert.True(t, ok)
	assert.Len(t, entry.IDs, 2)
}

func TestListELBInstances_Empty(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	// 设置 mock 客户端返回空列表
	mockELB := &mockELBClient{
		listLoadBalancersFunc: func(req *elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error) {
			return &elbmodel.ListLoadBalancersResponse{
				Loadbalancers: &[]elbmodel.LoadBalancer{},
			}, nil
		},
	}

	mockFactory := &mockClientFactory{
		newELBClientFunc: func(region, ak, sk string) (ELBClient, error) {
			return mockELB, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	elbs := c.listELBInstances(account, "cn-north-4")

	assert.Empty(t, elbs)
}

func TestListELBInstances_Cached(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	account := config.CloudAccount{
		AccountID: "test-account",
		Provider:  "huawei",
	}

	// 预设缓存
	c.setCachedIDs(account, "cn-north-4", "SYS.ELB", "elb", []string{"cached-elb-1", "cached-elb-2"})

	// mock 客户端不应该被调用
	called := false
	mockELB := &mockELBClient{
		listLoadBalancersFunc: func(req *elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error) {
			called = true
			return nil, nil
		},
	}

	mockFactory := &mockClientFactory{
		newELBClientFunc: func(region, ak, sk string) (ELBClient, error) {
			return mockELB, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	elbs := c.listELBInstances(account, "cn-north-4")

	assert.Len(t, elbs, 2)
	assert.Equal(t, "cached-elb-1", elbs[0].ID)
	assert.Equal(t, "cached-elb-2", elbs[1].ID)
	assert.False(t, called, "不应调用 API（缓存命中）")
}

func TestListELBInstances_RegionSkip(t *testing.T) {
	c := &Collector{
		cfg:                   &config.Config{},
		resCache:              make(map[string]resCacheEntry),
		productRegionManagers: make(map[string]providerscommon.RegionManager),
	}

	// 设置 RegionManager 跳过指定区域
	mockRM := &mockRegionManager{
		shouldSkipRegionFunc: func(accountID, region string) bool {
			return true
		},
	}

	c.productRegionManagers["elb"] = mockRM

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	elbs := c.listELBInstances(account, "cn-north-4")

	assert.Empty(t, elbs)
}

func TestListELBInstances_DegradedRegion(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	mockMgr := providerscommon.NewManager(
		providerscommon.DegradationConfig{
			MaxFailures:      3,
			FailureWindow:    5 * time.Minute,
			RecoveryInterval: 10 * time.Minute,
			RecoveryTimeout:  30 * time.Second,
		},
		zap.NewNop(),
	)

	c.degradeMgr = mockMgr

	// 标记区域为降级状态
	regionKey := "huawei:test-account:cn-north-4"
	mockMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, "test error")
	mockMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, "test error")
	mockMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, "test error")

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	elbs := c.listELBInstances(account, "cn-north-4")

	assert.Empty(t, elbs)
}

// mockRegionManager RegionManager mock 实现
type mockRegionManager struct {
	shouldSkipRegionFunc func(accountID, region string) bool
}

func (m *mockRegionManager) GetActiveRegions(accountID string, allRegions []string) []string {
	return allRegions
}

func (m *mockRegionManager) UpdateRegionStatus(accountID, region string, count int, status providerscommon.RegionStatus) {
}

func (m *mockRegionManager) UpdateFromPeer(accountID, region string, count int, status string) {}

func (m *mockRegionManager) SetBroadcaster(b providerscommon.Broadcaster, provider, product string) {}

func (m *mockRegionManager) MarkRegionForRediscovery(accountID, region string) {}

func (m *mockRegionManager) GetRegionInfo(accountID, region string) (providerscommon.RegionInfo, bool) {
	return providerscommon.RegionInfo{}, false
}

func (m *mockRegionManager) ShouldSkipRegion(accountID, region string) bool {
	if m.shouldSkipRegionFunc != nil {
		return m.shouldSkipRegionFunc(accountID, region)
	}
	return false
}

func (m *mockRegionManager) StartRediscoveryScheduler() {}

func (m *mockRegionManager) Stop() {}

func (m *mockRegionManager) GetStats() providerscommon.RegionManagerStats {
	return providerscommon.RegionManagerStats{}
}

func (m *mockRegionManager) CleanupInactiveAccounts(olderThan time.Duration) int {
	return 0
}

func (m *mockRegionManager) UpdatePrometheusMetrics() {}
