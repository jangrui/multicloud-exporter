package tencent

import (
	"context"
	"fmt"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"github.com/tencentyun/cos-go-sdk-v5"
)

type mockClientFactory struct {
	cvm     *mockCVMClient
	clb     *mockCLBClient
	vpc     *mockVPCClient
	monitor *mockMonitorClient
	cos     *mockCOSClient
}

func (f *mockClientFactory) NewCVMClient(region, ak, sk string) (CVMClient, error) {
	if f.cvm == nil {
		return nil, fmt.Errorf("mock cvm client not initialized")
	}
	return f.cvm, nil
}

func (f *mockClientFactory) NewCLBClient(region, ak, sk string) (CLBClient, error) {
	if f.clb == nil {
		return nil, fmt.Errorf("mock clb client not initialized")
	}
	return f.clb, nil
}

func (f *mockClientFactory) NewVPCClient(region, ak, sk string) (VPCClient, error) {
	if f.vpc == nil {
		return nil, fmt.Errorf("mock vpc client not initialized")
	}
	return f.vpc, nil
}

func (f *mockClientFactory) NewMonitorClient(region, ak, sk string) (MonitorClient, error) {
	if f.monitor == nil {
		return nil, fmt.Errorf("mock monitor client not initialized")
	}
	return f.monitor, nil
}

func (f *mockClientFactory) NewCOSClient(region, ak, sk string) (COSClient, error) {
	if f.cos == nil {
		return nil, fmt.Errorf("mock cos client not initialized")
	}
	return f.cos, nil
}

type mockCVMClient struct {
	DescribeRegionsFunc func(request *cvm.DescribeRegionsRequest) (response *cvm.DescribeRegionsResponse, err error)
}

func (m *mockCVMClient) DescribeRegions(request *cvm.DescribeRegionsRequest) (response *cvm.DescribeRegionsResponse, err error) {
	if m.DescribeRegionsFunc != nil {
		return m.DescribeRegionsFunc(request)
	}
	return &cvm.DescribeRegionsResponse{}, nil
}

type mockCLBClient struct {
	DescribeLoadBalancersFunc func(request *clb.DescribeLoadBalancersRequest) (response *clb.DescribeLoadBalancersResponse, err error)
}

func (m *mockCLBClient) DescribeLoadBalancers(request *clb.DescribeLoadBalancersRequest) (response *clb.DescribeLoadBalancersResponse, err error) {
	if m.DescribeLoadBalancersFunc != nil {
		return m.DescribeLoadBalancersFunc(request)
	}
	return &clb.DescribeLoadBalancersResponse{}, nil
}

type mockVPCClient struct {
	DescribeBandwidthPackagesFunc func(request *vpc.DescribeBandwidthPackagesRequest) (response *vpc.DescribeBandwidthPackagesResponse, err error)
}

func (m *mockVPCClient) DescribeBandwidthPackages(request *vpc.DescribeBandwidthPackagesRequest) (response *vpc.DescribeBandwidthPackagesResponse, err error) {
	if m.DescribeBandwidthPackagesFunc != nil {
		return m.DescribeBandwidthPackagesFunc(request)
	}
	return &vpc.DescribeBandwidthPackagesResponse{}, nil
}

type mockMonitorClient struct {
	GetMonitorDataFunc func(request *monitor.GetMonitorDataRequest) (response *monitor.GetMonitorDataResponse, err error)
}

func (m *mockMonitorClient) GetMonitorData(request *monitor.GetMonitorDataRequest) (response *monitor.GetMonitorDataResponse, err error) {
	if m.GetMonitorDataFunc != nil {
		return m.GetMonitorDataFunc(request)
	}
	return &monitor.GetMonitorDataResponse{}, nil
}

type mockCOSClient struct {
	GetServiceFunc func(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error)
}

func (m *mockCOSClient) GetService(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
	if m.GetServiceFunc != nil {
		return m.GetServiceFunc(ctx)
	}
	return &cos.ServiceGetResult{}, nil, nil
}
