package aws

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("AWS/NetworkELB", "nlb")
	metrics.RegisterNamespaceHelp("AWS/NetworkELB", func(metric string) string {
		return " - AWS Network Load Balancer Metrics"
	})
}
