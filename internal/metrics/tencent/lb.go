package tencent

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("QCE/LB", "clb")
	metrics.RegisterNamespaceMetricAlias("QCE/LB", map[string]string{
		"VipIntraffic":       "traffic_rx_bps",
		"VipOuttraffic":      "traffic_tx_bps",
		"VipInpkg":           "packet_rx",
		"VipOutpkg":          "packet_tx",
		"Vipindroppkts":      "drop_packet_rx",
		"Vipoutdroppkts":     "drop_packet_tx",
		"IntrafficVipRatio":  "traffic_rx_utilization_pct",
		"OuttrafficVipRatio": "traffic_tx_utilization_pct",
		"VNewConn":           "vip_new_connection",
		"NewConn":            "new_connection",
		"Connum":             "active_connection",
	})
	metrics.RegisterNamespaceHelp("QCE/LB", func(metric string) string {
		switch metric {
		case "traffic_rx_bps":
			return " - CLB 入方向带宽（bit/s）"
		case "traffic_tx_bps":
			return " - CLB 出方向带宽（bit/s）"
		case "packet_rx":
			return " - CLB 入包速率"
		case "packet_tx":
			return " - CLB 出包速率"
		case "drop_packet_rx":
			return " - CLB 入向丢包速率"
		case "drop_packet_tx":
			return " - CLB 出向丢包速率"
		case "drop_connection":
			return " - CLB 断开连接"
		case "traffic_rx_utilization_pct":
			return " - CLB 入向带宽利用率（%）"
		case "traffic_tx_utilization_pct":
			return " - CLB 出向带宽利用率（%）"
		case "vip_new_connection":
			return " - CLB VIP 新建连接数"
		case "new_connection":
			return " - CLB 新建连接数"
		case "active_connection":
			return " - CLB 活跃连接数"
		}
		return " - 云产品指标"
	})
}
