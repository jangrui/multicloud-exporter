package aliyun

import (
	"encoding/json"
	"fmt"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"os"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/stretchr/testify/assert"
)

func TestListOSSIDs_Pagination(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	c.ossCache = make(map[string]ossCacheEntry)

	mockOSS := &mockOSSClient{}
	callCount := 0
	mockOSS.ListBucketsFunc = func(options ...oss.Option) (oss.ListBucketsResult, error) {
		callCount++
		if callCount == 1 {
			return oss.ListBucketsResult{
				Buckets:     []oss.BucketProperties{{Name: "b1", Location: "oss-cn-hangzhou"}},
				IsTruncated: true,
				NextMarker:  "next",
			}, nil
		}
		return oss.ListBucketsResult{
			Buckets:     []oss.BucketProperties{{Name: "b2", Location: "oss-cn-hangzhou"}},
			IsTruncated: false,
		}, nil
	}

	c.clientFactory = &mockClientFactory{oss: mockOSS}
	ids := c.listOSSIDs(config.CloudAccount{AccountID: "acc2"}, "cn-hangzhou")
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "b1")
	assert.Contains(t, ids, "b2")
}

func TestListCBWPIDs(t *testing.T) {
	// Set PageSize to 1 to test pagination loop
	c := NewCollector(&config.Config{
		ServerConf: &config.ServerConf{
			PageSize: 1,
		},
	}, nil)

	callCount := 0
	mockVPC := &mockVPCClient{
		DescribeCommonBandwidthPackagesFunc: func(request *vpc.DescribeCommonBandwidthPackagesRequest) (*vpc.DescribeCommonBandwidthPackagesResponse, error) {
			callCount++
			resp := vpc.CreateDescribeCommonBandwidthPackagesResponse()
			switch callCount {
			case 1:
				resp.TotalCount = 2
				resp.PageNumber = 1
				resp.PageSize = 1
				resp.CommonBandwidthPackages.CommonBandwidthPackage = []vpc.CommonBandwidthPackage{
					{BandwidthPackageId: "cbwp-1"},
				}
			case 2:
				resp.TotalCount = 2
				resp.PageNumber = 2
				resp.CommonBandwidthPackages.CommonBandwidthPackage = []vpc.CommonBandwidthPackage{
					{BandwidthPackageId: "cbwp-2"},
				}
			default:
				// Stop the loop
				resp.TotalCount = 2
				resp.PageNumber = 3
				resp.CommonBandwidthPackages.CommonBandwidthPackage = []vpc.CommonBandwidthPackage{}
			}
			return resp, nil
		},
	}

	c.clientFactory = &mockClientFactory{vpc: mockVPC}
	ids := c.listCBWPIDs(config.CloudAccount{AccountID: "acc1"}, "cn-hangzhou")
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "cbwp-1")
	assert.Contains(t, ids, "cbwp-2")
}

func TestFetchCBWPTags(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	mockVPC := &mockVPCClient{}
	c.clientFactory = &mockClientFactory{vpc: mockVPC}

	// Case 1: Empty IDs
	tags := c.fetchCBWPTags(config.CloudAccount{}, "cn-hangzhou", nil)
	assert.Empty(t, tags)

	// Case 2: Success with CodeName
	mockVPC.ListTagResourcesFunc = func(request *vpc.ListTagResourcesRequest) (*vpc.ListTagResourcesResponse, error) {
		resp := vpc.CreateListTagResourcesResponse()
		resp.TagResources.TagResource = []vpc.TagResource{
			{ResourceId: "cbwp-1", TagKey: "CodeName", TagValue: "test-code"},
			{ResourceId: "cbwp-1", TagKey: "Other", TagValue: "ignore"},
		}
		return resp, nil
	}
	tags = c.fetchCBWPTags(config.CloudAccount{}, "cn-hangzhou", []string{"cbwp-1"})
	assert.Equal(t, "test-code", tags["cbwp-1"])

	// Case 3: Error
	mockVPC.ListTagResourcesFunc = func(request *vpc.ListTagResourcesRequest) (*vpc.ListTagResourcesResponse, error) {
		return nil, fmt.Errorf("error")
	}
	tags = c.fetchCBWPTags(config.CloudAccount{}, "cn-hangzhou", []string{"cbwp-1"})
	assert.Empty(t, tags)
}

