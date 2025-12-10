package huawei

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHuaweiDefaultResources(t *testing.T) {
	c := NewCollector()
	resources := c.GetDefaultResources()
	// Currently returns empty list
	assert.Empty(t, resources)
}
