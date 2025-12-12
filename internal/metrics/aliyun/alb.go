package aliyun

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("acs_alb", "alb")
	metrics.RegisterNamespaceHelp("acs_alb", func(metric string) string {
		return " - 云产品指标"
	})
}
