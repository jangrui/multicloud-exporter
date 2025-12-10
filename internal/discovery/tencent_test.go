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
		AccountsList: []config.CloudAccount{
			{
				Provider:        "tencent",
				AccessKeyID:     "ak",
				AccessKeySecret: "sk",
				Regions:         []string{"ap-guangzhou"},
				Resources:       []string{"bwp", "clb", "cos"},
			},
		},
	}

	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return &mockMonitorClient{
			DescribeBaseMetricsFunc: func(request *monitor.DescribeBaseMetricsRequest) (*monitor.DescribeBaseMetricsResponse, error) {
				resp := monitor.NewDescribeBaseMetricsResponse()
				if *request.Namespace == "QCE/BWP" {
					metricName := "OutTraffic"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				} else if *request.Namespace == "QCE/LB" {
					metricName := "ClientConnum"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				} else if *request.Namespace == "QCE/COS" {
					metricName := "Requests"
					resp.Response = &monitor.DescribeBaseMetricsResponseParams{
						MetricSet: []*monitor.MetricSet{
							{MetricName: &metricName},
						},
					}
				}
				return resp, nil
			},
		}, nil
	}

	prods := d.Discover(ctx, cfg)
	assert.Len(t, prods, 3)

	// Verify QCE/BWP
	foundBWP := false
	for _, p := range prods {
		if p.Namespace == "QCE/BWP" {
			foundBWP = true
			assert.Contains(t, p.MetricInfo[0].MetricList, "OutTraffic")
			// Verify fallback metrics are added
			assert.Contains(t, p.MetricInfo[0].MetricList, "InTraffic")
		}
	}
	assert.True(t, foundBWP)

	// Test case 4: Client error
	newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
		return nil, errors.New("client error")
	}
	prods = d.Discover(ctx, cfg)
	assert.Len(t, prods, 0)
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
