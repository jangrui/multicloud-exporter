package aws

import (
	"context"
	"testing"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers/common"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/prometheus/client_golang/prometheus"
)

func TestClassifyAWSError(t *testing.T) {
	cases := map[string]string{
		"ExpiredToken":         "auth_error",
		"InvalidClientTokenId": "auth_error",
		"AccessDenied":         "auth_error",
		"Throttling":           "limit_error",
		"Rate exceeded":        "limit_error",
		"TooManyRequests":      "limit_error",
		"timeout":              "network_error",
		"network":              "network_error",
		"other":                "error",
	}
	for msg, want := range cases {
		got := common.ClassifyAWSError(errString(msg))
		if got != want {
			t.Fatalf("classify %q: got=%q want=%q", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestSanitizeCWQueryID(t *testing.T) {
	if sanitizeCWQueryID(0) != "q0" {
		t.Fatalf("sanitize id 0 mismatch")
	}
	if sanitizeCWQueryID(123) != "q123" {
		t.Fatalf("sanitize id 123 mismatch")
	}
}

func TestStatForS3Metric(t *testing.T) {
	if statForS3Metric("BucketSizeBytes") != "Average" {
		t.Fatalf("BucketSizeBytes stat mismatch")
	}
	if statForS3Metric("NumberOfObjects") != "Average" {
		t.Fatalf("NumberOfObjects stat mismatch")
	}
	if statForS3Metric("FirstByteLatency") != "Average" {
		t.Fatalf("FirstByteLatency stat mismatch")
	}
	if statForS3Metric("TotalRequestLatency") != "Average" {
		t.Fatalf("TotalRequestLatency stat mismatch")
	}
	if statForS3Metric("GetRequests") != "Sum" {
		t.Fatalf("GetRequests stat mismatch")
	}
}

func TestFetchS3BucketCodeNames_ErrorNoPanic(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("", "", ""),
	}
	client := s3.NewFromConfig(cfg)
	c := &Collector{}
	out := c.fetchS3BucketCodeNames(context.Background(), client, []string{"b1", "b2"})
	if len(out) != 0 {
		t.Fatalf("expected no code names, got=%v", out)
	}
}

func TestCollectS3_NoConfig(t *testing.T) {
	c := &Collector{}
	c.collectS3(config.CloudAccount{AccountID: "a"})
}

func TestCollectS3_NoProduct(t *testing.T) {
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	c := &Collector{cfg: &config.Config{}, disc: mgr}
	c.collectS3(config.CloudAccount{AccountID: "a"})
}

type mockDiscovererS3 struct {
	prods []config.Product
}

func (m *mockDiscovererS3) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	return m.prods
}

type localS3Factory struct{}

func (localS3Factory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return &cloudwatch.Client{}, nil
}
func (localS3Factory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(ak, sk, ""),
	}
	return s3.NewFromConfig(cfg), nil
}
func (localS3Factory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (localS3Factory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (localS3Factory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

func TestCollectS3_ListBuckets_ErrorPaths(t *testing.T) {
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"BucketSizeBytes"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: localS3Factory{}}
	c.collectS3(config.CloudAccount{AccountID: "a"})
}

type s3ListMock struct {
	buckets []string
}

func (m *s3ListMock) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	out := &s3.ListBucketsOutput{}
	for _, b := range m.buckets {
		out.Buckets = append(out.Buckets, s3types.Bucket{Name: aws.String(b)})
	}
	return out, nil
}
func (m *s3ListMock) GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	return nil, errString("NoSuchTagSet")
}

type cwS3Mock struct {
	values map[string]float64
}

func (m *cwS3Mock) GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	var results []cwtypes.MetricDataResult
	for _, q := range params.MetricDataQueries {
		id := aws.ToString(q.Id)
		val := m.values[id]
		results = append(results, cwtypes.MetricDataResult{Id: aws.String(id), Values: []float64{val}})
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

type cwS3FlakyMock struct {
	values map[string]float64
	calls  int
}

func (m *cwS3FlakyMock) GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	m.calls++
	if m.calls == 1 {
		return nil, errString("Throttling")
	}
	var results []cwtypes.MetricDataResult
	for _, q := range params.MetricDataQueries {
		id := aws.ToString(q.Id)
		val := m.values[id]
		results = append(results, cwtypes.MetricDataResult{Id: aws.String(id), Values: []float64{val}})
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

type s3CWFactory struct {
	buckets []string
	values  map[string]float64
}

func (f s3CWFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return &cwS3Mock{values: f.values}, nil
}
func (f s3CWFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return &s3ListMock{buckets: f.buckets}, nil
}
func (f s3CWFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (f s3CWFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (f s3CWFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

type s3FlakyFactory struct {
	buckets []string
	values  map[string]float64
}

func (f s3FlakyFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return &cwS3FlakyMock{values: f.values}, nil
}
func (f s3FlakyFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return &s3ListFlaky{buckets: f.buckets}, nil
}
func (f s3FlakyFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return &elasticloadbalancing.Client{}, nil
}
func (f s3FlakyFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return &elasticloadbalancingv2.Client{}, nil
}
func (f s3FlakyFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	return &ec2.Client{}, nil
}

type s3ListFlaky struct {
	buckets []string
	calls   int
}

func (m *s3ListFlaky) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	m.calls++
	if m.calls == 1 {
		return nil, errString("Throttling")
	}
	out := &s3.ListBucketsOutput{}
	for _, b := range m.buckets {
		out.Buckets = append(out.Buckets, s3types.Bucket{Name: aws.String(b)})
	}
	return out, nil
}
func (m *s3ListFlaky) GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	return nil, errString("NoSuchTagSet")
}

func findCounterValue(name string, want map[string]string) (float64, bool) {
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
				if ok && m.GetCounter() != nil {
					return m.GetCounter().GetValue(), true
				}
			}
		}
	}
	return 0, false
}

func registerStdCounters() {
	_ = prometheus.Register(metrics.RateLimitTotal)
	_ = prometheus.Register(metrics.RequestTotal)
	_ = prometheus.Register(metrics.RequestDuration)
}

func TestCollectS3_BucketSizeBytes_AverageLabelsAndValue(t *testing.T) {
	metrics.Reset()
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"BucketSizeBytes"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3CWFactory{
		buckets: []string{"b1"},
		values:  map[string]float64{"q0": 1024},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	val, ok := findGaugeValue("s3_storage_usage_bytes", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "global",
		"resource_type":  "s3",
		"resource_id":    "b1",
		"namespace":      "AWS/S3",
		"metric_name":    "BucketSizeBytes",
		"bucketname":     "b1",
		"storagetype":    "StandardStorage",
		"code_name":      "",
	})
	if !ok || val != 1024 {
		t.Fatalf("BucketSizeBytes value mismatch: got=%v ok=%v", val, ok)
	}
}

func TestCollectS3_GetRequests_FilterIdLabel(t *testing.T) {
	metrics.Reset()
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"GetRequests"}, Period: func() *int { v := 60; return &v }()}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3CWFactory{
		buckets: []string{"b2"},
		values:  map[string]float64{"q0": 3600},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	val, ok := findGaugeValue("s3_requests_get", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "global",
		"resource_type":  "s3",
		"resource_id":    "b2",
		"namespace":      "AWS/S3",
		"metric_name":    "GetRequests",
		"bucketname":     "b2",
		"filterid":       "EntireBucket",
		"code_name":      "",
	})
	if !ok || val != 60 {
		t.Fatalf("GetRequests value mismatch: got=%v ok=%v", val, ok)
	}
}

