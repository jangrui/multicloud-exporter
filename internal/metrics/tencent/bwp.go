package tencent

import (
	help "multicloud-exporter/internal/help"
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("QCE/BWP", "bwp")
	// 注意：映射配置已迁移到 configs/mappings/bwp.metrics.yaml
	// 这里保留硬编码映射作为向后兼容，但会被配置文件覆盖
	// 配置文件使用统一命名：traffic_rx_bps, traffic_tx_bps, packet_rx, packet_tx, utilization_rx_pct, utilization_tx_pct
	metrics.RegisterNamespaceMetricAlias("QCE/BWP", map[string]string{
		"InTraffic":          "traffic_rx_bps",     // 统一命名，与配置文件一致
		"OutTraffic":         "traffic_tx_bps",     // 统一命名，与配置文件一致
		"InPkg":              "packet_rx",          // 统一命名，与配置文件一致
		"OutPkg":             "packet_tx",          // 统一命名，与配置文件一致
		"IntrafficBwpRatio":  "utilization_rx_pct", // 统一命名，与配置文件一致
		"OuttrafficBwpRatio": "utilization_tx_pct", // 统一命名，与配置文件一致
	})
	metrics.RegisterNamespaceHelp("QCE/BWP", help.BWPHelp)
}
