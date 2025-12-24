package huawei

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"multicloud-exporter/internal/config"
)

func TestHuaweiDefaultResources(t *testing.T) {
	c := NewCollector(&config.Config{}, nil)
	resources := c.GetDefaultResources()
	// Should return clb and s3
	assert.Contains(t, resources, "clb")
	assert.Contains(t, resources, "s3")
	assert.Len(t, resources, 2)
}
