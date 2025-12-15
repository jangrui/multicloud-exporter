package tencent

import (
	"context"
	"net/http"
	"net/url"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"github.com/tencentyun/cos-go-sdk-v5"
)

type CVMClient interface {
	DescribeRegions(request *cvm.DescribeRegionsRequest) (response *cvm.DescribeRegionsResponse, err error)
}

type CLBClient interface {
	DescribeLoadBalancers(request *clb.DescribeLoadBalancersRequest) (response *clb.DescribeLoadBalancersResponse, err error)
}

type VPCClient interface {
	DescribeBandwidthPackages(request *vpc.DescribeBandwidthPackagesRequest) (response *vpc.DescribeBandwidthPackagesResponse, err error)
}

type MonitorClient interface {
	GetMonitorData(request *monitor.GetMonitorDataRequest) (response *monitor.GetMonitorDataResponse, err error)
}

type COSClient interface {
	GetService(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error)
	GetBucketTagging(ctx context.Context, bucket string, region string) (map[string]string, error)
}

type ClientFactory interface {
	NewCVMClient(region, ak, sk string) (CVMClient, error)
	NewCLBClient(region, ak, sk string) (CLBClient, error)
	NewVPCClient(region, ak, sk string) (VPCClient, error)
	NewMonitorClient(region, ak, sk string) (MonitorClient, error)
	NewCOSClient(region, ak, sk string) (COSClient, error)
}

type defaultClientFactory struct{}

func (f *defaultClientFactory) NewCVMClient(region, ak, sk string) (CVMClient, error) {
	credential := common.NewCredential(ak, sk)
	return cvm.NewClient(credential, region, profile.NewClientProfile())
}

func (f *defaultClientFactory) NewCLBClient(region, ak, sk string) (CLBClient, error) {
	credential := common.NewCredential(ak, sk)
	return clb.NewClient(credential, region, profile.NewClientProfile())
}

func (f *defaultClientFactory) NewVPCClient(region, ak, sk string) (VPCClient, error) {
	credential := common.NewCredential(ak, sk)
	return vpc.NewClient(credential, region, profile.NewClientProfile())
}

func (f *defaultClientFactory) NewMonitorClient(region, ak, sk string) (MonitorClient, error) {
	credential := common.NewCredential(ak, sk)
	return monitor.NewClient(credential, region, profile.NewClientProfile())
}

type defaultCOSClient struct {
	client *cos.Client
	ak     string
	sk     string
	token  string
}

func (c *defaultCOSClient) GetService(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
	return c.client.Service.Get(ctx)
}

func (c *defaultCOSClient) GetBucketTagging(ctx context.Context, bucket string, region string) (map[string]string, error) {
	u, _ := cos.NewBucketURL(bucket, region, true)
	b := &cos.BaseURL{BucketURL: u}
	httpClient := &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:     c.ak,
			SecretKey:    c.sk,
			SessionToken: c.token,
		},
	}
	bc := cos.NewClient(b, httpClient)
	res, _, err := bc.Bucket.GetTagging(ctx)
	if err != nil || res == nil {
		return map[string]string{}, err
	}
	out := make(map[string]string)
	for _, t := range res.TagSet {
		if t.Key != "" {
			out[t.Key] = t.Value
		}
	}
	return out, nil
}

func (f *defaultClientFactory) NewCOSClient(region, ak, sk string) (COSClient, error) {
	u, _ := url.Parse("https://cos." + region + ".myqcloud.com")
	b := &cos.BaseURL{BucketURL: u}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  ak,
			SecretKey: sk,
		},
	})
	return &defaultCOSClient{client: c, ak: ak, sk: sk}, nil
}
