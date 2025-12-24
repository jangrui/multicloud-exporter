// 华为云 OBS 指标别名注册
package huawei

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	// 注册命名空间前缀：SYS.OBS -> s3
	metrics.RegisterNamespacePrefix("SYS.OBS", "s3")

	// 注册指标别名映射（华为云指标名 -> 统一指标名）
	metrics.RegisterNamespaceMetricAlias("SYS.OBS", map[string]string{
		// 存储容量指标
		"capacity_total":                          "storage_usage_bytes",
		"capacity_standard":                       "storage_standard_bytes",
		"capacity_infrequent_access":              "storage_ia_bytes",
		"capacity_archive":                        "storage_archive_bytes",
		"capacity_deep_archive":                   "storage_deep_archive_bytes",
		"capacity_standard_multi_az":              "storage_standard_multi_az_bytes",
		"capacity_standard_single_az":             "storage_standard_single_az_bytes",
		"capacity_infrequent_access_multi_az":     "storage_ia_multi_az_bytes",
		"capacity_infrequent_access_single_az":    "storage_ia_single_az_bytes",
		"capacity_intelligent_tiering_total":      "storage_intelligent_tiering_bytes",
		"capacity_intelligent_tiering_frequent":   "storage_intelligent_tiering_frequent_bytes",
		"capacity_intelligent_tiering_infrequent": "storage_intelligent_tiering_infrequent_bytes",
		"capacity_intelligent_tiering_archive":    "storage_intelligent_tiering_archive_bytes",

		// 对象数量指标
		"object_num_all":                     "object_count",
		"object_num_standard_total":          "object_count_standard",
		"object_num_infrequent_access_total": "object_count_ia",
		"object_num_archive_total":           "object_count_archive",
		"object_num_deep_archive_total":      "object_count_deep_archive",

		// 请求指标
		"get_request_count":             "requests_get",
		"put_request_count":             "requests_put",
		"head_request_count":            "requests_head",
		"request_count_per_second":      "requests_per_second",
		"request_count_get_per_second":  "requests_get_per_second",
		"request_count_put_per_second":  "requests_put_per_second",
		"request_count_head_per_second": "requests_head_per_second",
		"request_count_post_per_second": "requests_post_per_second",

		// 流量指标
		"download_bytes":            "traffic_internet_tx_bytes",
		"upload_bytes":              "traffic_internet_rx_bytes",
		"download_bytes_extranet":   "traffic_internet_tx_bytes",
		"upload_bytes_extranet":     "traffic_internet_rx_bytes",
		"download_bytes_intranet":   "traffic_intranet_tx_bytes",
		"upload_bytes_intranet":     "traffic_intranet_rx_bytes",
		"download_traffic":          "traffic_tx_bps",
		"upload_traffic":            "traffic_rx_bps",
		"download_traffic_extranet": "traffic_internet_tx_bps",
		"upload_traffic_extranet":   "traffic_internet_rx_bps",
		"download_traffic_intranet": "traffic_intranet_tx_bps",
		"upload_traffic_intranet":   "traffic_intranet_rx_bps",
		"cdn_bytes":                 "traffic_cdn_tx_bytes",
		"cdn_traffic":               "traffic_cdn_tx_bps",

		// 延迟指标
		"first_byte_latency":              "latency_first_byte_ms",
		"total_request_latency":           "latency_total_request_ms",
		"download_total_request_latency":  "latency_e2e_get_ms",
		"download_server_request_latency": "latency_server_get_ms",
		"upload_total_request_latency":    "latency_e2e_put_ms",
		"upload_server_request_latency":   "latency_server_put_ms",

		// 成功率指标
		"request_success_rate":   "availability_pct",
		"effective_request_rate": "effective_request_rate_pct",
		"request_break_rate":     "request_break_rate_pct",

		// HTTP 状态码指标
		"request_count_monitor_2XX": "response_2xx_count",
		"request_count_monitor_3XX": "response_3xx_count",
		"request_count_monitor_4XX": "response_4xx_count",
		"request_count_4xx":         "response_4xx_count",

		// 带宽指标
		"download_transfer_rate":               "bandwidth_tx_bps",
		"upload_transfer_rate":                 "bandwidth_rx_bps",
		"standard_request_download_bandwidths": "bandwidth_standard_tx_bps",
		"standard_request_upload_bandwidths":   "bandwidth_standard_rx_bps",
	})

	// 注册指标帮助信息
	metrics.RegisterNamespaceHelp("SYS.OBS", func(metric string) string {
		switch metric {
		case "storage_usage_bytes":
			return " - OBS 存储空间使用量（Bytes）"
		case "storage_standard_bytes":
			return " - OBS 标准存储用量（Bytes）"
		case "storage_ia_bytes":
			return " - OBS 低频存储用量（Bytes）"
		case "storage_archive_bytes":
			return " - OBS 归档存储用量（Bytes）"
		case "object_count":
			return " - OBS 对象总数"
		case "requests_get":
			return " - OBS GET 请求数"
		case "requests_put":
			return " - OBS PUT 请求数"
		case "traffic_internet_tx_bytes":
			return " - OBS 公网出流量（Bytes）"
		case "traffic_internet_rx_bytes":
			return " - OBS 公网入流量（Bytes）"
		case "latency_first_byte_ms":
			return " - OBS 首字节延迟（ms）"
		case "availability_pct":
			return " - OBS 请求成功率（%）"
		case "response_4xx_count":
			return " - OBS 4XX 响应数"
		}
		return " - 华为云 OBS 指标"
	})
}
