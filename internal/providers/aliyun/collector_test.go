package aliyun

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	metrics "multicloud-exporter/internal/metrics"

	_ "multicloud-exporter/internal/metrics/aliyun"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestCollector_getAccountUID(t *testing.T) {
	mockSTS := &mockSTSClient{
		GetCallerIdentityFunc: func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
			return &sts.GetCallerIdentityResponse{
				AccountId: "123456789",
			}, nil
		},
	}
	factory := &mockClientFactory{sts: mockSTS}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	uid := c.getAccountUID(config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}, "cn-hangzhou")

	assert.Equal(t, "123456789", uid)

	// Test Cache
	mockSTS.GetCallerIdentityFunc = func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
		return nil, fmt.Errorf("should not be called")
	}
	uidCached := c.getAccountUID(config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}, "cn-hangzhou")
	assert.Equal(t, "123456789", uidCached)
}

func TestCollector_getAccountUID_Error(t *testing.T) {
	mockSTS := &mockSTSClient{
		GetCallerIdentityFunc: func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
			return nil, fmt.Errorf("sts error")
		},
	}
	factory := &mockClientFactory{sts: mockSTS}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	uid := c.getAccountUID(config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}, "cn-hangzhou")

	assert.Equal(t, "test-account", uid)
}

func TestCollector_getAllRegions(t *testing.T) {
	mockECS := &mockECSClient{
		DescribeRegionsFunc: func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
			return &ecs.DescribeRegionsResponse{
				Regions: ecs.Regions{
					Region: []ecs.Region{
						{RegionId: "cn-hangzhou"},
						{RegionId: "cn-shanghai"},
					},
				},
			}, nil
		},
	}
	factory := &mockClientFactory{ecs: mockECS}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	regions := c.getAllRegions(config.CloudAccount{
		AccessKeyID: "ak",
	})
	assert.Len(t, regions, 2)
	assert.Contains(t, regions, "cn-hangzhou")
	assert.Contains(t, regions, "cn-shanghai")
}

func TestCollector_getAllRegions_Error(t *testing.T) {
	mockECS := &mockECSClient{
		DescribeRegionsFunc: func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
			return nil, fmt.Errorf("ecs error")
		},
	}
	factory := &mockClientFactory{ecs: mockECS}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	regions := c.getAllRegions(config.CloudAccount{
		AccessKeyID: "ak",
		AccountID:   "acc",
	})
	assert.Len(t, regions, 1)
	assert.Equal(t, "cn-hangzhou", regions[0])
}

