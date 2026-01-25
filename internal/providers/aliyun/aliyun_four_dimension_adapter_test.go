package aliyun

import (
	"testing"

	"multicloud-exporter/internal/config"
	"go.uber.org/zap"
	"github.com/stretchr/testify/assert"
)

func TestFourDimensionAdapter_mapProductIDToNamespace(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	// NewCollector 初始化
	collector := NewCollector(cfg, nil, nil)
	
	// 初始化 Logger
	l, _ := zap.NewDevelopment()
	
	adapter := NewFourDimensionAdapter(collector, l)

	tests := []struct {
		name      string
		productID string
		want      string
	}{
		{"SLB", "slb", "acs_slb_dashboard"},
		{"CLB", "clb", "acs_slb_dashboard"},
		{"OSS", "oss", "acs_oss_dashboard"},
		{"S3", "s3", "acs_oss_dashboard"},
		{"CBWP", "cbwp", "acs_bandwidth_package"},
		{"BWP", "bwp", "acs_bandwidth_package"},
		{"ALB", "alb", "acs_alb"},
		{"NLB", "nlb", "acs_nlb"},
		{"GWLB", "gwlb", "acs_gwlb"},
		{"Unknown", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.mapProductIDToNamespace(tt.productID)
			assert.Equal(t, tt.want, got)
		})
	}
}
