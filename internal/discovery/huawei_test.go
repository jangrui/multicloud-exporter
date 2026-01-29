package discovery

import (
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestHuaweiDiscovery_BuildOBSProductDefault(t *testing.T) {
	product := buildOBSProductDefault(nil)

	assert.Equal(t, "SYS.OBS", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 2)

	// 检查第一个 MetricGroup (Capacity)
	assert.Equal(t, 86400, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "capacity_total")
	assert.Contains(t, product.MetricInfo[0].MetricList, "object_num_all")

	// 检查第二个 MetricGroup (Request)
	assert.Equal(t, 300, *product.MetricInfo[1].Period)
	assert.Contains(t, product.MetricInfo[1].MetricList, "get_request_count")
	assert.Contains(t, product.MetricInfo[1].MetricList, "put_request_count")
}

func TestHuaweiDiscovery_BuildOBSProductWithCustomConfig(t *testing.T) {
	period := 120
	accounts := []config.CloudAccount{
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"obs": {
					{
						MetricList: []string{"CustomMetric1", "CustomMetric2"},
						Period:     &period,
					},
				},
			},
		},
	}

	product := buildOBSProductWithCustomConfig(accounts)

	assert.Equal(t, "SYS.OBS", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestHuaweiDiscovery_UnionStrategy(t *testing.T) {
	period1 := 60
	period2 := 120

	accounts := []config.CloudAccount{
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"obs": {
					{MetricList: []string{"MetricA", "MetricB"}, Period: &period1},
					{MetricList: []string{"MetricC"}, Period: &period2},
				},
			},
		},
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"obs": {
					{MetricList: []string{"MetricA", "MetricD"}, Period: &period1},
					{MetricList: []string{"MetricE"}, Period: &period2},
				},
			},
		},
	}

	product := buildOBSProductWithCustomConfig(accounts)

	assert.Equal(t, "SYS.OBS", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 2)

	// 检查 Period=60 的 MetricGroup（union: MetricA, MetricB, MetricD）
	assert.Equal(t, period1, *product.MetricInfo[0].Period)
	assert.Len(t, product.MetricInfo[0].MetricList, 3)
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricA")
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricB")
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricD")

	// 检查 Period=120 的 MetricGroup（union: MetricC, MetricE）
	assert.Equal(t, period2, *product.MetricInfo[1].Period)
	assert.Len(t, product.MetricInfo[1].MetricList, 2)
	assert.Contains(t, product.MetricInfo[1].MetricList, "MetricC")
	assert.Contains(t, product.MetricInfo[1].MetricList, "MetricE")
}

func TestHuaweiDiscovery_BuildELBProductDefault(t *testing.T) {
	fallback := []string{"m1_cps", "m2_act_conn"}
	product := buildELBProductDefault(fallback)

	assert.Equal(t, "SYS.ELB", product.Namespace)
	assert.True(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Contains(t, product.MetricInfo[0].MetricList, "m1_cps")
	assert.Contains(t, product.MetricInfo[0].MetricList, "m2_act_conn")
}

func TestHuaweiDiscovery_BuildELBProductWithCustomConfig(t *testing.T) {
	period := 120
	accounts := []config.CloudAccount{
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"elb": {
					{
						MetricList: []string{"CustomMetric1", "CustomMetric2"},
						Period:     &period,
					},
				},
			},
		},
	}

	product := buildELBProductWithCustomConfig(accounts)

	assert.Equal(t, "SYS.ELB", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric1")
	assert.Contains(t, product.MetricInfo[0].MetricList, "CustomMetric2")
}

func TestHuaweiDiscovery_UnionStrategy_ELB(t *testing.T) {
	period := 60
	accounts := []config.CloudAccount{
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"elb": {
					{MetricList: []string{"MetricA", "MetricB"}, Period: &period},
				},
			},
		},
		{
			ProductMetric: map[string][]config.MetricGroupConfig{
				"elb": {
					{MetricList: []string{"MetricB", "MetricC"}, Period: &period},
				},
			},
		},
	}

	product := buildELBProductWithCustomConfig(accounts)

	assert.Equal(t, "SYS.ELB", product.Namespace)
	assert.False(t, product.AutoDiscover)
	assert.Len(t, product.MetricInfo, 1)
	assert.Equal(t, period, *product.MetricInfo[0].Period)
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricA")
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricB")
	assert.Contains(t, product.MetricInfo[0].MetricList, "MetricC")
}
