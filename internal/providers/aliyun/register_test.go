package aliyun

import (
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultResources(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	resources := c.GetDefaultResources()
	assert.Contains(t, resources, "cbwp")
	assert.Contains(t, resources, "slb")
	assert.Contains(t, resources, "oss")
	assert.GreaterOrEqual(t, len(resources), 3)
}

func TestRegister(t *testing.T) {
	// 验证 factory 是否已注册
	// 注意：由于 init() 在测试启动时已运行，这里只需验证能否获取到 factory
	factory, ok := providers.GetFactory("aliyun")
	assert.True(t, ok, "aliyun provider factory should be registered")
	assert.NotNil(t, factory)

	if factory != nil {
		p := factory(&config.Config{}, &discovery.Manager{})
		assert.NotNil(t, p)
		_, ok := p.(*Collector)
		assert.True(t, ok, "factory should return *aliyun.Collector")
	}
}
