package tencent

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("QCE/CLB", "lb")
	metrics.RegisterNamespaceMetricAlias("QCE/CLB", map[string]string{
		"VipIntraffic":   "traffic_rx_bps",
		"VipOuttraffic":  "traffic_tx_bps",
		"VipInpkg":       "packet_rx",
		"VipOutpkg":      "packet_tx",
		"Vipindroppkts":  "drop_packet_rx",
		"Vipoutdroppkts": "drop_packet_tx",
	})
	metrics.RegisterNamespaceHelp("QCE/CLB", func(metric string) string {
		switch metric {
		case "traffic_rx_bps":
			return " - LB 入方向带宽（bit/s）"
		case "traffic_tx_bps":
			return " - LB 出方向带宽（bit/s）"
		case "packet_rx":
			return " - LB 入包速率"
		case "packet_tx":
			return " - LB 出包速率"
		case "drop_packet_rx":
			return " - LB 入向丢包速率"
		case "drop_packet_tx":
			return " - LB 出向丢包速率"
		}
		return " - 云产品指标"
	})
}
