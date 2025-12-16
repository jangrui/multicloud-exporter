package aliyun

import (
	help "multicloud-exporter/internal/help"
	metrics "multicloud-exporter/internal/metrics"
)

func init() {
	metrics.RegisterNamespacePrefix("acs_bandwidth_package", "bwp")
	// 注意：映射配置已迁移到 configs/mappings/bwp.metrics.yaml
	// 这里保留硬编码映射作为向后兼容，但会被配置文件覆盖
	// 配置文件使用统一命名：traffic_rx_bps, traffic_tx_bps, packet_rx, packet_tx, utilization_rx_pct, utilization_tx_pct
	metrics.RegisterNamespaceMetricAlias("acs_bandwidth_package", map[string]string{
		"in_bandwidth_utilization":  "utilization_rx_pct", // 统一命名，与配置文件一致
		"out_bandwidth_utilization": "utilization_tx_pct", // 统一命名，与配置文件一致
		"net_rx.rate":               "traffic_rx_bps",     // 统一命名，与配置文件一致
		"net_tx.rate":               "traffic_tx_bps",     // 统一命名，与配置文件一致
		"net_rx.Pkgs":               "packet_rx",          // 统一命名，与配置文件一致
		"net_tx.Pkgs":               "packet_tx",          // 统一命名，与配置文件一致
		"in_ratelimit_drop_pps":     "drop_rx_pps",        // 统一命名
		"out_ratelimit_drop_pps":    "drop_tx_pps",        // 统一命名
		"DownstreamBandwidth":       "traffic_rx_bps",     // 统一命名，与配置文件一致
		"UpstreamBandwidth":         "traffic_tx_bps",     // 统一命名，与配置文件一致
		"DownstreamPacket":          "packet_rx",          // 统一命名，与配置文件一致
		"UpstreamPacket":            "packet_tx",          // 统一命名，与配置文件一致
	})
	metrics.RegisterNamespaceHelp("acs_bandwidth_package", help.BWPHelp)
}
