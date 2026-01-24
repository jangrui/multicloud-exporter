package aliyun

import (
	"time"

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

// ECSClient 用于 Mock 测试的 ECS 客户端接口
type ECSClient interface {
	DescribeRegions(request *ecs.DescribeRegionsRequest) (response *ecs.DescribeRegionsResponse, err error)
}

// ALBClient 用于 Mock 测试的 ALB 客户端接口
type ALBClient interface {
	ListLoadBalancers(request *alb20200616.ListLoadBalancersRequest) (response *alb20200616.ListLoadBalancersResponse, err error)
}

// NLBClient 用于 Mock 测试的 NLB 客户端接口
type NLBClient interface {
	ListLoadBalancers(request *nlb20220430.ListLoadBalancersRequest) (response *nlb20220430.ListLoadBalancersResponse, err error)
}

// SLBClient 用于 Mock 测试的 SLB 客户端接口
type SLBClient interface {
	DescribeLoadBalancers(request *slb.DescribeLoadBalancersRequest) (response *slb.DescribeLoadBalancersResponse, err error)
	DescribeLoadBalancerAttribute(request *slb.DescribeLoadBalancerAttributeRequest) (response *slb.DescribeLoadBalancerAttributeResponse, err error)
}

// VPCClient 用于 Mock 测试的 VPC 客户端接口
type VPCClient interface {
	DescribeCommonBandwidthPackages(request *vpc.DescribeCommonBandwidthPackagesRequest) (response *vpc.DescribeCommonBandwidthPackagesResponse, err error)
	ListTagResources(request *vpc.ListTagResourcesRequest) (response *vpc.ListTagResourcesResponse, err error)
}

// TagClient 用于 Mock 测试的标签客户端接口
type TagClient interface {
	ListTagResources(request *tag.ListTagResourcesRequest) (response *tag.ListTagResourcesResponse, err error)
}

// CMSClient 用于 Mock 测试的 CMS 客户端接口
type CMSClient interface {
	DescribeMetricMetaList(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error)
	DescribeMetricList(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error)
	DescribeMetricLast(request *cms.DescribeMetricLastRequest) (response *cms.DescribeMetricLastResponse, err error)
}

// STSClient 用于 Mock 测试的 STS 客户端接口
type STSClient interface {
	GetCallerIdentity(request *sts.GetCallerIdentityRequest) (response *sts.GetCallerIdentityResponse, err error)
}

// OSSClient 用于 Mock 测试的 OSS 客户端接口
type OSSClient interface {
	ListBuckets(options ...oss.Option) (oss.ListBucketsResult, error)
	GetBucketTagging(bucketName string, options ...oss.Option) (oss.GetBucketTaggingResult, error)
}

// ClientFactory 创建客户端的工厂接口
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

// defaultClientFactory 使用真实 SDK 实现 ClientFactory
type defaultClientFactory struct{}

func (f *defaultClientFactory) NewECSClient(region, ak, sk string) (ECSClient, error) {
	client, err := ecs.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewALBClient(region, ak, sk string) (ALBClient, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("alb." + region + ".aliyuncs.com"),
		ConnectTimeout:  tea.Int(10000), // 10s
		ReadTimeout:     tea.Int(30000), // 30s
	}
	return alb20200616.NewClient(cfg)
}

func (f *defaultClientFactory) NewNLBClient(region, ak, sk string) (NLBClient, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("nlb." + region + ".aliyuncs.com"),
		ConnectTimeout:  tea.Int(10000), // 10s
		ReadTimeout:     tea.Int(30000), // 30s
	}
	return nlb20220430.NewClient(cfg)
}

func (f *defaultClientFactory) NewSLBClient(region, ak, sk string) (SLBClient, error) {
	client, err := slb.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewVPCClient(region, ak, sk string) (VPCClient, error) {
	client, err := vpc.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewTagClient(region, ak, sk string) (TagClient, error) {
	client, err := tag.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewCMSClient(region, ak, sk string) (CMSClient, error) {
	client, err := cms.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewSTSClient(region, ak, sk string) (STSClient, error) {
	client, err := sts.NewClientWithAccessKey(region, ak, sk)
	if err == nil {
		client.GetConfig().WithScheme("HTTPS")
		client.SetConnectTimeout(10 * time.Second)
		client.SetReadTimeout(30 * time.Second)
	}
	return client, err
}

func (f *defaultClientFactory) NewOSSClient(region, ak, sk string) (OSSClient, error) {
	endpoint := "https://oss-" + region + ".aliyuncs.com"
	return oss.New(endpoint, ak, sk, oss.Timeout(10, 30))
}
