package tencent

import (
	"context"
	"testing"

	"multicloud-exporter/internal/config"
	"go.uber.org/zap"
	"github.com/stretchr/testify/assert"
)

func TestFourDimensionAdapter_CollectRegionMetrics_SwitchCase(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	// NewCollector 初始化
	collector := NewCollector(cfg, nil, nil)
	
	// 初始化 Logger
	l, _ := zap.NewDevelopment()
	
	adapter := NewFourDimensionAdapter(collector, l)

	account := config.CloudAccount{
		AccountID: "test-account",
		Provider:  "tencent",
	}

	// Test "s3" mapping to COS
	err := adapter.CollectRegionMetrics(context.Background(), account, "s3", "ap-guangzhou")
	assert.NoError(t, err)

	// Test "cos" mapping to COS
	err = adapter.CollectRegionMetrics(context.Background(), account, "cos", "ap-guangzhou")
	assert.NoError(t, err)
}
