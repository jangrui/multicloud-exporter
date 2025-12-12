package tencent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultResources(t *testing.T) {
	c := &Collector{}
	resources := c.GetDefaultResources()
	assert.Equal(t, []string{"clb", "bwp", "s3"}, resources)
}
