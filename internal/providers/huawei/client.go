// 华为云客户端工厂：提供 ELB、OBS、CES 客户端创建
package huawei

import (
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	cesregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/region"
	elb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	elbregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/region"
)

// ELBClient 定义 ELB 客户端接口
type ELBClient interface {
	ListLoadBalancers(request *elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error)
}

// CESClient 定义 CES 监控客户端接口
type CESClient interface {
	BatchListMetricData(request *cesmodel.BatchListMetricDataRequest) (*cesmodel.BatchListMetricDataResponse, error)
	ListMetrics(request *cesmodel.ListMetricsRequest) (*cesmodel.ListMetricsResponse, error)
}

// OBSClient 定义 OBS 存储客户端接口
type OBSClient interface {
	ListBuckets(input *obs.ListBucketsInput) (output *obs.ListBucketsOutput, err error)
	Close()
}

// obsClientWrapper 包装 obs.ObsClient 以适配 OBSClient 接口
type obsClientWrapper struct {
	client *obs.ObsClient
}

func (w *obsClientWrapper) ListBuckets(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
	return w.client.ListBuckets(input)
}

func (w *obsClientWrapper) Close() {
	w.client.Close()
}

// ClientFactory 定义客户端工厂接口
type ClientFactory interface {
	NewELBClient(region, ak, sk string) (ELBClient, error)
	NewCESClient(region, ak, sk string) (CESClient, error)
	NewOBSClient(region, ak, sk string) (OBSClient, error)
}

type defaultClientFactory struct{}

// NewELBClient 创建 ELB 客户端
func (f *defaultClientFactory) NewELBClient(region, ak, sk string) (ELBClient, error) {
	auth, err := basic.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	reg, err := elbregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := elb.ElbClientBuilder().
		WithRegion(reg).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	return elb.NewElbClient(hcClient), nil
}

// NewCESClient 创建 CES 监控客户端
func (f *defaultClientFactory) NewCESClient(region, ak, sk string) (CESClient, error) {
	auth, err := basic.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	reg, err := cesregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := ces.CesClientBuilder().
		WithRegion(reg).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	return ces.NewCesClient(hcClient), nil
}

// NewOBSClient 创建 OBS 存储客户端
func (f *defaultClientFactory) NewOBSClient(region, ak, sk string) (OBSClient, error) {
	endpoint := "https://obs." + region + ".myhuaweicloud.com"
	client, err := obs.New(ak, sk, endpoint)
	if err != nil {
		return nil, err
	}
	return &obsClientWrapper{client: client}, nil
}
