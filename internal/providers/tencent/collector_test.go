package tencent

import (
	"context"
	"reflect"
	"testing"
	"unsafe"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
	"github.com/tencentyun/cos-go-sdk-v5"
)

func setDiscoveryProducts(t *testing.T, mgr *discovery.Manager, products map[string][]config.Product) {
	val := reflect.ValueOf(mgr).Elem()
	field := val.FieldByName("products")
	if !field.IsValid() {
		t.Fatal("field 'products' not found in discovery.Manager")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(products))
}

func TestCollector_Collect(t *testing.T) {
	// Setup Mocks
	mockCVM := &mockCVMClient{
		DescribeRegionsFunc: func(request *cvm.DescribeRegionsRequest) (*cvm.DescribeRegionsResponse, error) {
			return &cvm.DescribeRegionsResponse{
				Response: &cvm.DescribeRegionsResponseParams{
					RegionSet: []*cvm.RegionInfo{
						{Region: common.StringPtr("ap-guangzhou")},
					},
				},
			}, nil
		},
	}

	mockCLB := &mockCLBClient{
		DescribeLoadBalancersFunc: func(request *clb.DescribeLoadBalancersRequest) (*clb.DescribeLoadBalancersResponse, error) {
			return &clb.DescribeLoadBalancersResponse{
				Response: &clb.DescribeLoadBalancersResponseParams{
					LoadBalancerSet: []*clb.LoadBalancer{
						{
							LoadBalancerId: common.StringPtr("lb-123"),
							LoadBalancerVips: []*string{
								common.StringPtr("1.2.3.4"),
							},
						},
					},
				},
			}, nil
		},
	}

	mockVPC := &mockVPCClient{
		DescribeBandwidthPackagesFunc: func(request *vpc.DescribeBandwidthPackagesRequest) (*vpc.DescribeBandwidthPackagesResponse, error) {
			return &vpc.DescribeBandwidthPackagesResponse{
				Response: &vpc.DescribeBandwidthPackagesResponseParams{
					BandwidthPackageSet: []*vpc.BandwidthPackage{
						{BandwidthPackageId: common.StringPtr("bwp-123")},
					},
				},
			}, nil
		},
	}

	mockMonitor := &mockMonitorClient{
		GetMonitorDataFunc: func(request *monitor.GetMonitorDataRequest) (*monitor.GetMonitorDataResponse, error) {
			return &monitor.GetMonitorDataResponse{
				Response: &monitor.GetMonitorDataResponseParams{
					DataPoints: []*monitor.DataPoint{
						{
							Dimensions: []*monitor.Dimension{
								{Name: common.StringPtr("vip"), Value: common.StringPtr("1.2.3.4")},
								{Name: common.StringPtr("bandwidthPackageId"), Value: common.StringPtr("bwp-123")},
								{Name: common.StringPtr("bucket"), Value: common.StringPtr("my-bucket")},
							},
							Values: []*float64{common.Float64Ptr(100.0)},
						},
					},
				},
			}, nil
		},
	}

	mockCOS := &mockCOSClient{
		GetServiceFunc: func(ctx context.Context) (*cos.ServiceGetResult, *cos.Response, error) {
			return &cos.ServiceGetResult{
				Buckets: []cos.Bucket{
					{Name: "my-bucket", Region: "ap-guangzhou"},
				},
			}, nil, nil
		},
	}

	factory := &mockClientFactory{
		cvm:     mockCVM,
		clb:     mockCLB,
		vpc:     mockVPC,
		monitor: mockMonitor,
		cos:     mockCOS,
	}

	cfg := &config.Config{
		Server: &config.ServerConf{
			RegionConcurrency: 1,
			MetricConcurrency: 1,
		},
	}
	mgr := discovery.NewManager(cfg)

	setDiscoveryProducts(t, mgr, map[string][]config.Product{
		"tencent": {
			{
				Namespace: "QCE/LB",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"Traffic"},
					},
				},
			},
			{
				Namespace: "QCE/BWP",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"BandwidthUsage"},
					},
				},
			},
			{
				Namespace: "QCE/COS",
				MetricInfo: []config.MetricGroup{
					{
						MetricList: []string{"StorageUsage"},
					},
				},
			},
		},
	})

	c := NewCollector(cfg, mgr)
	c.clientFactory = factory

	// Execute Collect for CLB
	c.Collect(config.CloudAccount{
		AccountID:       "test-acc",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Regions:         []string{"*"},
		Resources:       []string{"clb", "bwp", "cos"},
	})
}
