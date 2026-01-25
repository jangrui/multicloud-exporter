package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"multicloud-exporter/internal/config"
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
		Provider:  "aws",
	}

	// Test "clb" mapping to CLB
	err := adapter.CollectRegionMetrics(context.Background(), account, "clb", "us-west-2")
	assert.NoError(t, err)

	// Test "lb" mapping to CLB (default)
	err = adapter.CollectRegionMetrics(context.Background(), account, "lb", "us-west-2")
	assert.NoError(t, err)

	// Test "alb" mapping to ALB
	err = adapter.CollectRegionMetrics(context.Background(), account, "alb", "us-west-2")
	assert.NoError(t, err)
}
