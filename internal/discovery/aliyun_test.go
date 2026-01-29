package discovery

import (
	"context"
	"errors"
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/stretchr/testify/assert"
)

type mockCMSClient struct {
	DescribeMetricMetaListFunc func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error)
}

func (m *mockCMSClient) DescribeMetricMetaList(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
	if m.DescribeMetricMetaListFunc != nil {
		return m.DescribeMetricMetaListFunc(request)
	}
	return nil, nil
}

func TestAliyunDiscoverer_Discover(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Setup mock
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			if request.Namespace == "acs_slb_dashboard" {
				return &cms.DescribeMetricMetaListResponse{
					Resources: cms.ResourcesInDescribeMetricMetaList{
						Resource: []cms.Resource{
							{MetricName: "InstanceTrafficRXUtilization", Dimensions: "instanceId,userId"},
							{MetricName: "FilteredOut", Dimensions: "unknown"},
						},
					},
				}, nil
			}
			return nil, errors.New("error")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	d := &AliyunDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"clb"},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_slb_dashboard", prods[0].Namespace)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "InstanceTrafficRXUtilization")
	// Check fallback metrics
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "InstanceTrafficRX")
}

func TestAliyunDiscoverer_Discover_BWP_Fallback(t *testing.T) {
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			return nil, errors.New("error")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	d := &AliyunDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"bwp"},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.NotEmpty(t, prods)
	found := false
	for _, p := range prods {
		if p.Namespace == "acs_bandwidth_package" {
			found = true
			assert.Contains(t, p.MetricInfo[0].MetricList, "net_rx.rate")
			assert.Contains(t, p.MetricInfo[0].MetricList, "net_tx.rate")
			break
		}
	}
	assert.True(t, found)
}

func TestAliyunDiscoverer_Discover_OSS_Fallback(t *testing.T) {
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			return nil, errors.New("error")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	d := &AliyunDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"s3"},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.NotEmpty(t, prods)
	found := false
	for _, p := range prods {
		if p.Namespace == "acs_oss_dashboard" {
			found = true
			assert.Contains(t, p.MetricInfo[0].MetricList, "UserStorage")
			assert.Contains(t, p.MetricInfo[0].MetricList, "InternetRecv")
			break
		}
	}
	assert.True(t, found)
}

func TestAliyunDiscoverer_Discover_NewProducts_Fallback(t *testing.T) {
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			return nil, errors.New("error")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	d := &AliyunDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"alb", "nlb", "gwlb"},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 3)

	for _, p := range prods {
		switch p.Namespace {
		case "acs_alb":
			assert.Contains(t, p.MetricInfo[0].MetricList, "LoadBalancerQPS")
		case "acs_nlb":
			assert.Contains(t, p.MetricInfo[0].MetricList, "InstanceTrafficRX")
		case "acs_gwlb":
			assert.Contains(t, p.MetricInfo[0].MetricList, "ActiveConnection")
		}
	}
}

func TestFetchAliyunMetricMeta(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Setup mock
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			return &cms.DescribeMetricMetaListResponse{
				Resources: cms.ResourcesInDescribeMetricMetaList{
					Resource: []cms.Resource{
						{
							MetricName:  "TestMetric",
							Dimensions:  "dim1,dim2",
							Unit:        "Count",
							Description: "Test Desc",
						},
					},
				},
			}, nil
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	metas, err := FetchAliyunMetricMeta("cn-hangzhou", "ak", "sk", "acs_test")
	assert.NoError(t, err)
	assert.Len(t, metas, 1)
	assert.Equal(t, "TestMetric", metas[0].Name)
	assert.Equal(t, "Count", metas[0].Unit)
	assert.Equal(t, []string{"dim1", "dim2"}, metas[0].Dimensions)
}