func TestListSLBIDs(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)

	mockSLB := &mockSLBClient{
		DescribeLoadBalancersFunc: func(request *slb.DescribeLoadBalancersRequest) (*slb.DescribeLoadBalancersResponse, error) {
			resp := slb.CreateDescribeLoadBalancersResponse()
			resp.LoadBalancers.LoadBalancer = []slb.LoadBalancer{
				{LoadBalancerId: "lb-1"},
			}
			return resp, nil
		},
		DescribeLoadBalancerAttributeFunc: func(request *slb.DescribeLoadBalancerAttributeRequest) (*slb.DescribeLoadBalancerAttributeResponse, error) {
			resp := slb.CreateDescribeLoadBalancerAttributeResponse()
			resp.ListenerPortsAndProtocol.ListenerPortAndProtocol = []slb.ListenerPortAndProtocol{
				{ListenerPort: 80, ListenerProtocol: "http"},
			}
			return resp, nil
		},
	}

	c.clientFactory = &mockClientFactory{slb: mockSLB}
	ids, meta := c.listSLBIDs(config.CloudAccount{AccountID: "acc1"}, "cn-hangzhou")
	assert.Len(t, ids, 1)
	assert.Equal(t, "lb-1", ids[0])

	// Check meta
	m, ok := meta["lb-1"].([]map[string]string)
	if assert.True(t, ok) {
		assert.Len(t, m, 1)
		assert.Equal(t, "80", m[0]["port"])
	}
}

func TestListSLBIDs_Pagination(t *testing.T) {
	// Set PageSize to 1 to test pagination loop
	c := NewCollector(&config.Config{
		ServerConf: &config.ServerConf{
			PageSize: 1,
		},
	}, nil)

	callCount := 0
	mockSLB := &mockSLBClient{
		DescribeLoadBalancersFunc: func(request *slb.DescribeLoadBalancersRequest) (*slb.DescribeLoadBalancersResponse, error) {
			callCount++
			resp := slb.CreateDescribeLoadBalancersResponse()
			switch callCount {
			case 1:
				resp.TotalCount = 2
				resp.PageNumber = 1
				resp.PageSize = 1
				resp.LoadBalancers.LoadBalancer = []slb.LoadBalancer{
					{LoadBalancerId: "lb-1"},
				}
			case 2:
				resp.TotalCount = 2
				resp.PageNumber = 2
				resp.LoadBalancers.LoadBalancer = []slb.LoadBalancer{
					{LoadBalancerId: "lb-2"},
				}
			default:
				// Stop the loop - should not reach here if TotalCount logic works
				resp.TotalCount = 2
				resp.PageNumber = 3
				resp.LoadBalancers.LoadBalancer = []slb.LoadBalancer{}
			}
			return resp, nil
		},
		DescribeLoadBalancerAttributeFunc: func(request *slb.DescribeLoadBalancerAttributeRequest) (*slb.DescribeLoadBalancerAttributeResponse, error) {
			resp := slb.CreateDescribeLoadBalancerAttributeResponse()
			resp.ListenerPortsAndProtocol.ListenerPortAndProtocol = []slb.ListenerPortAndProtocol{}
			return resp, nil
		},
	}

	c.clientFactory = &mockClientFactory{slb: mockSLB}
	ids, _ := c.listSLBIDs(config.CloudAccount{AccountID: "acc1"}, "cn-hangzhou")
	assert.Len(t, ids, 2, "Should collect all 2 LBs using TotalCount pagination")
	assert.Contains(t, ids, "lb-1")
	assert.Contains(t, ids, "lb-2")
	assert.Equal(t, 2, callCount, "Should only call API twice (page 1 and page 2)")
}

