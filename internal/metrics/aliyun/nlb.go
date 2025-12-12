package aliyun

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("acs_nlb", "nlb")
	metrics.RegisterNamespaceHelp("acs_nlb", func(metric string) string {
		return " - 云产品指标"
	})
}
