package discovery

import (
	"context"
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestAWSDiscoverer_Discover(t *testing.T) {
	tests := []struct {
		name      string
		resources []string
		expected  []string // expected namespaces
	}{
		{
			name:      "Single ALB",
			resources: []string{"alb"},
			expected:  []string{"AWS/ApplicationELB"},
		},
		{
			name:      "Multiple LBs",
			resources: []string{"clb", "nlb"},
			expected:  []string{"AWS/ELB", "AWS/NetworkELB"},
		},
		{
			name:      "All Wildcard",
			resources: []string{"*"},
			expected:  []string{"AWS/S3", "AWS/ApplicationELB", "AWS/ELB", "AWS/NetworkELB", "AWS/GatewayELB"},
		},
		{
			name:      "S3 and GWLB",
			resources: []string{"s3", "gwlb"},
			expected:  []string{"AWS/S3", "AWS/GatewayELB"},
		},
	}

	d := &AWSDiscoverer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AccountsByProvider: map[string][]config.CloudAccount{
					"aws": {
						{
							Provider:  "aws",
							Resources: tt.resources,
						},
					},
				},
			}

			prods := d.Discover(context.Background(), cfg)
			var namespaces []string
			for _, p := range prods {
				namespaces = append(namespaces, p.Namespace)
			}

			for _, exp := range tt.expected {
				assert.Contains(t, namespaces, exp)
			}
			assert.Len(t, prods, len(tt.expected))
		})
	}
}

func TestAWSDiscoverer_MetricInfo(t *testing.T) {
	d := &AWSDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aws": {
				{
					Provider:  "aws",
					Resources: []string{"alb"},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "AWS/ApplicationELB", prods[0].Namespace)

	// Check if metric info is populated
	assert.NotEmpty(t, prods[0].MetricInfo)
	assert.NotEmpty(t, prods[0].MetricInfo[0].MetricList)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "RequestCount")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "ActiveConnectionCount")
}

func TestAWSDiscovery_BuildS3ProductDefault(t *testing.T) {
	product := buildS3ProductDefault()

	assert.Equal(t, "AWS/S3", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 2)

	// 检查第一个 MetricGroup (Storage)
	assert.Equal(t, 86400, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "BucketSizeBytes")
	assert.Contains(t, product.MetricInfo[0].MetricList, "NumberOfObjects")

	// 检查第二个 MetricGroup (Requests)
	assert.Equal(t, 60, *product.MetricInfo[1].Period)
	assert.Contains(t, product.MetricInfo[1].MetricList, "AllRequests")
	assert.Contains(t, product.MetricInfo[1].MetricList, "GetRequests")
}

func TestAWSDiscovery_BuildALBProductDefault(t *testing.T) {
	product := buildAWSALBProductDefault()

	assert.Equal(t, "AWS/ApplicationELB", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "RequestCount")
	assert.Contains(t, product.MetricInfo[0].MetricList, "ActiveConnectionCount")
}

func TestAWSDiscovery_BuildCLBProductDefault(t *testing.T) {
	product := buildAWSCLBProductDefault()

	assert.Equal(t, "AWS/ELB", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "RequestCount")
	assert.Contains(t, product.MetricInfo[0].MetricList, "HealthyHostCount")
}

func TestAWSDiscovery_BuildNLBProductDefault(t *testing.T) {
	product := buildAWSNLBProductDefault()

	assert.Equal(t, "AWS/NetworkELB", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "ActiveFlowCount")
	assert.Contains(t, product.MetricInfo[0].MetricList, "ProcessedBytes")
}

func TestAWSDiscovery_BuildGWLBProductDefault(t *testing.T) {
	product := buildAWSGWLBProductDefault()

	assert.Equal(t, "AWS/GatewayELB", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "ActiveFlowCount")
	assert.Contains(t, product.MetricInfo[0].MetricList, "ProcessedBytes")
}

