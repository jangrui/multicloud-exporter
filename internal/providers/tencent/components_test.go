package tencent

import (
	"context"
	"fmt"
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"github.com/tencentyun/cos-go-sdk-v5"
)

func init() {
	// logger.Init is handled in other tests or init
}

func TestListCLBVips(t *testing.T) {
	mockCLB := &mockCLBClient{
		DescribeLoadBalancersFunc: func(request *clb.DescribeLoadBalancersRequest) (*clb.DescribeLoadBalancersResponse, error) {
			return &clb.DescribeLoadBalancersResponse{
				Response: &clb.DescribeLoadBalancersResponseParams{
					LoadBalancerSet: []*clb.LoadBalancer{
						{
							LoadBalancerId:   common.StringPtr("lb-1"),
							LoadBalancerVips: []*string{common.StringPtr("1.1.1.1")},
						},
						{
							LoadBalancerId:   common.StringPtr("lb-2"),
							LoadBalancerVips: []*string{common.StringPtr("2.2.2.2")},
						},
					},
				},
			}, nil
		},
	}

	factory := &mockClientFactory{clb: mockCLB}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	// Case 1: Success
	vips := c.listCLBVips(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, vips, 2)
	assert.Contains(t, vips, "1.1.1.1")
	assert.Contains(t, vips, "2.2.2.2")

	// Case 2: Cache hit
	// Modify mock to return error, should use cache
	mockCLB.DescribeLoadBalancersFunc = func(request *clb.DescribeLoadBalancersRequest) (*clb.DescribeLoadBalancersResponse, error) {
		return nil, fmt.Errorf("should not be called")
	}
	vipsCached := c.listCLBVips(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, vipsCached, 2)

	// Case 3: Error
	c.cacheMu.Lock()
	delete(c.resCache, c.cacheKey(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", "QCE/LB", "clb"))
	c.cacheMu.Unlock()

	mockCLB.DescribeLoadBalancersFunc = func(request *clb.DescribeLoadBalancersRequest) (*clb.DescribeLoadBalancersResponse, error) {
		return nil, fmt.Errorf("api error")
	}
	vipsError := c.listCLBVips(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Empty(t, vipsError)
}

func TestListBWPIDs(t *testing.T) {
	mockVPC := &mockVPCClient{
		DescribeBandwidthPackagesFunc: func(request *vpc.DescribeBandwidthPackagesRequest) (*vpc.DescribeBandwidthPackagesResponse, error) {
			return &vpc.DescribeBandwidthPackagesResponse{
				Response: &vpc.DescribeBandwidthPackagesResponseParams{
					BandwidthPackageSet: []*vpc.BandwidthPackage{
						{BandwidthPackageId: common.StringPtr("bwp-1")},
						{BandwidthPackageId: common.StringPtr("bwp-2")},
					},
				},
			}, nil
		},
	}

	factory := &mockClientFactory{vpc: mockVPC}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	// Case 1: Success
	ids := c.listBWPIDs(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "bwp-1")

	// Case 2: Cache hit
	mockVPC.DescribeBandwidthPackagesFunc = func(request *vpc.DescribeBandwidthPackagesRequest) (*vpc.DescribeBandwidthPackagesResponse, error) {
		return nil, fmt.Errorf("should not be called")
	}
	idsCached := c.listBWPIDs(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, idsCached, 2)

	// Case 3: Error
	c.cacheMu.Lock()
	delete(c.resCache, c.cacheKey(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", "QCE/BWP", "bwp"))
	c.cacheMu.Unlock()

	mockVPC.DescribeBandwidthPackagesFunc = func(request *vpc.DescribeBandwidthPackagesRequest) (*vpc.DescribeBandwidthPackagesResponse, error) {
		return nil, fmt.Errorf("api error")
	}
	idsError := c.listBWPIDs(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Empty(t, idsError)
}

func TestListCOSBuckets(t *testing.T) {
	mockCOS := &mockCOSClient{
		GetServiceFunc: func(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
			return &cos.ServiceGetResult{
				Buckets: []cos.Bucket{
					{Name: "bucket-1", Region: "ap-guangzhou"},
					{Name: "bucket-2", Region: "ap-shanghai"},
				},
			}, nil, nil
		},
	}

	factory := &mockClientFactory{cos: mockCOS}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	// Case 1: Success (Filter by region)
	buckets := c.listCOSBuckets(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, buckets, 1)
	assert.Equal(t, "bucket-1", buckets[0])

	// Case 2: Cache hit
	mockCOS.GetServiceFunc = func(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
		return nil, nil, fmt.Errorf("should not be called")
	}
	bucketsCached := c.listCOSBuckets(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Len(t, bucketsCached, 1)

	// Case 3: Error
	c.cacheMu.Lock()
	delete(c.resCache, c.cacheKey(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", "QCE/COS", "cos"))
	c.cacheMu.Unlock()

	mockCOS.GetServiceFunc = func(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
		return nil, nil, fmt.Errorf("api error")
	}
	bucketsError := c.listCOSBuckets(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou")
	assert.Empty(t, bucketsError)
}

func TestFetchCLBMonitor_Error(t *testing.T) {
	mockMonitor := &mockMonitorClient{
		GetMonitorDataFunc: func(request *monitor.GetMonitorDataRequest) (*monitor.GetMonitorDataResponse, error) {
			return nil, fmt.Errorf("monitor error")
		},
	}

	factory := &mockClientFactory{monitor: mockMonitor}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	// Should not panic or crash
	prod := config.Product{
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"TrafficRX"}},
		},
	}
	c.fetchCLBMonitor(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", prod, []string{"vip1"})
}

func TestFetchBWPMonitor_Error(t *testing.T) {
	mockMonitor := &mockMonitorClient{
		GetMonitorDataFunc: func(request *monitor.GetMonitorDataRequest) (*monitor.GetMonitorDataResponse, error) {
			return nil, fmt.Errorf("monitor error")
		},
	}

	factory := &mockClientFactory{monitor: mockMonitor}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	prod := config.Product{
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"OutBandwidth"}},
		},
	}
	c.fetchBWPMonitor(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", prod, []string{"bwp1"})
}

func TestFetchCOSMonitor_Error(t *testing.T) {
	mockMonitor := &mockMonitorClient{
		GetMonitorDataFunc: func(request *monitor.GetMonitorDataRequest) (*monitor.GetMonitorDataResponse, error) {
			return nil, fmt.Errorf("monitor error")
		},
	}

	factory := &mockClientFactory{monitor: mockMonitor}
	c := NewCollector(&config.Config{}, nil)
	c.clientFactory = factory

	prod := config.Product{
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"StdStorage"}},
		},
	}
	c.fetchCOSMonitor(config.CloudAccount{AccountID: "acc1"}, "ap-guangzhou", prod, []string{"bucket1"})
}
