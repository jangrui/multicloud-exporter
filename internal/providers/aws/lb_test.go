package aws

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/metrics"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMain(m *testing.M) {
	// Load config files before running tests (required after removing hardcoded registrations)
	loadTestConfigs()
	m.Run()
}

// loadTestConfigs loads metric mapping configs from the project root
func loadTestConfigs() {
	basePath := "../../.."
	mappingDir := filepath.Join(basePath, "configs", "mappings")

	files, err := filepath.Glob(filepath.Join(mappingDir, "*.yaml"))
	if err != nil {
		panic(fmt.Sprintf("Failed to find config files: %v", err))
	}
	if len(files) == 0 {
		panic("No config files found in configs/mappings")
	}

	// Load all metric mapping files
	for _, file := range files {
		if err := config.LoadMetricMappings(file); err != nil {
			panic(fmt.Sprintf("Failed to load config %s: %v", file, err))
		}
	}
}

type mockDiscoverer struct {
	prods []config.Product
}

func (m *mockDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	return m.prods
}

type errLister struct{}

func (e *errLister) List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error) {
	return nil, errors.New("list error")
}

type emptyLister struct{}

func (e *emptyLister) List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error) {
	return []lbInfo{}, nil
}

func TestGetProductConfig_Found(t *testing.T) {
	prod := config.Product{
		Namespace:    "AWS/ELB",
		AutoDiscover: true,
		MetricInfo:   []config.MetricGroup{{MetricList: []string{"qps"}}},
	}
	discovery.Register("aws", &mockDiscoverer{prods: []config.Product{prod}})
	mgr := discovery.NewManager(&config.Config{})
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh manager: %v", err)
	}
	c := &Collector{disc: mgr}
	got := c.getProductConfig("AWS/ELB")
	if got == nil || got.Namespace != "AWS/ELB" {
		t.Fatalf("getProductConfig failed")
	}
}

func TestCollectLBGeneric_NoProduct(t *testing.T) {
	c := &Collector{}
	acc := config.CloudAccount{AccountID: "acc", Regions: []string{"us-east-1"}}
	c.collectLBGeneric(acc, "AWS/ELB", &emptyLister{})
}

func TestProcessRegionLB_ErrorFromLister(t *testing.T) {
	prod := &config.Product{
		Namespace:  "AWS/ELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"qps"}}},
	}
	c := &Collector{}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &errLister{})
}

type fixedLister struct {
	lbs []lbInfo
}

func (f *fixedLister) List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error) {
	return f.lbs, nil
}

type cwOnlyFactory struct{}

func (f *cwOnlyFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(ak, sk, ""),
	}
	return cloudwatch.NewFromConfig(cfg), nil
}
func (f *cwOnlyFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return &s3.Client{}, nil
}
func (f *cwOnlyFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (f *cwOnlyFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (f *cwOnlyFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

func TestProcessRegionLB_BuildQueries_HandleCWError(t *testing.T) {
	prod := &config.Product{
		Namespace: "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"ProcessedBytes", "ActiveConnectionCount"}},
		},
	}
	lbs := []lbInfo{
		{Name: "alb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-1/aaaaaaaaaaaaaaaa", CodeName: "alb-1"},
		{Name: "alb-2", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-2/bbbbbbbbbbbbbbbb", CodeName: "alb-2"},
	}
	c := &Collector{clientFactory: &cwOnlyFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}

func TestProcessRegionLB_CLB_BuildQueries_HandleCWError(t *testing.T) {
	prod := &config.Product{
		Namespace: "AWS/ELB",
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"RequestCount", "Latency"}},
		},
	}
	lbs := []lbInfo{
		{Name: "clb-1", CodeName: "clb-1"},
	}
	c := &Collector{clientFactory: &cwOnlyFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}

func TestProcessRegionLB_NLB_BuildQueries_HandleCWError(t *testing.T) {
	prod := &config.Product{
		Namespace: "AWS/NetworkELB",
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"ProcessedBytes", "ActiveFlowCount"}},
		},
	}
	lbs := []lbInfo{
		{Name: "nlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/net/nlb-1/cccccccccccccccc", CodeName: "nlb-1"},
	}
	c := &Collector{clientFactory: &cwOnlyFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}

