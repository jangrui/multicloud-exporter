package aws

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("AWS/ELB", "clb")
	metrics.RegisterNamespaceHelp("AWS/ELB", func(metric string) string {
		return " - AWS Classic Load Balancer Metrics"
	})
}
