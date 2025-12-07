package aliyun

import (
	help "multicloud-exporter/internal/help"
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("acs_bandwidth_package", "bwp")
	metrics.RegisterNamespaceMetricAlias("acs_bandwidth_package", map[string]string{
		"in_bandwidth_utilization":  "in_utilization_pct",
		"out_bandwidth_utilization": "out_utilization_pct",
		"net_rx.rate":               "in_bps",
		"net_tx.rate":               "out_bps",
		"net_rx.Pkgs":               "in_pps",
		"net_tx.Pkgs":               "out_pps",
		"in_ratelimit_drop_pps":     "in_drop_pps",
		"out_ratelimit_drop_pps":    "out_drop_pps",
	})
	metrics.RegisterNamespaceHelp("acs_bandwidth_package", help.BWPHelp)
}
