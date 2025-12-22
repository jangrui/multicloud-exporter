package discovery

import (
	"context"
	"errors"
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

type mockMonitorClient struct {
	DescribeBaseMetricsFunc func(request *monitor.DescribeBaseMetricsRequest) (response *monitor.DescribeBaseMetricsResponse, err error)
}

func (m *mockMonitorClient) DescribeBaseMetrics(request *monitor.DescribeBaseMetricsRequest) (response *monitor.DescribeBaseMetricsResponse, err error) {
	if m.DescribeBaseMetricsFunc != nil {
		return m.DescribeBaseMetricsFunc(request)
	}
	return nil, nil
}

func TestTencentDiscoverer_Discover(t *testing.T) {
	// Backup and restore newTencentMonitorClient
	originalNewTencentMonitorClient := newTencentMonitorClient
	defer func() { newTencentMonitorClient = originalNewTencentMonitorClient }()

	d := &TencentDiscoverer{}
	ctx := context.Background()

	// Test case 1: Nil config
	assert.Nil(t, d.Discover(ctx, nil))

	// Test case 2: No accounts
	cfg := &config.Config{}
	assert.Nil(t, d.Discover(ctx, cfg))

	// Test case 3: Success with BWP, CLB, COS
	cfg = &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"tencent": {
			{
				Provider:        "tencent",
				AccessKeyID:     "ak",
				AccessKeySecret: "sk",
				Regions:         []string{"ap-guangzhou"},
				Resources:       []string{"bwp", "clb", "s3"},
			},
			},
		},
	}

	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				resp := monitor.NewDescribeBaseMetricsResponse()
				switch ns := *request.Namespace; ns {
				case "QCE/BWP":
					metricName := "OutTraffic"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				case "QCE/LB":
					metricName := "ClientConnum"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				case "QCE/COS":
					metricName := "Requests"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				}
				return resp, nil
			},
			},
		}, nil
	}

	prods := d.Discover(ctx, cfg)
	assert.GreaterOrEqual(t, len(prods), 3)

	// Verify QCE/BWP
	foundBWP := false
	for _, p := range prods {
		if p.Namespace == "QCE/BWP" {
			foundBWP = true
			// Verify fallback metrics are added
			assert.Contains(t, p.MetricInfo[0].MetricList, "InTraffic")
		}
	}
	assert.True(t, foundBWP)

	// Verify CLB namespace exists (only QCE/LB)
	foundCLB := false
	for _, p := range prods {
		if p.Namespace == "QCE/LB" {
			foundCLB = true
			break
		}
	}
	assert.True(t, foundCLB)

	// Verify GWLB namespace exists (qce/gwlb) when requested
	// Note: GWLB (qce/gwlb) 仅在资源列表包含 gwlb 或通配符时出现，测试环境不强制断言

	// Test case 4: Client error
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return nil, errors.New("client error")
	}
	prods = d.Discover(ctx, cfg)
	// Expect 3 products (BWP, CLB, COS) because fallback metrics should be used
	assert.Len(t, prods, 3)
	for _, p := range prods {
		assert.NotEmpty(t, p.MetricInfo)
		assert.NotEmpty(t, p.MetricInfo[0].MetricList)
	}
}

func TestTencentDiscoverer_Discover_COS_Fallback(t *testing.T) {
	// Backup and restore newTencentMonitorClient
	originalNewTencentMonitorClient := newTencentMonitorClient
	defer func() { newTencentMonitorClient = originalNewTencentMonitorClient }()

	d := &TencentDiscoverer{}
	ctx := context.Background()
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"tencent": {
			{
				Provider:        "tencent",
				AccessKeyID:     "ak",
				AccessKeySecret: "sk",
				Regions:         []string{"ap-guangzhou"},
				Resources:       []string{"s3"},
			},
			},
		},
	}

	// Mock API failure
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				return nil, errors.New("api failed")
			},
			},
		}, nil
	}

	prods := d.Discover(ctx, cfg)
	// Expect COS to be present with fallback metrics
	assert.Len(t, prods, 1)
	assert.Equal(t, "QCE/COS", prods[0].Namespace)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "StdStorage")
}