func TestCollector_Collect(t *testing.T) {
	// Setup Mocks
	mockECS := &mockECSClient{
		DescribeRegionsFunc: func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
			return &ecs.DescribeRegionsResponse{
				Regions: ecs.Regions{
					Region: []ecs.Region{{RegionId: "cn-hangzhou"}},
				},
			}, nil
		},
	}

	mockSLB := &mockSLBClient{
		DescribeLoadBalancersFunc: func(request *slb.DescribeLoadBalancersRequest) (*slb.DescribeLoadBalancersResponse, error) {
			return &slb.DescribeLoadBalancersResponse{
				LoadBalancers: slb.LoadBalancers{
					LoadBalancer: []slb.LoadBalancer{
						{LoadBalancerId: "lb-123"},
					},
				},
			}, nil
		},
		DescribeLoadBalancerAttributeFunc: func(request *slb.DescribeLoadBalancerAttributeRequest) (*slb.DescribeLoadBalancerAttributeResponse, error) {
			return &slb.DescribeLoadBalancerAttributeResponse{}, nil
		},
	}

	mockTag := &mockTagClient{
		ListTagResourcesFunc: func(request *tag.ListTagResourcesRequest) (*tag.ListTagResourcesResponse, error) {
			resp := &tag.ListTagResourcesResponse{
				BaseResponse: &responses.BaseResponse{},
			}
			return resp, nil
		},
	}

	mockOSS := &mockOSSClient{
		ListBucketsFunc: func(options ...oss.Option) (oss.ListBucketsResult, error) {
			return oss.ListBucketsResult{
				Buckets: []oss.BucketProperties{
					{Name: "bucket-1"},
				},
			}, nil
		},
	}

	mockVPC := &mockVPCClient{
		DescribeCommonBandwidthPackagesFunc: func(request *vpc.DescribeCommonBandwidthPackagesRequest) (*vpc.DescribeCommonBandwidthPackagesResponse, error) {
			return &vpc.DescribeCommonBandwidthPackagesResponse{
				CommonBandwidthPackages: vpc.CommonBandwidthPackages{
					CommonBandwidthPackage: []vpc.CommonBandwidthPackage{
						{BandwidthPackageId: "cbwp-123"},
					},
				},
			}, nil
		},
		ListTagResourcesFunc: func(request *vpc.ListTagResourcesRequest) (*vpc.ListTagResourcesResponse, error) {
			return &vpc.ListTagResourcesResponse{
				TagResources: vpc.TagResourcesInListTagResources{
					TagResource: []vpc.TagResource{
						{ResourceId: "cbwp-123", TagKey: "env", TagValue: "prod"},
					},
				},
			}, nil
		},
	}

	mockSTS := &mockSTSClient{
		GetCallerIdentityFunc: func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
			return &sts.GetCallerIdentityResponse{AccountId: "123"}, nil
		},
	}

	mockCMS := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (*cms.DescribeMetricMetaListResponse, error) {
			resp := &cms.DescribeMetricMetaListResponse{}
			jsonStr := `{
				"Resources": {
					"Resource": [
						{
							"Dimensions": "instanceId",
							"Statistics": "Average,Maximum",
							"Periods": "60,300"
						}
					]
				}
			}`
			_ = json.Unmarshal([]byte(jsonStr), resp)
			return resp, nil
		},
		DescribeMetricLastFunc: func(request *cms.DescribeMetricLastRequest) (*cms.DescribeMetricLastResponse, error) {
			dp := []map[string]interface{}{
				{
					"instanceId": "lb-123",
					"Average":    10.0,
					"Maximum":    20.0,
				},
				{
					"instanceId": "bucket-1",
					"Average":    100.0,
					"Maximum":    200.0,
				},
				{
					"instanceId": "cbwp-123",
					"Average":    50.0,
					"Maximum":    60.0,
				},
			}
			dpBytes, _ := json.Marshal(dp)
			return &cms.DescribeMetricLastResponse{
				Datapoints: string(dpBytes),
			}, nil
		},
	}

	factory := &mockClientFactory{
		ecs: mockECS,
		cms: mockCMS,
		slb: mockSLB,
		tag: mockTag,
		sts: mockSTS,
		oss: mockOSS,
		vpc: mockVPC,
	}

	cfg := &config.Config{
		Server: &config.ServerConf{
			RegionConcurrency: 1,
			MetricConcurrency: 1,
		},
		Estimation: &config.EstimationConf{
			CLB: &config.CLBEstimationConf{
				AliyunBandwidthCapBps: 100,
				PerInstanceCapBps:     map[string]int{"lb-123": 100},
			},
		},
	}
	mgr := discovery.NewManager(cfg)

	setDiscoveryProducts(t, mgr, map[string][]config.Product{
		"aliyun": {
			{
				Namespace: "acs_slb_dashboard",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"TrafficRXNew"},
					},
				},
			},
			{
				Namespace: "acs_oss_dashboard",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"UserIntranetFlow"},
					},
				},
			},
			{
				Namespace: "acs_bandwidth_package",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"UpstreamTraffic"},
					},
				},
			},
		},
	})

	c := NewCollector(cfg, mgr)
	c.clientFactory = factory

	// Execute Collect
	c.Collect(config.CloudAccount{
		AccountID:       "test-acc",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Regions:         []string{"*"},
		Resources:       []string{"*"},
	})
}

