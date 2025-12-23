package aws

import (
	"context"
	"errors"
	"fmt"
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
		MetricInfo:   []config.MetricGroup{{MetricList: []string{"RequestCount"}}},
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"RequestCount"}}},
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ProcessedBytes"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-1/aaaaaaaaaaaaaaaa", CodeName: "alb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_processedbytes", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-1",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "ProcessedBytes",
		"code_name":      "alb-1",
	})
	if !ok || val != 10 {
		t.Fatalf("value not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsAverage_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ActiveConnectionCount"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-2", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-2/aaaaaaaaaaaaaaaa", CodeName: "alb-2"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_activeconnectioncount", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-2",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "ActiveConnectionCount",
		"code_name":      "alb-2",
	})
	if !ok || val != 600 {
		t.Fatalf("average stat not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_DimensionFallback_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ProcessedBytes"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-bad", ARN: "bad-arn", CodeName: "alb-bad"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	_, ok := findGaugeValue("alb_processedbytes", map[string]string{
		"resource_id": "alb-bad",
	})
	if !ok {
		t.Fatalf("dimension fallback not applied")
	}
}

func TestProcessRegionLB_ScaleApplied_ALB(t *testing.T) {
	metrics.Reset()
	metrics.RegisterNamespaceMetricScale("AWS/ApplicationELB", map[string]float64{"ProcessedBytes": 0.5})
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ProcessedBytes"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-3", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-3/aaaaaaaaaaaaaaaa", CodeName: "alb-3"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_processedbytes", map[string]string{
		"resource_id": "alb-3",
	})
	if !ok || val != 5 {
		t.Fatalf("scale factor not applied: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsApplyRate_NLB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/NetworkELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ProcessedBytes"}}},
	}
	lbs := []lbInfo{
		{Name: "nlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/net/nlb-1/cccccccccccccccc", CodeName: "nlb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("nlb_processedbytes", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "nlb",
		"resource_id":    "nlb-1",
		"namespace":      "AWS/NetworkELB",
		"metric_name":    "ProcessedBytes",
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"ActiveConnectionCount"}}},
	}
	lbs := []lbInfo{
		{Name: "gwlb-1", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/gwlb/gwlb-1/dddddddddddddddd", CodeName: "gwlb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("gwlb_activeconnectioncount", map[string]string{
		"resource_type": "gwlb",
		"resource_id":   "gwlb-1",
	})
	if !ok || val != 600 {
		t.Fatalf("gwlb average not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_ResultsAverage_CLB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"Latency"}}},
	}
	lbs := []lbInfo{
		{Name: "clb-1", CodeName: "clb-1"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("clb_latency", map[string]string{
		"resource_type": "clb",
		"resource_id":   "clb-1",
	})
	if !ok || val != 600 {
		t.Fatalf("clb average not applied correctly: got=%v ok=%v", val, ok)
	}
}

func TestProcessRegionLB_NewConnection_Rate_ALB(t *testing.T) {
	metrics.Reset()
	prod := &config.Product{
		Namespace:  "AWS/ApplicationELB",
		MetricInfo: []config.MetricGroup{{MetricList: []string{"NewConnectionCount"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-4", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-4/aaaaaaaaaaaaaaaa", CodeName: "alb-4"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_newconnectioncount", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-4",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "NewConnectionCount",
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"HealthyHostCount"}}},
	}
	lbs := []lbInfo{
		{Name: "alb-5", ARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-5/aaaaaaaaaaaaaaaa", CodeName: "alb-5"},
	}
	c := &Collector{clientFactory: cwMockFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("alb_healthyhostcount", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "alb",
		"resource_id":    "alb-5",
		"namespace":      "AWS/ApplicationELB",
		"metric_name":    "HealthyHostCount",
		"code_name":      "alb-5",
	})
	if !ok || val != 600 {
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"RequestCount"}}},
	}
	lbs := []lbInfo{
		{Name: "clb-0", CodeName: "clb-0"},
	}
	c := &Collector{clientFactory: cwEmptyFactory{}}
	acc := config.CloudAccount{AccountID: "acc"}
	c.processRegionLB(acc, "us-east-1", prod, &fixedLister{lbs: lbs})
	val, ok := findGaugeValue("clb_requestcount", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "us-east-1",
		"resource_type":  "clb",
		"resource_id":    "clb-0",
		"namespace":      "AWS/ELB",
		"metric_name":    "RequestCount",
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
		MetricInfo: []config.MetricGroup{{MetricList: []string{"RequestCount"}}},
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
		{Namespace: "AWS/ApplicationELB", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"ProcessedBytes"}}}},
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