func TestProcessRegionLB_GWLB_BuildQueries_HandleCWError(t *testing.T) {
	prod := &config.Product{
		Namespace: "AWS/GatewayELB",
		MetricInfo: []config.MetricGroup{
			{MetricList: []string{"NewConnection", "ActiveConnectionCount"}},
		},
	}
	lbs := []lbInfo{
		{Name: "gwlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/gwlb/gwlb-1/dddddddddddddddd", CodeName: "gwlb-1"},
	}
	c := &Collector{clientFactory: &cwOnlyFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}
func TestProcessRegionLB_BatchQueries_SplitsCorrectly(t *testing.T) {
	var metricsList []string
	for i := 0; i < 501; i++ {
		metricsList = append(metricsList, fmt.Sprintf("Metric_%d", i))
	}
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: metricsList}},
	}
	lbs := []lbInfo{
		{Name: "alb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-1/aaaaaaaaaaaaaaaa", CodeName: "alb-1"},
	}
	c := &Collector{clientFactory: &cwOnlyFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}

type cwMock struct {
	val float64
}

func (m *cwMock) GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	id := aws.ToString(params.MetricDataQueries[0].Id)
	return &cloudwatch.GetMetricDataOutput{
		MetricDataResults: []cwtypes.MetricDataResult{
			{Id: aws.String(id), Values: []float64{m.val}},
		},
	}, nil
}

type cwMockFactory struct{}