func TestCollector_ALIYUN_CLB_Utilization_Estimate(t *testing.T) {
	// Reset metrics to avoid interference from previous tests
	metrics.Reset()

	// Prepare minimal mocks reusing previous setup
	mockECS := &mockECSClient{
		DescribeRegionsFunc: func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
			return &ecs.DescribeRegionsResponse{
				Regions: ecs.Regions{
					Region: []ecs.Region{{RegionId: "cn-hangzhou"}},
				},
			}, nil
		},
	}
	mockSLB := &mockSLBClient{
		DescribeLoadBalancersFunc: func(request *slb.DescribeLoadBalancersRequest) (*slb.DescribeLoadBalancersResponse, error) {
			return &slb.DescribeLoadBalancersResponse{
				LoadBalancers: slb.LoadBalancers{
					LoadBalancer: []slb.LoadBalancer{
						{LoadBalancerId: "lb-123"},
					},
				},
			}, nil
		},
		DescribeLoadBalancerAttributeFunc: func(request *slb.DescribeLoadBalancerAttributeRequest) (*slb.DescribeLoadBalancerAttributeResponse, error) {
			return &slb.DescribeLoadBalancerAttributeResponse{}, nil
		},
	}
	mockCMS := &mockCMSClient{
		DescribeMetricMetaListFunc: func(request *cms.DescribeMetricMetaListRequest) (*cms.DescribeMetricMetaListResponse, error) {
			resp := &cms.DescribeMetricMetaListResponse{}
			jsonStr := `{
				"Resources": {
					"Resource": [
						{
							"Dimensions": "instanceId",
							"Statistics": "Average",
							"Periods": "60"
						}
					]
				}
			}`
			_ = json.Unmarshal([]byte(jsonStr), resp)
			return resp, nil
		},
		DescribeMetricLastFunc: func(request *cms.DescribeMetricLastRequest) (*cms.DescribeMetricLastResponse, error) {
			dp := []map[string]interface{}{
				{"instanceId": "lb-123", "Average": 10.0},
			}
			dpBytes, _ := json.Marshal(dp)
			return &cms.DescribeMetricLastResponse{Datapoints: string(dpBytes)}, nil
		},
	}
	mockTag := &mockTagClient{
		ListTagResourcesFunc: func(request *tag.ListTagResourcesRequest) (*tag.ListTagResourcesResponse, error) {
			resp := &tag.ListTagResourcesResponse{
				BaseResponse: &responses.BaseResponse{},
				TagResources: []tag.TagResource{
					{
						ResourceId: "lb-123",
						Tags: []tag.Tag{
							{Key: "BandwidthCapBps", Value: "1000"},
						},
					},
				},
			}
			return resp, nil
		},
	}
	mockSTS := &mockSTSClient{
		GetCallerIdentityFunc: func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
			return &sts.GetCallerIdentityResponse{AccountId: "123"}, nil
		},
	}
	factory := &mockClientFactory{ecs: mockECS, cms: mockCMS, slb: mockSLB, tag: mockTag, sts: mockSTS}
	cfg := &config.Config{
		Server: &config.ServerConf{RegionConcurrency: 1, MetricConcurrency: 1},
		Estimation: &config.EstimationConf{
			CLB: &config.CLBEstimationConf{
				// 配置文件中故意设置一个较小值，验证 Tag 优先级更高 (1000 > 100)
				AliyunBandwidthCapBps: 100,
			},
		},
	}
	mgr := discovery.NewManager(cfg)
	setDiscoveryProducts(t, mgr, map[string][]config.Product{
		"aliyun": {{
			Namespace: "acs_slb_dashboard",
			MetricInfo: []config.MetricGroup{{
				MetricList: []string{"InstanceTrafficRX"},
			}},
		}},
	})
	c := NewCollector(cfg, mgr)
	c.clientFactory = factory
	c.Collect(config.CloudAccount{
		AccountID:       "test-acc",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Regions:         []string{"*"},
		Resources:       []string{"*"},
	})
	// Check metric is registered
	mfs, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)
	found := false
	var utilVal float64
	for _, mf := range mfs {
		if mf.GetName() == "clb_traffic_rx_utilization_pct" {
			// Match the specific metric with correct labels: test-acc, cn-hangzhou, clb, lb-123
			for _, metric := range mf.GetMetric() {
				labels := metric.GetLabel()
				accountID := ""
				region := ""
				resourceID := ""
				for _, label := range labels {
					switch label.GetName() {
					case "account_id":
						accountID = label.GetValue()
					case "region":
						region = label.GetValue()
					case "resource_id":
						resourceID = label.GetValue()
					}
				}
				// Match the specific resource we're testing
				if accountID == "test-acc" && region == "cn-hangzhou" && resourceID == "lb-123" {
					found = true
					utilVal = metric.GetGauge().GetValue()
					break
				}
			}
			if found {
				break
			}
		}
	}
	assert.True(t, found, "expected clb_traffic_rx_utilization_pct to be gathered for lb-123")
	// 10.0 bps / 1000 bps * 100 = 1.0%
	assert.InDelta(t, 1.0, utilVal, 0.001, "utilization calculation mismatch: expected 1.0% but got %.2f%%", utilVal)
}

func setDiscoveryProducts(t *testing.T, mgr *discovery.Manager, products map[string][]config.Product) {
	// Use reflection to set private field 'products'
	val := reflect.ValueOf(mgr).Elem()
	field := val.FieldByName("products")
	if !field.IsValid() {
		t.Fatal("field 'products' not found in discovery.Manager")
	}

	// Create a new map accessible via reflection
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(products))
}
