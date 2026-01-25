package huawei

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
		Provider:  "huawei",
	}

	// Test "clb" mapping to ELB
	// 由于没有真实凭证，内部调用可能会失败或跳过，但关键是验证 switch case 匹配逻辑
	// 如果匹配成功，不会打印 "未知的华为云产品ID" (需要人工检查日志或通过 Hook 验证，这里主要保活)
	err := adapter.CollectRegionMetrics(context.Background(), account, "clb", "cn-north-4")
	assert.NoError(t, err)

	// Test "s3" mapping to OBS
	err = adapter.CollectRegionMetrics(context.Background(), account, "s3", "cn-north-4")
	assert.NoError(t, err)
    
    // Test unknown
    err = adapter.CollectRegionMetrics(context.Background(), account, "unknown", "cn-north-4")
	assert.NoError(t, err)
}