func (cwMockFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return &cwMock{val: 600}, nil
}
func (cwMockFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return &s3.Client{}, nil
}
func (cwMockFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (cwMockFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (cwMockFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

func findGaugeValue(name string, want map[string]string) (float64, bool) {
	families, _ := prometheus.DefaultGatherer.Gather()
	for _, fam := range families {
		if fam.GetName() == name {
			for _, m := range fam.GetMetric() {
				ok := true
				for _, lp := range m.GetLabel() {
					k := lp.GetName()
					v := lp.GetValue()
					if w, exists := want[k]; exists && w != v {
						ok = false
						break
					}
				}
				if ok && m.GetGauge() != nil {
					return m.GetGauge().GetValue(), true
				}
			}
		}
	}
	return 0, false
}

func TestProcessRegionLB_ResultsApplyRate_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"traffic_rx_bps"}}}, // Changed from ProcessedBytes
	}
	lbs := []lbInfo{
		{Name: "alb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-1/aaaaaaaaaaaaaaaa", CodeName: "alb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_traffic_rx_bps", map[string]string{ // Changed from alb_processedbytes
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-1",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "traffic_rx_bps", // Changed from ProcessedBytes
		"code_name":      "alb-1",
	})
	// Expected: (600 / 60) * 8 (scale from config) = 80
	if !ok || val != 80 {
		t.Fatalf("value not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsAverage_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"active_connection"}}}, // Changed from ActiveConnectionCount
	}
	lbs := []lbInfo{
		{Name: "alb-2", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-2/aaaaaaaaaaaaaaaa", CodeName: "alb-2"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_active_connection", map[string]string{ // Changed from alb_activeconnectioncount
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-2",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "active_connection", // Changed from ActiveConnectionCount
		"code_name":      "alb-2",
	})
	if !ok || val != 10 {
		t.Fatalf("average stat not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_DimensionFallback_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"traffic_rx_bps"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-bad", ARN: "bad-arn", CodeName: "alb-bad"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	_, ok := findGaugeValue("alb_traffic_rx_bps", map[string]string{
		"resource_id": "alb-bad",
	})
	if !ok {
		t.Fatalf("dimension fallback not applied")
	}
}

func TestProcessRegionLB_ScaleApplied_ALB(t *testing.T) {
	metrics.Reset()
	// Config file defines scale: 8 for traffic_rx_bps (byte to bit conversion)
	// Mock returns 600 as Sum over 60s period
	// Expected: (600 / 60) * 8 = 80
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"traffic_rx_bps"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-3", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-3/aaaaaaaaaaaaaaaa", CodeName: "alb-3"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_traffic_rx_bps", map[string]string{
		"resource_id": "alb-3",
	})
	if !ok || val != 80 {
		t.Fatalf("scale factor not applied: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsApplyRate_NLB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/NetworkELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"traffic_rx_bps"}}},
	}
	lbs := []lbInfo{
		{Name: "nlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/net/nlb-1/cccccccccccccccc", CodeName: "nlb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("nlb_traffic_rx_bps", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "nlb",
		"resource_id":    "nlb-1",
		"namespace":      "AWS/NetworkELB",
		"metric_name":    "traffic_rx_bps",
		"code_name":      "nlb-1",
	})
	if !ok || val != 10 {
		t.Fatalf("nlb rate not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsAverage_GWLB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/GatewayELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"active_connection"}}},
	}
	lbs := []lbInfo{
		{Name: "gwlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/gwlb/gwlb-1/dddddddddddddddd", CodeName: "gwlb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("gwlb_active_connection", map[string]string{
		"resource_type": "gwlb",
		"resource_id":   "gwlb-1",
	})
	if !ok || val != 10 {
		t.Fatalf("gwlb average not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsAverage_CLB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"rt"}}}, // Changed from Latency
	}
	lbs := []lbInfo{
		{Name: "clb-1", CodeName: "clb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("clb_rt", map[string]string{
		"resource_type": "clb",
		"resource_id":   "clb-1",
	})
	// Expected: (600 / 60) * 1000 (scale from config) = 10000
	if !ok || val != 10000 {
		t.Fatalf("clb average not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_NewConnection_Rate_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"new_connection"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-4", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-4/aaaaaaaaaaaaaaaa", CodeName: "alb-4"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_new_connection", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-4",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "new_connection",
		"code_name":      "alb-4",
	})
	if !ok || val != 10 {
		t.Fatalf("new connection rate not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_HostCount_Average_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"healthy_host_count"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-5", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-5/aaaaaaaaaaaaaaaa", CodeName: "alb-5"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_healthy_host_count", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-5",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "healthy_host_count",
		"code_name":      "alb-5",
	})
	if !ok || val != 10 {
		t.Fatalf("host count average not applied correctly: got=%v ok=%v", val, ok)
	}
}

type cwEmpty struct{}

func (cwEmpty) GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: []cwtypes.MetricDataResult{}}, nil
}

type cwEmptyFactory struct{}

func (cwEmptyFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return cwEmpty{}, nil
}
func (cwEmptyFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return &s3.Client{}, nil
}
func (cwEmptyFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (cwEmptyFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (cwEmptyFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

func TestProcessRegionLB_ExposeZero_WhenNoResults(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"qps"}}},
	}
	lbs := []lbInfo{
		{Name: "clb-0", CodeName: "clb-0"},
	}
	c := &Collector{clientFactory: cwEmptyFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("clb_qps", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "clb",
		"resource_id":    "clb-0",
		"namespace":      "AWS/ELB",
		"metric_name":    "qps",
		"code_name":      "clb-0",
	})
	if !ok || val != 0 {
		t.Fatalf("expected zero gauge exposure, got=%v ok=%v", val, ok)
	}
}

type badCWFactory struct{}

func (badCWFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return nil, errors.New("cw client error")
}
func (badCWFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return nil, nil
}
func (badCWFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return nil, nil
}
func (badCWFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return nil, nil
}
func (badCWFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return nil, nil
}

func TestProcessRegionLB_CWClientError_NoPanic(t *testing.T) {
	prod := &config.Product{
		Namespace:  "AWS/ELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"qps"}}},
	}
	lbs := []lbInfo{{Name: "clb-1"}}
	c := &Collector{clientFactory: badCWFactory{}}
	c.processRegionLB(config.CloudAccount{AccountID: "acc"}, "us-east-1", prod, &fixedLister{lbs: lbs})
}

type regionsFactory struct {
	err error
}

func (f *regionsFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return &cloudwatch.Client{}, nil
}
func (f *regionsFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return nil, nil
}
func (f *regionsFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return nil, nil
}
func (f *regionsFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return nil, nil
}
func (f *regionsFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ec2.Client{}, nil
}

func TestCollectLBGeneric_RegionsWildcard_Fallback(t *testing.T) {
	discovery.Register("aws", &mockDiscoverer{prods: []config.Product{
		{Namespace: "AWS/ApplicationELB", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"traffic_rx_bps"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	c := &Collector{disc: mgr, clientFactory: &regionsFactory{err: errors.New("ec2 error")}}
	acc := config.CloudAccount{AccountID: "acc", Regions: []string{"*"}}
	c.collectLBGeneric(acc, "AWS/ApplicationELB", &emptyLister{})
}
func TestResolveCodeName(t *testing.T) {
	tags := map[string]string{
		"kubernetes.io/service-name": "ns/svc",
		"Name":                       "classic-name",
	}
	if resolveCodeName(tags, "fallback") != "ns/svc" {
		t.Fatalf("resolveCodeName priority mismatch")
	}
	if resolveCodeName(map[string]string{"Name": "n"}, "f") != "n" {
		t.Fatalf("resolveCodeName Name mismatch")
	}
	if resolveCodeName(map[string]string{}, "f") != "f" {
		t.Fatalf("resolveCodeName fallback mismatch")
	}
}
