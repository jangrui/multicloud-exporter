package tencent

import (
	help "multicloud-exporter/internal/help"
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("QCE/BWP", "bwp")
	metrics.RegisterNamespaceMetricAlias("QCE/BWP", map[string]string{
		"InTraffic":          "in_bps",
		"OutTraffic":         "out_bps",
		"InPkg":              "in_pps",
		"OutPkg":             "out_pps",
		"IntrafficBwpRatio":  "in_utilization_pct",
		"OuttrafficBwpRatio": "out_utilization_pct",
	})
	metrics.RegisterNamespaceHelp("QCE/BWP", help.BWPHelp)
}