func TestFetchSLBTags(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)

	mockTag := &mockTagClient{
		ListTagResourcesFunc: func(request *tag.ListTagResourcesRequest) (*tag.ListTagResourcesResponse, error) {
			resp := tag.CreateListTagResourcesResponse()
			// Populate struct directly
			resp.TagResources = []tag.TagResource{
				{
					ResourceId: "lb-1",
					Tags: []tag.Tag{
						{Key: "code_name", Value: "test-name"},
					},
				},
			}
			return resp, nil
		},
	}

	mockSTS := &mockSTSClient{
		GetCallerIdentityFunc: func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
			resp := sts.CreateGetCallerIdentityResponse()
			resp.AccountId = "123"
			return resp, nil
		},
	}

	c.clientFactory = &mockClientFactory{
		tag: mockTag,
		sts: mockSTS,
	}

	tags := c.fetchSLBTags(config.CloudAccount{AccountID: "acc1"}, "cn-hangzhou", "ns", "metric", []string{"lb-1"})
	assert.Equal(t, "test-name", tags["lb-1"])
}

func TestParseSLBTagsContent(t *testing.T) {
	content := `{"TagResources":{"TagResource":[{"ResourceId":"lb-1","TagKey":"code_name","TagValue":"v"}]}}`
	tags := parseSLBTagsContent([]byte(content))
	assert.Equal(t, "v", tags["lb-1"])
}

func TestBuildMetricDimensions(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)

	// Case 1: Simple
	dims, dyn := c.buildMetricDimensions([]string{"id1"}, "instanceId", []string{"instanceId", "region"}, nil)
	assert.Len(t, dims, 1)
	assert.Equal(t, "id1", dims[0]["instanceId"])
	// "region" is treated as a reserved label (not a CMS dimension key) in buildMetricDimensions.
	assert.Len(t, dyn, 0)

	// Case 2: With Meta (Sub-resources)
	meta := map[string]interface{}{
		"id1": []map[string]string{
			{"mountPath": "/data1"},
			{"mountPath": "/data2"},
		},
	}
	dims, dyn = c.buildMetricDimensions([]string{"id1", "id2"}, "instanceId", []string{"instanceId", "mountPath"}, meta)

	// id1 should generate 2 entries (/data1, /data2)
	// id2 should generate 1 entry (fallback)
	assert.Len(t, dims, 3)
	assert.Contains(t, dyn, "mountPath")
}

func TestCheckRequiredDimensions(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	assert.True(t, c.checkRequiredDimensions("ns", []string{"instanceId"}))
	// Test default mapping logic (hasAnyDim check)
	// Since "ns" is not in default map, it falls through to instanceId check
	assert.True(t, c.checkRequiredDimensions("ns", []string{"instanceId", "other"}))
	assert.False(t, c.checkRequiredDimensions("ns", []string{"other"}))
}

func TestProcessMetricBatch(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", "test-acc", "region", "cn-hangzhou")

	mockCMS := &mockCMSClient{}

	dims := []map[string]string{{"instanceId": "inst-1"}}
	req := cms.CreateDescribeMetricLastRequest()

	// Case 1: Client error
	mockCMS.DescribeMetricLastFunc = func(request *cms.DescribeMetricLastRequest) (*cms.DescribeMetricLastResponse, error) {
		return nil, fmt.Errorf("cms error")
	}
	c.processMetricBatch(mockCMS, req, dims, config.CloudAccount{AccountID: "test-acc"}, "cn-hangzhou", "acs_ecs_dashboard", "CPU", "instanceId", "ecs", nil, nil, nil, ctxLog)

	// Case 2: Success with data
	mockCMS.DescribeMetricLastFunc = func(request *cms.DescribeMetricLastRequest) (*cms.DescribeMetricLastResponse, error) {
		resp := cms.CreateDescribeMetricLastResponse()
		points := []map[string]interface{}{
			{"instanceId": "inst-1", "Average": 50.5},
		}
		data, _ := json.Marshal(points)
		resp.Datapoints = string(data)
		return resp, nil
	}
	c.processMetricBatch(mockCMS, req, dims, config.CloudAccount{AccountID: "test-acc"}, "cn-hangzhou", "acs_ecs_dashboard", "CPU", "instanceId", "ecs", nil, nil, []string{"Average"}, ctxLog)

	// Case 3: JSON error
	mockCMS.DescribeMetricLastFunc = func(request *cms.DescribeMetricLastRequest) (*cms.DescribeMetricLastResponse, error) {
		resp := cms.CreateDescribeMetricLastResponse()
		resp.Datapoints = "invalid-json"
		return resp, nil
	}
	c.processMetricBatch(mockCMS, req, dims, config.CloudAccount{AccountID: "test-acc"}, "cn-hangzhou", "acs_ecs_dashboard", "CPU", "instanceId", "ecs", nil, nil, []string{"Average"}, ctxLog)
}

