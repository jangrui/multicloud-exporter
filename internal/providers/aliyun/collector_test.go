package aliyun

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
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
