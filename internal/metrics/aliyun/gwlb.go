package aliyun

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("acs_gwlb", "gwlb")
	metrics.RegisterNamespaceHelp("acs_gwlb", func(metric string) string {
		return " - 云产品指标"
	})
}