func TestChooseStatistics(t *testing.T) {
	available := []string{"Average", "Maximum", "Minimum"}

	// Case 1: Empty desired
	assert.Equal(t, available, chooseStatistics(available, nil))

	// Case 2: Match
	assert.Equal(t, []string{"Average"}, chooseStatistics(available, []string{"Average"}))

	// Case 3: Partial match
	assert.Equal(t, []string{"Average"}, chooseStatistics(available, []string{"Average", "Unknown"}))

	// Case 4: No match (fallback)
	assert.Equal(t, available, chooseStatistics(available, []string{"Unknown"}))
}

func TestGetAccountUID(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	mockSTS := &mockSTSClient{}
	c.clientFactory = &mockClientFactory{sts: mockSTS}
	acc := config.CloudAccount{AccountID: "acc1", AccessKeyID: "ak1"}

	// Case 1: STS Success
	mockSTS.GetCallerIdentityFunc = func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
		resp := sts.CreateGetCallerIdentityResponse()
		resp.AccountId = "uid-123"
		return resp, nil
	}
	uid := c.getAccountUID(acc, "cn-hangzhou")
	assert.Equal(t, "uid-123", uid)

	// Case 2: Cache Hit
	// Reset STS mock to fail, but it should return cached value
	mockSTS.GetCallerIdentityFunc = func(request *sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error) {
		return nil, fmt.Errorf("error")
	}
	uid = c.getAccountUID(acc, "cn-hangzhou")
	assert.Equal(t, "uid-123", uid)

	// Case 3: STS Error (new account)
	acc2 := config.CloudAccount{AccountID: "acc2", AccessKeyID: "ak2"}
	uid = c.getAccountUID(acc2, "cn-hangzhou")
	assert.Equal(t, "acc2", uid) // Fallback to config AccountID
}

func TestGetAllRegions(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	mockECS := &mockECSClient{}
	c.clientFactory = &mockClientFactory{ecs: mockECS}
	acc := config.CloudAccount{AccountID: "acc1"}

	// Case 1: Success
	mockECS.DescribeRegionsFunc = func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
		resp := ecs.CreateDescribeRegionsResponse()
		resp.Regions.Region = []ecs.Region{
			{RegionId: "cn-hangzhou"},
			{RegionId: "cn-beijing"},
		}
		return resp, nil
	}
	regions := c.getAllRegions(acc)
	assert.Len(t, regions, 2)
	assert.Contains(t, regions, "cn-hangzhou")

	// Case 2: Error (fallback)
	mockECS.DescribeRegionsFunc = func(request *ecs.DescribeRegionsRequest) (*ecs.DescribeRegionsResponse, error) {
		return nil, fmt.Errorf("error")
	}
	regions = c.getAllRegions(acc)
	assert.Len(t, regions, 1)
	assert.Equal(t, "cn-hangzhou", regions[0])

	// Case 3: Error with Env
	if err := os.Setenv("DEFAULT_REGIONS", "cn-shanghai, cn-shenzhen"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("DEFAULT_REGIONS") }()
	regions = c.getAllRegions(acc)
	assert.Len(t, regions, 2)
	assert.Contains(t, regions, "cn-shanghai")
}
