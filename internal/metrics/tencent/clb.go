package tencent

import (
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("QCE/LB", "clb")
	metrics.RegisterNamespaceMetricAlias("QCE/LB", map[string]string{
		"VIntraffic":         "traffic_rx_bps", // VIP 入流量指标（实际采集到的指标名）
		"VOuttraffic":        "traffic_tx_bps", // VIP 出流量指标（实际采集到的指标名）
		"VInpkg":             "packet_rx",      // VIP 入包速率指标（实际采集到的指标名）
		"VOutpkg":            "packet_tx",      // VIP 出包速率指标（实际采集到的指标名）
		"Vipindroppkts":      "drop_packet_rx",
		"Vipoutdroppkts":     "drop_packet_tx",
		"IntrafficVipRatio":  "traffic_rx_utilization_pct",
		"OuttrafficVipRatio": "traffic_tx_utilization_pct",
		"VNewConn":           "vip_new_connection",
		"VConnum":            "active_connection", // VIP 活跃连接数指标（实际采集到的指标名）
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