func TestFetchTencentMetricMeta(t *testing.T) {
	// Backup and restore newTencentMonitorClient
	originalNewTencentMonitorClient := newTencentMonitorClient
	defer func() { newTencentMonitorClient = originalNewTencentMonitorClient }()

	// Test case 1: Success
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				resp := monitor.NewDescribeBaseMetricsResponse()
				metricName := "TestMetric"
				unit := "count"
				resp.Response = &monitor.DescribeBaseMetricsResponseParams{
					MetricSet: []*monitor.MetricSet{
						{
							MetricName: &metricName,
							Unit:       &unit,
						},
					},
				}
				return resp, nil
			},
			},
		}, nil
	}

	metas, err := FetchTencentMetricMeta("region", "ak", "sk", "QCE/TEST")
	assert.NoError(t, err)
	assert.Len(t, metas, 1)
	assert.Equal(t, "TestMetric", metas[0].Name)
	assert.Equal(t, "count", metas[0].Unit)

	// Test case 2: Client error
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return nil, errors.New("client error")
	}
	_, err = FetchTencentMetricMeta("region", "ak", "sk", "QCE/TEST")
	assert.Error(t, err)

	// Test case 4: Complex Dimensions and Meaning
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				resp := monitor.NewDescribeBaseMetricsResponse()
				metricName := "ComplexMetric"
				unit := "s"
				zhMeaning := "测试指标"

				// Construct complex dimensions structure similar to Tencent API response
				// Since we can't easily construct the specific type for Dimensions (it's interface{}),
				// we rely on the fact that the SDK likely unmarshals it or we can pass it if it's just interface{}
				// In the source code: if b, e := json.Marshal(m.Dimensions); e == nil

				// We can mock the Dimensions field if we can set it.
				// The SDK struct field Dimensions is []*monitor.Dimension which is NOT what the code seems to expect?
				// Wait, let's check the code: m.Dimensions is referenced.
				// In v20180724, MetricSet.Dimensions is []*DimensionNew.
				// But the code does json.Marshal(m.Dimensions).

				// Let's look at the source code again.
				// "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
				// MetricSet struct has Dimensions []*DimensionNew `json:"Dimensions,omitnil" name:"Dimensions"`

				// So we need to populate DimensionNew.

				dName := "instanceId"
				dims := []*monitor.DimensionsDesc{
					{Dimensions: []*string{&dName}},
				}

				resp.Response = &monitor.DescribeBaseMetricsResponseParams{
					MetricSet: []*monitor.MetricSet{
						{
							MetricName: &metricName,
							Unit:       &unit,
							Meaning: &monitor.MetricObjectMeaning{
								Zh: &zhMeaning,
							},
							Dimensions: dims,
						},
					},
				}
				return resp, nil
			},
			},
		}, nil
	}
	metas, err = FetchTencentMetricMeta("region", "ak", "sk", "QCE/TEST")
	assert.NoError(t, err)
	assert.Len(t, metas, 1)
	assert.Equal(t, "ComplexMetric", metas[0].Name)
	assert.Equal(t, "测试指标", metas[0].Description)
	// The code tries to parse Dimensions via JSON marshal/unmarshal to []map[string]interface{}
	// This implies the structure it expects is different from what I just mocked or the code handles it generically.
	// If m.Dimensions is []*DimensionNew, json.Marshal will produce [{"Key":"instanceId", ...}]
	// The code checks for "Name" or "Key" in the map.
	assert.Contains(t, metas[0].Dimensions, "instanceId")

	// Test case 5: Empty dimensions fallback
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				resp := monitor.NewDescribeBaseMetricsResponse()
				metricName := "FallbackMetric"
				resp.Response = &monitor.DescribeBaseMetricsResponseParams{
					MetricSet: []*monitor.MetricSet{
						{MetricName: &metricName},
					},
				}
				return resp, nil
			},
			},
		}, nil
	}
	// Need to register a default mapping for this to work, or use an existing one.
	// The code uses config.DefaultResourceDimMapping() which has "tencent.QCE/CVM" etc.
	// Let's use "QCE/CVM" namespace.
	metas, err = FetchTencentMetricMeta("region", "ak", "sk", "QCE/CVM")
	assert.NoError(t, err)
	assert.Len(t, metas, 1)
	assert.Equal(t, "FallbackMetric", metas[0].Name)
	// Default mapping for QCE/CVM is "InstanceId"
	assert.Contains(t, metas[0].Dimensions, "InstanceId")
}
