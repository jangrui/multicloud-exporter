package discovery

import (
	"context"
	"multicloud-exporter/internal/config"
	"testing"

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
