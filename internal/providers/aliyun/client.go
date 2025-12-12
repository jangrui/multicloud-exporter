package aliyun

import (
	alb20200616 "github.com/alibabacloud-go/alb-20200616/v2/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	nlb20220430 "github.com/alibabacloud-go/nlb-20220430/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ECSClient interface for mocking
type ECSClient interface {
	DescribeRegions(request *ecs.DescribeRegionsRequest) (response *ecs.DescribeRegionsResponse, err error)
}

// ALBClient interface for mocking
type ALBClient interface {
	ListLoadBalancers(request *alb20200616.ListLoadBalancersRequest) (response *alb20200616.ListLoadBalancersResponse, err error)
}

// NLBClient interface for mocking
type NLBClient interface {
	ListLoadBalancers(request *nlb20220430.ListLoadBalancersRequest) (response *nlb20220430.ListLoadBalancersResponse, err error)
}

// SLBClient interface for mocking
type SLBClient interface {
	DescribeLoadBalancers(request *slb.DescribeLoadBalancersRequest) (response *slb.DescribeLoadBalancersResponse, err error)
	DescribeLoadBalancerAttribute(request *slb.DescribeLoadBalancerAttributeRequest) (response *slb.DescribeLoadBalancerAttributeResponse, err error)
}

// VPCClient interface for mocking
type VPCClient interface {
	DescribeCommonBandwidthPackages(request *vpc.DescribeCommonBandwidthPackagesRequest) (response *vpc.DescribeCommonBandwidthPackagesResponse, err error)
	ListTagResources(request *vpc.ListTagResourcesRequest) (response *vpc.ListTagResourcesResponse, err error)
}

// TagClient interface for mocking
type TagClient interface {
	ListTagResources(request *tag.ListTagResourcesRequest) (response *tag.ListTagResourcesResponse, err error)
}

// CMSClient interface for mocking
type CMSClient interface {
	DescribeMetricMetaList(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error)
	DescribeMetricList(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error)
	DescribeMetricLast(request *cms.DescribeMetricLastRequest) (response *cms.DescribeMetricLastResponse, err error)
}

// STSClient interface for mocking
type STSClient interface {
	GetCallerIdentity(request *sts.GetCallerIdentityRequest) (response *sts.GetCallerIdentityResponse, err error)
}

// OSSClient interface for mocking
type OSSClient interface {
	ListBuckets(options ...oss.Option) (oss.ListBucketsResult, error)
}

// ClientFactory interface for creating clients
type ClientFactory interface {
	NewECSClient(region, ak, sk string) (ECSClient, error)
	NewCMSClient(region, ak, sk string) (CMSClient, error)
	NewSTSClient(region, ak, sk string) (STSClient, error)
	NewALBClient(region, ak, sk string) (ALBClient, error)
	NewNLBClient(region, ak, sk string) (NLBClient, error)
	NewSLBClient(region, ak, sk string) (SLBClient, error)
	NewVPCClient(region, ak, sk string) (VPCClient, error)
	NewTagClient(region, ak, sk string) (TagClient, error)
	NewOSSClient(region, ak, sk string) (OSSClient, error)
}

// defaultClientFactory implements ClientFactory using real SDK
type defaultClientFactory struct{}

func (f *defaultClientFactory) NewECSClient(region, ak, sk string) (ECSClient, error) {
	return ecs.NewClientWithAccessKey(region, ak, sk)
}

func (f *defaultClientFactory) NewALBClient(region, ak, sk string) (ALBClient, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("alb." + region + ".aliyuncs.com"),
	}
	return alb20200616.NewClient(cfg)
}

func (f *defaultClientFactory) NewNLBClient(region, ak, sk string) (NLBClient, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("nlb." + region + ".aliyuncs.com"),
	}
	return nlb20220430.NewClient(cfg)
}

func (f *defaultClientFactory) NewSLBClient(region, ak, sk string) (SLBClient, error) {
	return slb.NewClientWithAccessKey(region, ak, sk)
}

func (f *defaultClientFactory) NewVPCClient(region, ak, sk string) (VPCClient, error) {
	return vpc.NewClientWithAccessKey(region, ak, sk)
}

func (f *defaultClientFactory) NewTagClient(region, ak, sk string) (TagClient, error) {
	return tag.NewClientWithAccessKey(region, ak, sk)
}

func (f *defaultClientFactory) NewCMSClient(region, ak, sk string) (CMSClient, error) {
	return cms.NewClientWithAccessKey(region, ak, sk)
}

func (f *defaultClientFactory) NewSTSClient(region, ak, sk string) (STSClient, error) {
	client, err := sts.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.GetConfig().WithScheme("HTTPS")
	}
	return client, err
}

func (f *defaultClientFactory) NewOSSClient(region, ak, sk string) (OSSClient, error) {
	endpoint := "https://oss-" + region + ".aliyuncs.com"
	return oss.New(endpoint, ak, sk)
}