func TestFetchAliyunMetricMeta_Error(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Setup mock factory error
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return nil, errors.New("factory error")
	}
	_, err := FetchAliyunMetricMeta("cn-hangzhou", "ak", "sk", "acs_test")
	assert.Error(t, err)

	// Setup mock api error
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return &mockCMSClient{
			DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
				return nil, errors.New("api error")
			},
		}, nil
	}
	_, err = FetchAliyunMetricMeta("cn-hangzhou", "ak", "sk", "acs_test")
	assert.Error(t, err)
}

func TestAliyunDiscovery_OSS_CustomConfig(t *testing.T) {
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Mock client that should not be called when custom config is used
	callCount := 0
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			callCount++
			return nil, errors.New("should not be called")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	period := 120
	d := &AliyunDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"s3"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"oss": {
							{
								MetricList: []string{"CustomMetric1", "CustomMetric2"},
								Period:     &period,
							},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_oss_dashboard", prods[0].Namespace)
	assert.False(t, prods[0].AutoDiscover) // 不应该调用元数据 API

	// 验证自定义指标
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric2")

	// 验证 API 没有被调用
	assert.Equal(t, 0, callCount)
}

func TestAliyunDiscovery_BuildNLBProductDefault(t *testing.T) {
	fallbackList := []string{"InstanceActiveConnection", "DropConnection", "InstanceTrafficRX"}
	product := buildNLBProductDefault(fallbackList)

	assert.Equal(t, "acs_nlb", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "InstanceActiveConnection")
	assert.Contains(t, product.MetricInfo[0].MetricList, "DropConnection")
	assert.Contains(t, product.MetricInfo[0].MetricList, "InstanceTrafficRX")
}

func TestAliyunDiscovery_BuildNLBProductWithCustomConfig(t *testing.T) {
	period := 120
	metricGroups := []config.MetricGroupConfig{
		{
			MetricList: []string{"CustomMetric1", "CustomMetric2"},
			Period:     &period,
		},
	}

	product := buildNLBProductWithCustomConfig(metricGroups)

	assert.Equal(t, "acs_nlb", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestAliyunDiscovery_NLB_CustomConfig(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Mock client that should not be called when custom config is used
	callCount := 0
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			callCount++
			return nil, errors.New("should not be called")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	period := 120
	d := &AliyunDiscoverer{}
	ctx := context.Background()
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"nlb"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"nlb": {
							{
								MetricList: []string{"CustomMetric1", "CustomMetric2"},
								Period:     &period,
							},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(ctx, cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_nlb", prods[0].Namespace)
	assert.False(t, prods[0].AutoDiscover) // 不应该调用元数据 API

	// 验证自定义指标
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric2")

	// 验证 API 没有被调用
	assert.Equal(t, 0, callCount)
}

func TestAliyunDiscovery_BuildCLBProductDefault(t *testing.T) {
	fallbackList := []string{"InstanceTrafficRX", "InstanceDropPacketRX", "Qps"}
	product := buildCLBProductDefault(fallbackList)

	assert.Equal(t, "acs_slb_dashboard", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "InstanceTrafficRX")
	assert.Contains(t, product.MetricInfo[0].MetricList, "InstanceDropPacketRX")
	assert.Contains(t, product.MetricInfo[0].MetricList, "Qps")
}

func TestAliyunDiscovery_BuildCLBProductWithCustomConfig(t *testing.T) {
	period := 120
	metricGroups := []config.MetricGroupConfig{
		{
			MetricList: []string{"CustomMetric1", "CustomMetric2"},
			Period:     &period,
		},
	}

	product := buildCLBProductWithCustomConfig(metricGroups)

	assert.Equal(t, "acs_slb_dashboard", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestAliyunDiscovery_CLB_CustomConfig(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Mock client that should not be called when custom config is used
	callCount := 0
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			callCount++
			return nil, errors.New("should not be called")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	period := 120
	d := &AliyunDiscoverer{}
	ctx := context.Background()
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"clb"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"clb": {
							{
								MetricList: []string{"CustomMetric1", "CustomMetric2"},
								Period:     &period,
							},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(ctx, cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_slb_dashboard", prods[0].Namespace)
	assert.False(t, prods[0].AutoDiscover) // 不应该调用元数据 API

	// 验证自定义指标
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric2")

	// 验证 API 没有被调用
	assert.Equal(t, 0, callCount)
}

func TestAliyunDiscovery_BuildALBProductDefault(t *testing.T) {
	fallbackList := []string{"LoadBalancerActiveConnection", "LoadBalancerQPS", "LoadBalancerHTTPCode2XX"}
	product := buildALBProductDefault(fallbackList)

	assert.Equal(t, "acs_alb", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "LoadBalancerActiveConnection")
	assert.Contains(t, product.MetricInfo[0].MetricList, "LoadBalancerQPS")
	assert.Contains(t, product.MetricInfo[0].MetricList, "LoadBalancerHTTPCode2XX")
}

func TestAliyunDiscovery_BuildALBProductWithCustomConfig(t *testing.T) {
	period := 120
	metricGroups := []config.MetricGroupConfig{
		{
			MetricList: []string{"CustomMetric1", "CustomMetric2"},
			Period:     &period,
		},
	}

	product := buildALBProductWithCustomConfig(metricGroups)

	assert.Equal(t, "acs_alb", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestAliyunDiscovery_ALB_CustomConfig(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Mock client that should not be called when custom config is used
	callCount := 0
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			callCount++
			return nil, errors.New("should not be called")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	period := 120
	d := &AliyunDiscoverer{}
	ctx := context.Background()
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"alb"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"alb": {
							{
								MetricList: []string{"CustomMetric1", "CustomMetric2"},
								Period:     &period,
							},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(ctx, cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_alb", prods[0].Namespace)
	assert.False(t, prods[0].AutoDiscover) // 不应该调用元数据 API

	// 验证自定义指标
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric2")

	// 验证 API 没有被调用
	assert.Equal(t, 0, callCount)
}

func TestAliyunDiscovery_BuildBWPProductDefault(t *testing.T) {
	fallbackList := []string{"net_rx.rate", "net_tx.rate", "in_bandwidth_utilization"}
	product := buildBWPProductDefault(fallbackList)

	assert.Equal(t, "acs_bandwidth_package", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "net_rx.rate")
	assert.Contains(t, product.MetricInfo[0].MetricList, "net_tx.rate")
	assert.Contains(t, product.MetricInfo[0].MetricList, "in_bandwidth_utilization")
}

func TestAliyunDiscovery_BuildBWPProductWithCustomConfig(t *testing.T) {
	period := 120
	metricGroups := []config.MetricGroupConfig{
		{
			MetricList: []string{"CustomMetric1", "CustomMetric2"},
			Period:     &period,
		},
	}

	product := buildBWPProductWithCustomConfig(metricGroups)

	assert.Equal(t, "acs_bandwidth_package", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestAliyunDiscovery_BWP_CustomConfig(t *testing.T) {
	// Backup and restore
	oldFactory := newAliyunCMSClient
	defer func() { newAliyunCMSClient = oldFactory }()

	// Mock client that should not be called when custom config is used
	callCount := 0
	mock := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
			callCount++
			return nil, errors.New("should not be called")
		},
	}
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return mock, nil
	}

	period := 120
	d := &AliyunDiscoverer{}
	ctx := context.Background()
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aliyun": {
				{
					Provider:        "aliyun",
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Regions:         []string{"cn-hangzhou"},
					Resources:       []string{"bwp"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"bwp": {
							{
								MetricList: []string{"CustomMetric1", "CustomMetric2"},
								Period:     &period,
							},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(ctx, cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "acs_bandwidth_package", prods[0].Namespace)
	assert.False(t, prods[0].AutoDiscover) // 不应该调用元数据 API

	// 验证自定义指标
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric2")

	// 验证 API 没有被调用
	assert.Equal(t, 0, callCount)
}
