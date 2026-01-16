package tencent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultResources(t *testing.T) {
	c := &Collector{}
	resources := c.GetDefaultResources()
	assert.ElementsMatch(t, []string{"clb", "bwp", "s3", "gwlb"}, resources)
}