func TestAWSDiscovery_BuildS3ProductWithCustomConfig(t *testing.T) {
	period := 120
	metricGroups := []config.MetricGroupConfig{
		{
			MetricList: []string{"CustomMetric1", "CustomMetric2"},
			Period:     &period,
		},
	}

	product := buildS3ProductWithCustomConfig(metricGroups)

	assert.Equal(t, "AWS/S3", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestAWSDiscovery_S3_CustomMetricsOnly(t *testing.T) {
	period := 3600
	d := &AWSDiscoverer{}
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aws": {
				{
					Provider:  "aws",
					Resources: []string{"s3"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"s3": {
							{MetricList: []string{"BucketSizeBytes", "NumberOfObjects"}, Period: &period},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "AWS/S3", prods[0].Namespace)
	assert.Len(t, prods[0].MetricInfo, 1)
	assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
	assert.Len(t, prods[0].MetricInfo[0].MetricList, 2)
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "BucketSizeBytes")
	assert.Contains(t, prods[0].MetricInfo[0].MetricList, "NumberOfObjects")
	assert.NotContains(t, prods[0].MetricInfo[0].MetricList, "AllRequests")
}

func TestAWSDiscovery_CustomConfigPerProduct(t *testing.T) {
	period := 120
	tests := []struct {
		name       string
		productKey string
		resource   string
		namespace  string
	}{
		{name: "ALB", productKey: "alb", resource: "alb", namespace: "AWS/ApplicationELB"},
		{name: "CLB", productKey: "clb", resource: "clb", namespace: "AWS/ELB"},
		{name: "NLB", productKey: "nlb", resource: "nlb", namespace: "AWS/NetworkELB"},
		{name: "GWLB", productKey: "gwlb", resource: "gwlb", namespace: "AWS/GatewayELB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &AWSDiscoverer{}
			cfg := &config.Config{
				AccountsByProvider: map[string][]config.CloudAccount{
					"aws": {
						{
							Provider:  "aws",
							Resources: []string{tt.resource},
							ProductMetric: map[string][]config.MetricGroupConfig{
								tt.productKey: {
									{MetricList: []string{"CustomMetric"}, Period: &period},
								},
							},
						},
					},
				},
			}
			prods := d.Discover(context.Background(), cfg)
			assert.Len(t, prods, 1)
			assert.Equal(t, tt.namespace, prods[0].Namespace)
			assert.Len(t, prods[0].MetricInfo, 1)
			assert.Equal(t, period, *prods[0].MetricInfo[0].Period)
			assert.Contains(t, prods[0].MetricInfo[0].MetricList, "CustomMetric")
		})
	}
}

func TestAWSDiscovery_UnionStrategy_ALB(t *testing.T) {
	d := &AWSDiscoverer{}

	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aws": {
				{
					Provider:  "aws",
					Resources: []string{"alb"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"alb": {
							{MetricList: []string{"MetricA"}, Period: intPtr(60)},
						},
					},
				},
				{
					Provider:  "aws",
					Resources: []string{"alb"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"alb": {
							{MetricList: []string{"MetricB"}, Period: intPtr(60)},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "AWS/ApplicationELB", prods[0].Namespace)

	var allMetrics []string
	for _, mg := range prods[0].MetricInfo {
		allMetrics = append(allMetrics, mg.MetricList...)
	}
	assert.Contains(t, allMetrics, "MetricA")
	assert.Contains(t, allMetrics, "MetricB")
}

func TestAWSDiscovery_UnionStrategy(t *testing.T) {
	d := &AWSDiscoverer{}

	// 创建两个账号，都有自定义的 S3 配置
	cfg := &config.Config{
		AccountsByProvider: map[string][]config.CloudAccount{
			"aws": {
				{
					Provider:  "aws",
					Resources: []string{"s3"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"s3": {
							{MetricList: []string{"MetricA", "MetricB"}, Period: intPtr(60)},
						},
					},
				},
				{
					Provider:  "aws",
					Resources: []string{"s3"},
					ProductMetric: map[string][]config.MetricGroupConfig{
						"s3": {
							{MetricList: []string{"MetricC", "MetricD"}, Period: intPtr(120)},
						},
					},
				},
			},
		},
	}

	prods := d.Discover(context.Background(), cfg)
	assert.Len(t, prods, 1)
	assert.Equal(t, "AWS/S3", prods[0].Namespace)

	// 验证两个账号的配置都被合并了（union 策略）
	var allMetrics []string
	for _, mg := range prods[0].MetricInfo {
		allMetrics = append(allMetrics, mg.MetricList...)
	}
	assert.Contains(t, allMetrics, "MetricA")
	assert.Contains(t, allMetrics, "MetricB")
	assert.Contains(t, allMetrics, "MetricC")
	assert.Contains(t, allMetrics, "MetricD")
}
