// 华为云 ELB 指标别名注册
package huawei

import (
	"multicloud-exporter/internal/metrics"
)

func init() {
	// 注册命名空间前缀：SYS.ELB -> clb
	metrics.RegisterNamespacePrefix("SYS.ELB", "clb")

	// 注册指标别名映射（华为云指标名 -> 统一指标名）
	metrics.RegisterNamespaceMetricAlias("SYS.ELB", map[string]string{
		// 流量指标
		"m7_in_Bps":         "traffic_rx_bps",
		"m8_out_Bps":        "traffic_tx_bps",
		"m22_in_bandwidth":  "traffic_rx_bps",
		"m23_out_bandwidth": "traffic_tx_bps",

		// 包速率指标
		"m5_in_pps":  "packet_rx",
		"m6_out_pps": "packet_tx",

		// 连接数指标
		"m2_act_conn":   "active_connection",
		"m3_inact_conn": "inactive_connection",
		"m1_cps":        "new_connection",
		"m4_ncps":       "new_connection",

		// 健康检查指标
		"ma_normal_servers":   "healthy_server_count",
		"m9_abnormal_servers": "unhealthy_server_count",

		// L7 指标
		"mb_l7_qps":          "qps",
		"m14_l7_rt":          "rt",
		"m17_l7_upstream_rt": "upstream_rt",
		"m1c_l7_rt_max":      "rt_max",
		"m1d_l7_rt_min":      "rt_min",

		// HTTP 状态码指标
		"mc_l7_http_2xx":           "status_code_2xx",
		"md_l7_http_3xx":           "status_code_3xx",
		"me_l7_http_4xx":           "status_code_4xx",
		"mf_l7_http_5xx":           "status_code_5xx",
		"m10_l7_http_other_status": "status_code_other",
		"m11_l7_http_404":          "status_code_404",
		"m12_l7_http_499":          "status_code_499",
		"m13_l7_http_502":          "status_code_502",

		// 上游错误指标
		"m15_l7_upstream_4xx": "upstream_code_4xx",
		"m16_l7_upstream_5xx": "upstream_code_5xx",

		// 其他指标
		"m21_client_rps":         "client_rps",
		"m1e_server_rps":         "server_rps",
		"m1f_lvs_rps":            "lvs_rps",
		"m1a_l7_upstream_rt_max": "upstream_rt_max",
		"m1b_l7_upstream_rt_min": "upstream_rt_min",
	})

	// 注册指标帮助信息
	metrics.RegisterNamespaceHelp("SYS.ELB", func(metric string) string {
		switch metric {
		case "traffic_rx_bps":
			return " - ELB 入方向流量（Bytes/s）"
		case "traffic_tx_bps":
			return " - ELB 出方向流量（Bytes/s）"
		case "packet_rx":
			return " - ELB 入方向包速率（count/s）"
		case "packet_tx":
			return " - ELB 出方向包速率（count/s）"
		case "active_connection":
			return " - ELB 活跃连接数"
		case "inactive_connection":
			return " - ELB 非活跃连接数"
		case "new_connection":
			return " - ELB 新建连接数（count/s）"
		case "healthy_server_count":
			return " - ELB 健康后端服务数"
		case "unhealthy_server_count":
			return " - ELB 异常后端服务数"
		case "qps":
			return " - ELB L7 每秒请求数"
		case "rt":
			return " - ELB L7 响应时间（ms）"
		case "status_code_2xx":
			return " - ELB L7 HTTP 2XX 状态码数量"
		case "status_code_3xx":
			return " - ELB L7 HTTP 3XX 状态码数量"
		case "status_code_4xx":
			return " - ELB L7 HTTP 4XX 状态码数量"
		case "status_code_5xx":
			return " - ELB L7 HTTP 5XX 状态码数量"
		case "upstream_code_4xx":
			return " - ELB 上游 HTTP 4XX 状态码数量"
		case "upstream_code_5xx":
			return " - ELB 上游 HTTP 5XX 状态码数量"
		}
		return " - 华为云 ELB 指标"
	})
}
