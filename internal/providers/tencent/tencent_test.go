package tencent

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	providerscommon "multicloud-exporter/internal/providers/common"

	"github.com/stretchr/testify/assert"
	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type mockDiscoverer struct {
	products []config.Product
}

func (m *mockDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	return m.products
}

func TestGetAllRegions(t *testing.T) {
	cfg := &config.Config{}
	mgr := discovery.NewManager(cfg)
	collector := NewCollector(cfg, mgr)

	// Mock factory
	factory := &mockClientFactory{}
	collector.clientFactory = factory

	account := config.CloudAccount{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
	}

	// Case 1: Success
	factory.cvm = &mockCVMClient{
		DescribeRegionsFunc: func(request *cvm.DescribeRegionsRequest) (*cvm.DescribeRegionsResponse, error) {
			r1 := "ap-beijing"
			r2 := "ap-shanghai"
			resp := cvm.NewDescribeRegionsResponse()
			resp.Response = &cvm.DescribeRegionsResponseParams{
				RegionSet: []*cvm.RegionInfo{
					{Region: &r1},
					{Region: &r2},
				},
			}
			return resp, nil
		},
	}

	regions := collector.getAllRegions(account)
	assert.Equal(t, []string{"ap-beijing", "ap-shanghai"}, regions)

	// Case 2: Error (Fallback to default)
	factory.cvm = &mockCVMClient{
		DescribeRegionsFunc: func(request *cvm.DescribeRegionsRequest) (*cvm.DescribeRegionsResponse, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	regions = collector.getAllRegions(account)
	assert.Equal(t, []string{"ap-guangzhou"}, regions)

	// Case 3: Error with ENV
	if err := os.Setenv("DEFAULT_REGIONS", "ap-nanjing, ap-chengdu"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("DEFAULT_REGIONS") }()
	regions = collector.getAllRegions(account)
	assert.Equal(t, []string{"ap-nanjing", "ap-chengdu"}, regions)

	// Case 4: Empty response
	factory.cvm = &mockCVMClient{
		DescribeRegionsFunc: func(request *cvm.DescribeRegionsRequest) (*cvm.DescribeRegionsResponse, error) {
			resp := cvm.NewDescribeRegionsResponse()
			resp.Response = &cvm.DescribeRegionsResponseParams{
				RegionSet: []*cvm.RegionInfo{},
			}
			return resp, nil
		},
	}
	regions = collector.getAllRegions(account)
	assert.Equal(t, []string{"ap-guangzhou"}, regions)

	// Case 5: Client creation failure
	// To simulate this, we can make the factory return an error for NewCVMClient
	// But our mock factory checks for nil. So let's set cvm to nil.
	factory.cvm = nil
	regions = collector.getAllRegions(account)
	assert.Equal(t, []string{"ap-guangzhou"}, regions)
}

func TestCollectRegion(t *testing.T) {
	cfg := &config.Config{}
	mgr := discovery.NewManager(cfg)
	collector := NewCollector(cfg, mgr)

	// Mock factory
	factory := &mockClientFactory{
		cvm:     &mockCVMClient{},
		clb:     &mockCLBClient{},
		vpc:     &mockVPCClient{},
		cos:     &mockCOSClient{},
		monitor: &mockMonitorClient{},
	}
	collector.clientFactory = factory

	// Register mock discoverer
	md := &mockDiscoverer{
		products: []config.Product{
			{Namespace: "QCE/LB"},
			{Namespace: "QCE/BWP"},
		},
	}
	discovery.Register("tencent", md)
	// Refresh manager to load products
	_ = mgr.Refresh(context.Background())

	account := config.CloudAccount{
		AccountID:       "123",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
	}

	// Case 1: Specific resource "clb"
	account.Resources = []string{"clb"}
	// We are not asserting internal calls here, just ensuring it runs without panic
	// and covers the switch case.
	// Ideally we should verify if collectCLB was called, but since those are private methods
	// and we don't have spy on them easily without refactoring, we rely on coverage.
	// We can verify if mock client methods are called if we really want.
	calledCLB := false
	factory.clb.DescribeLoadBalancersFunc = func(request *clb.DescribeLoadBalancersRequest) (response *clb.DescribeLoadBalancersResponse, err error) {
		calledCLB = true
		return nil, nil
	}
	// Note: The mock signature in mock_test.go uses specific types, not interface{}.
	// We can't easily replace the func with a generic one.
	// But we can check if it triggers the logic.

	collector.collectRegion(account, "ap-beijing")
	assert.True(t, calledCLB)

	// Case 2: Wildcard resource
	account.Resources = []string{"*"}
	collector.collectRegion(account, "ap-beijing")

	// Case 3: Unknown resource
	account.Resources = []string{"unknown_service"}
	collector.collectRegion(account, "ap-beijing")
}

func TestCollect(t *testing.T) {
	cfg := &config.Config{}
	mgr := discovery.NewManager(cfg)
	collector := NewCollector(cfg, mgr)
	factory := &mockClientFactory{
		cvm: &mockCVMClient{
			DescribeRegionsFunc: func(request *cvm.DescribeRegionsRequest) (*cvm.DescribeRegionsResponse, error) {
				r1 := "ap-beijing"
				resp := cvm.NewDescribeRegionsResponse()
				resp.Response = &cvm.DescribeRegionsResponseParams{
					RegionSet: []*cvm.RegionInfo{{Region: &r1}},
				}
				return resp, nil
			},
		},
	}
	collector.clientFactory = factory

	account := config.CloudAccount{
		Regions: []string{}, // Trigger auto-discovery
	}

	// Just ensure no panic
	collector.Collect(account)

	account.Regions = []string{"*"}
	collector.Collect(account)
}

func TestCollectRegion_MoreResources(t *testing.T) {
	cfg := &config.Config{}
	mgr := discovery.NewManager(cfg)
	collector := NewCollector(cfg, mgr)
	factory := &mockClientFactory{
		cvm:     &mockCVMClient{},
		clb:     &mockCLBClient{},
		vpc:     &mockVPCClient{},
		cos:     &mockCOSClient{},
		monitor: &mockMonitorClient{},
	}
	collector.clientFactory = factory

	// Register mock discoverer
	md := &mockDiscoverer{
		products: []config.Product{
			{Namespace: "QCE/LB"},
			{Namespace: "QCE/BWP"},
			{Namespace: "QCE/COS"},
		},
	}
	discovery.Register("tencent", md)
	_ = mgr.Refresh(context.Background())

	account := config.CloudAccount{
		AccountID:       "123",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
	}

	resources := []string{"clb", "bwp", "s3"}
	for _, res := range resources {
		account.Resources = []string{res}
		collector.collectRegion(account, "ap-beijing")
	}
}

func TestGetCachedIDs(t *testing.T) {
	cfg := &config.Config{}
	mgr := discovery.NewManager(cfg)
	collector := NewCollector(cfg, mgr)

	account := config.CloudAccount{AccountID: "acc1"}
	region := "r1"
	ns := "ns1"
	rtype := "t1"
	ids := []string{"id1", "id2"}

	// Test Set and Get
	collector.setCachedIDs(account, region, ns, rtype, ids)
	cached, ok := collector.getCachedIDs(account, region, ns, rtype)
	assert.True(t, ok)
	assert.Equal(t, ids, cached)

	// Test Cache Miss
	cached, ok = collector.getCachedIDs(account, region, ns, "other")
	assert.False(t, ok)
	assert.Nil(t, cached)

	// Test TTL Config (ServerConf)
	collector.cfg.ServerConf = &config.ServerConf{
		DiscoveryTTL: "1ms",
	}
	collector.setCachedIDs(account, region, ns, rtype, ids)
	time.Sleep(2 * time.Millisecond)
	cached, ok = collector.getCachedIDs(account, region, ns, rtype)
	assert.False(t, ok)
	assert.Nil(t, cached)

	// Test TTL Config (Server)
	collector.cfg.ServerConf = nil
	collector.cfg.Server = &config.ServerConf{
		DiscoveryTTL: "1ms",
	}
	collector.setCachedIDs(account, region, ns, rtype, ids)
	time.Sleep(2 * time.Millisecond)
	cached, ok = collector.getCachedIDs(account, region, ns, rtype)
	assert.False(t, ok)
	assert.Nil(t, cached)
}

func TestClassifyTencentError(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{fmt.Errorf("AuthFailure.SignatureFailure"), "auth_error"},
		{fmt.Errorf("InvalidCredential"), "auth_error"},
		{fmt.Errorf("RequestLimitExceeded"), "limit_error"},
		{fmt.Errorf("read tcp 1.2.3.4:443: i/o timeout"), "network_error"},
		{fmt.Errorf("dial tcp: network unreachable"), "network_error"},
		{fmt.Errorf("InternalError"), "error"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, providerscommon.ClassifyTencentError(tt.err))
	}
}

func TestMinPeriodForMetric(t *testing.T) {
	// Backup and restore describeBaseMetricsJSON
	backup := describeBaseMetricsJSON
	defer func() { describeBaseMetricsJSON = backup }()

	// Case 1: Success with int period
	describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
		jsonStr := `{
			"MetricSet": [
				{
					"MetricName": "CPUUsage",
					"Period": 60
				},
				{
					"MetricName": "MemUsage",
					"Periods": [10, 60, 300]
				},
				{
					"MetricName": "DiskUsage",
					"Periods": ["60", "300"]
				}
			]
		}`
		return []byte(jsonStr), nil
	}

	account := config.CloudAccount{}
	// Test Cache Miss -> Hit
	p := minPeriodForMetric("ap-guangzhou", account, "ns", "CPUUsage", 60)
	assert.Equal(t, int64(60), p)

	// Test Cache Hit
	p = minPeriodForMetric("ap-guangzhou", account, "ns", "CPUUsage", 60)
	assert.Equal(t, int64(60), p)

	// Test Periods array (min of 10, 60, 300 -> 10)
	p = minPeriodForMetric("ap-guangzhou", account, "ns", "MemUsage", 60)
	assert.Equal(t, int64(10), p)

	// Test Periods string array
	p = minPeriodForMetric("ap-guangzhou", account, "ns", "DiskUsage", 60)
	assert.Equal(t, int64(60), p)

	// Case 2: API Error
	describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
		return nil, fmt.Errorf("api error")
	}
	// Clear cache for this key
	key := "ns|Unknown"
	periodMu.Lock()
	delete(periodCache, key)
	periodMu.Unlock()

	p = minPeriodForMetric("ap-guangzhou", account, "ns", "Unknown", 60)
	assert.Equal(t, int64(60), p) // Default fallback
}
