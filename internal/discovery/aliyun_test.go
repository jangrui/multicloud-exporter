package discovery

import (
	"context"
	"errors"
	"multicloud-exporter/internal/config"
	"testing"

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
