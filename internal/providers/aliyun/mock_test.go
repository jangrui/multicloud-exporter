package aliyun

import (
	"fmt"

	alb20200616 "github.com/alibabacloud-go/alb-20200616/v2/client"
	nlb20220430 "github.com/alibabacloud-go/nlb-20220430/v4/client"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type mockClientFactory struct {
	ecs *mockECSClient
	cms *mockCMSClient
	sts *mockSTSClient
	alb *mockALBClient
	nlb *mockNLBClient
	slb *mockSLBClient
	vpc *mockVPCClient
	tag *mockTagClient
	oss *mockOSSClient
}

func (f *mockClientFactory) NewECSClient(region, ak, sk string) (ECSClient, error) {
	if f.ecs == nil {
		return nil, fmt.Errorf("mock ecs client not initialized")
	}
	return f.ecs, nil
}

func (f *mockClientFactory) NewCMSClient(region, ak, sk string) (CMSClient, error) {
	if f.cms == nil {
		return nil, fmt.Errorf("mock cms client not initialized")
	}
	return f.cms, nil
}

func (f *mockClientFactory) NewSTSClient(region, ak, sk string) (STSClient, error) {
	if f.sts == nil {
		return nil, fmt.Errorf("mock sts client not initialized")
	}
	return f.sts, nil
}

func (f *mockClientFactory) NewALBClient(region, ak, sk string) (ALBClient, error) {
	if f.alb == nil {
		return nil, fmt.Errorf("mock alb client not initialized")
	}
	return f.alb, nil
}

func (f *mockClientFactory) NewNLBClient(region, ak, sk string) (NLBClient, error) {
	if f.nlb == nil {
		return nil, fmt.Errorf("mock nlb client not initialized")
	}
	return f.nlb, nil
}

func (f *mockClientFactory) NewSLBClient(region, ak, sk string) (SLBClient, error) {
	if f.slb == nil {
		return nil, fmt.Errorf("mock slb client not initialized")
	}
	return f.slb, nil
}

func (f *mockClientFactory) NewVPCClient(region, ak, sk string) (VPCClient, error) {
	if f.vpc == nil {
		return nil, fmt.Errorf("mock vpc client not initialized")
	}
	return f.vpc, nil
}

func (f *mockClientFactory) NewTagClient(region, ak, sk string) (TagClient, error) {
	if f.tag == nil {
		return nil, fmt.Errorf("mock tag client not initialized")
	}
	return f.tag, nil
}

func (f *mockClientFactory) NewOSSClient(region, ak, sk string) (OSSClient, error) {
	if f.oss == nil {
		return nil, fmt.Errorf("mock oss client not initialized")
	}
	return f.oss, nil
}

type mockECSClient struct {
	DescribeRegionsFunc func(request *ecs.DescribeRegionsRequest) (response *ecs.DescribeRegionsResponse, err error)
}

func (m *mockECSClient) DescribeRegions(request *ecs.DescribeRegionsRequest) (response *ecs.DescribeRegionsResponse, err error) {
	if m.DescribeRegionsFunc != nil {
		return m.DescribeRegionsFunc(request)
	}
	return &ecs.DescribeRegionsResponse{}, nil
}

type mockCMSClient struct {
	DescribeMetricMetaListFunc func(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error)
	DescribeMetricListFunc     func(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error)
	DescribeMetricLastFunc     func(request *cms.DescribeMetricLastRequest) (response *cms.DescribeMetricLastResponse, err error)
}

func (m *mockCMSClient) DescribeMetricMetaList(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error) {
	if m.DescribeMetricMetaListFunc != nil {
		return m.DescribeMetricMetaListFunc(request)
	}
	return &cms.DescribeMetricMetaListResponse{}, nil
}

func (m *mockCMSClient) DescribeMetricList(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error) {
	if m.DescribeMetricListFunc != nil {
		return m.DescribeMetricListFunc(request)
	}
	return &cms.DescribeMetricListResponse{}, nil
}

func (m *mockCMSClient) DescribeMetricLast(request *cms.DescribeMetricLastRequest) (response *cms.DescribeMetricLastResponse, err error) {
	if m.DescribeMetricLastFunc != nil {
		return m.DescribeMetricLastFunc(request)
	}
	return &cms.DescribeMetricLastResponse{}, nil
}

type mockSTSClient struct {
	GetCallerIdentityFunc func(request *sts.GetCallerIdentityRequest) (response *sts.GetCallerIdentityResponse, err error)
}

func (m *mockSTSClient) GetCallerIdentity(request *sts.GetCallerIdentityRequest) (response *sts.GetCallerIdentityResponse, err error) {
	if m.GetCallerIdentityFunc != nil {
		return m.GetCallerIdentityFunc(request)
	}
	return &sts.GetCallerIdentityResponse{}, nil
}

type mockSLBClient struct {
	DescribeLoadBalancersFunc         func(request *slb.DescribeLoadBalancersRequest) (response *slb.DescribeLoadBalancersResponse, err error)
	DescribeLoadBalancerAttributeFunc func(request *slb.DescribeLoadBalancerAttributeRequest) (response *slb.DescribeLoadBalancerAttributeResponse, err error)
}

func (m *mockSLBClient) DescribeLoadBalancers(request *slb.DescribeLoadBalancersRequest) (response *slb.DescribeLoadBalancersResponse, err error) {
	if m.DescribeLoadBalancersFunc != nil {
		return m.DescribeLoadBalancersFunc(request)
	}
	return &slb.DescribeLoadBalancersResponse{}, nil
}

func (m *mockSLBClient) DescribeLoadBalancerAttribute(request *slb.DescribeLoadBalancerAttributeRequest) (response *slb.DescribeLoadBalancerAttributeResponse, err error) {
	if m.DescribeLoadBalancerAttributeFunc != nil {
		return m.DescribeLoadBalancerAttributeFunc(request)
	}
	return &slb.DescribeLoadBalancerAttributeResponse{}, nil
}

type mockVPCClient struct {
	DescribeCommonBandwidthPackagesFunc func(request *vpc.DescribeCommonBandwidthPackagesRequest) (response *vpc.DescribeCommonBandwidthPackagesResponse, err error)
	ListTagResourcesFunc                func(request *vpc.ListTagResourcesRequest) (response *vpc.ListTagResourcesResponse, err error)
}

func (m *mockVPCClient) DescribeCommonBandwidthPackages(request *vpc.DescribeCommonBandwidthPackagesRequest) (response *vpc.DescribeCommonBandwidthPackagesResponse, err error) {
	if m.DescribeCommonBandwidthPackagesFunc != nil {
		return m.DescribeCommonBandwidthPackagesFunc(request)
	}
	return &vpc.DescribeCommonBandwidthPackagesResponse{}, nil
}

func (m *mockVPCClient) ListTagResources(request *vpc.ListTagResourcesRequest) (response *vpc.ListTagResourcesResponse, err error) {
	if m.ListTagResourcesFunc != nil {
		return m.ListTagResourcesFunc(request)
	}
	return &vpc.ListTagResourcesResponse{}, nil
}

type mockTagClient struct {
	ListTagResourcesFunc func(request *tag.ListTagResourcesRequest) (response *tag.ListTagResourcesResponse, err error)
}

func (m *mockTagClient) ListTagResources(request *tag.ListTagResourcesRequest) (response *tag.ListTagResourcesResponse, err error) {
	if m.ListTagResourcesFunc != nil {
		return m.ListTagResourcesFunc(request)
	}
	return &tag.ListTagResourcesResponse{}, nil
}

type mockOSSClient struct {
	ListBucketsFunc      func(options ...oss.Option) (oss.ListBucketsResult, error)
	GetBucketTaggingFunc func(bucketName string, options ...oss.Option) (oss.GetBucketTaggingResult, error)
}

func (m *mockOSSClient) ListBuckets(options ...oss.Option) (oss.ListBucketsResult, error) {
	if m.ListBucketsFunc != nil {
		return m.ListBucketsFunc(options...)
	}
	return oss.ListBucketsResult{}, nil
}

func (m *mockOSSClient) GetBucketTagging(bucketName string, options ...oss.Option) (oss.GetBucketTaggingResult, error) {
	if m.GetBucketTaggingFunc != nil {
		return m.GetBucketTaggingFunc(bucketName, options...)
	}
	return oss.GetBucketTaggingResult{}, nil
}

type mockALBClient struct {
	ListLoadBalancersFunc func(request *alb20200616.ListLoadBalancersRequest) (response *alb20200616.ListLoadBalancersResponse, err error)
}

func (m *mockALBClient) ListLoadBalancers(request *alb20200616.ListLoadBalancersRequest) (response *alb20200616.ListLoadBalancersResponse, err error) {
	if m.ListLoadBalancersFunc != nil {
		return m.ListLoadBalancersFunc(request)
	}
	return &alb20200616.ListLoadBalancersResponse{}, nil
}

type mockNLBClient struct {
	ListLoadBalancersFunc func(request *nlb20220430.ListLoadBalancersRequest) (response *nlb20220430.ListLoadBalancersResponse, err error)
}

func (m *mockNLBClient) ListLoadBalancers(request *nlb20220430.ListLoadBalancersRequest) (response *nlb20220430.ListLoadBalancersResponse, err error) {
	if m.ListLoadBalancersFunc != nil {
		return m.ListLoadBalancersFunc(request)
	}
	return &nlb20220430.ListLoadBalancersResponse{}, nil
}
