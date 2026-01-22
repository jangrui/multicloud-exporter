package huawei

import (
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
)

// setClientFactoryForTesting 设置客户端工厂（仅用于测试）
func (h *Collector) setClientFactoryForTesting(factory ClientFactory) {
	h.clientFactory = factory
}

// mockELBClient ELB 客户端 mock 实现
type mockELBClient struct {
	listLoadBalancersFunc func(*elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error)
}

func (m *mockELBClient) ListLoadBalancers(request *elbmodel.ListLoadBalancersRequest) (*elbmodel.ListLoadBalancersResponse, error) {
	if m.listLoadBalancersFunc != nil {
		return m.listLoadBalancersFunc(request)
	}
	return nil, nil
}

// mockCESClient CES 客户端 mock 实现
type mockCESClient struct {
	batchListMetricDataFunc func(*cesmodel.BatchListMetricDataRequest) (*cesmodel.BatchListMetricDataResponse, error)
	listMetricsFunc         func(*cesmodel.ListMetricsRequest) (*cesmodel.ListMetricsResponse, error)
}

func (m *mockCESClient) BatchListMetricData(request *cesmodel.BatchListMetricDataRequest) (*cesmodel.BatchListMetricDataResponse, error) {
	if m.batchListMetricDataFunc != nil {
		return m.batchListMetricDataFunc(request)
	}
	return nil, nil
}

func (m *mockCESClient) ListMetrics(request *cesmodel.ListMetricsRequest) (*cesmodel.ListMetricsResponse, error) {
	if m.listMetricsFunc != nil {
		return m.listMetricsFunc(request)
	}
	return nil, nil
}

// mockOBSClient OBS 客户端 mock 实现
type mockOBSClient struct {
	listBucketsFunc func(*obs.ListBucketsInput) (*obs.ListBucketsOutput, error)
	closeFunc       func()
}

func (m *mockOBSClient) ListBuckets(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
	if m.listBucketsFunc != nil {
		return m.listBucketsFunc(input)
	}
	return nil, nil
}

func (m *mockOBSClient) Close() {
	if m.closeFunc != nil {
		m.closeFunc()
	}
}

// mockClientFactory 客户端工厂 mock 实现
type mockClientFactory struct {
	newELBClientFunc func(region, ak, sk string) (ELBClient, error)
	newCESClientFunc func(region, ak, sk string) (CESClient, error)
	newOBSClientFunc func(region, ak, sk string) (OBSClient, error)
}

func (f *mockClientFactory) NewELBClient(region, ak, sk string) (ELBClient, error) {
	if f.newELBClientFunc != nil {
		return f.newELBClientFunc(region, ak, sk)
	}
	return &mockELBClient{}, nil
}

func (f *mockClientFactory) NewCESClient(region, ak, sk string) (CESClient, error) {
	if f.newCESClientFunc != nil {
		return f.newCESClientFunc(region, ak, sk)
	}
	return &mockCESClient{}, nil
}

func (f *mockClientFactory) NewOBSClient(region, ak, sk string) (OBSClient, error) {
	if f.newOBSClientFunc != nil {
		return f.newOBSClientFunc(region, ak, sk)
	}
	return &mockOBSClient{}, nil
}