func TestCollectS3_NumberOfObjects_StorageTypeAll(t *testing.T) {
	metrics.Reset()
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"NumberOfObjects"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3CWFactory{
		buckets: []string{"b3"},
		values:  map[string]float64{"q0": 42},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	val, ok := findGaugeValue("s3_number_of_objects", map[string]string{
		"cloud_provider": "aws",
		"account_id":     "acc",
		"region":         "global",
		"resource_type":  "s3",
		"resource_id":    "b3",
		"namespace":      "AWS/S3",
		"metric_name":    "NumberOfObjects",
		"bucketname":     "b3",
		"storagetype":    "AllStorageTypes",
		"code_name":      "",
	})
	if !ok || val != 42 {
		t.Fatalf("NumberOfObjects value mismatch: got=%v ok=%v", val, ok)
	}
}

func TestCollectS3_ScaleApplied_GetRequests(t *testing.T) {
	metrics.Reset()
	metrics.RegisterNamespaceMetricScale("AWS/S3", map[string]float64{"GetRequests": 0.5})
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"GetRequests"}, Period: func() *int { v := 60; return &v }()}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3CWFactory{
		buckets: []string{"b4"},
		values:  map[string]float64{"q0": 3600},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	val, ok := findGaugeValue("s3_requests_get", map[string]string{
		"resource_id": "b4",
		"code_name":   "",
	})
	if !ok || val != 30 {
		t.Fatalf("S3 scale factor not applied: got=%v ok=%v", val, ok)
	}
}

func TestCollectS3_ListBuckets_RateLimitCounter(t *testing.T) {
	metrics.Reset()
	registerStdCounters()
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"BucketSizeBytes"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3FlakyFactory{
		buckets: []string{"b5"},
		values:  map[string]float64{"q0": 1},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	cnt, ok := findCounterValue("multicloud_rate_limit_total", map[string]string{
		"cloud_provider": "aws",
		"api":            "ListBuckets",
	})
	if !ok || cnt < 1 {
		t.Fatalf("expected rate limit counter for ListBuckets, got=%v ok=%v", cnt, ok)
	}
}

func TestCollectS3_GetMetricData_RateLimitCounter(t *testing.T) {
	metrics.Reset()
	registerStdCounters()
	discovery.Register("aws", &mockDiscovererS3{prods: []config.Product{
		{Namespace: "AWS/S3", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: []string{"GetRequests"}}}},
	}})
	mgr := discovery.NewManager(&config.Config{})
	_ = mgr.Refresh(context.Background())
	f := s3FlakyFactory{
		buckets: []string{"b6"},
		values:  map[string]float64{"q0": 100},
	}
	c := &Collector{cfg: &config.Config{}, disc: mgr, clientFactory: f}
	c.collectS3(config.CloudAccount{AccountID: "acc"})
	cnt, ok := findCounterValue("multicloud_rate_limit_total", map[string]string{
		"cloud_provider": "aws",
		"api":            "GetMetricData",
	})
	if !ok || cnt < 1 {
		t.Fatalf("expected rate limit counter for GetMetricData, got=%v ok=%v", cnt, ok)
	}
	// also ensure RequestTotal has both error and success
	errCnt, ok2 := findCounterValue("multicloud_request_total", map[string]string{
		"cloud_provider": "aws",
		"api":            "GetMetricData",
		"status":         "limit_error",
	})
	succCnt, ok3 := findCounterValue("multicloud_request_total", map[string]string{
		"cloud_provider": "aws",
		"api":            "GetMetricData",
		"status":         "success",
	})
	if !ok2 || !ok3 || errCnt < 1 || succCnt < 1 {
		t.Fatalf("expected request counters for error and success, got err=%v ok2=%v succ=%v ok3=%v", errCnt, ok2, succCnt, ok3)
	}
}
