package aws

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("AWS/GatewayELB", "gwlb")
	metrics.RegisterNamespaceHelp("AWS/GatewayELB", func(metric string) string {
		return " - AWS Gateway Load Balancer Metrics"
	})
}
