package aws

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("AWS/ApplicationELB", "alb")
	metrics.RegisterNamespaceHelp("AWS/ApplicationELB", func(metric string) string {
		return " - AWS Application Load Balancer Metrics"
	})
}
