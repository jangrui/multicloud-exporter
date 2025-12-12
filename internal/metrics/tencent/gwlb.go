package tencent

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("qce/gwlb", "gwlb")
	metrics.RegisterNamespaceMetricAlias("qce/gwlb", map[string]string{
		"InTraffic":   "traffic_rx_bps",
		"OutTraffic":  "traffic_tx_bps",
		"NewConn":     "new_connection",
		"ConcurConn":  "active_connection",
	})
	metrics.RegisterNamespaceMetricScale("qce/gwlb", map[string]float64{
		"InTraffic":  1000000,
		"OutTraffic": 1000000,
	})
	metrics.RegisterNamespaceHelp("qce/gwlb", func(metric string) string {
		switch metric {
		case "traffic_rx_bps":
			return " - GWLB 入方向带宽（bit/s）"
		case "traffic_tx_bps":
			return " - GWLB 出方向带宽（bit/s）"
		case "new_connection":
			return " - GWLB 新建连接数"
		case "active_connection":
			return " - GWLB 活跃连接数"
		}
		return " - 云产品指标"
	})
}
