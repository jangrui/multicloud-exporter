package aliyun

import (
	metrics "multicloud-exporter/internal/metrics"
	"strings"
	"unicode"
)

func camelToSnakeSLB(s string) string {
	var b []rune
	var prev rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b = append(b, '_')
		}
		b = append(b, unicode.ToLower(r))
		prev = r
	}
	return string(b)
}

func canonicalizeSLB(metric string) string {
    m := strings.ReplaceAll(metric, ".", "_")
    m = camelToSnakeSLB(m)
    ml := strings.ToLower(m)
    ml = strings.TrimPrefix(ml, "group_")
    switch ml {
    case "traffic_rxnew":
        return "traffic_rx_bps"
    case "traffic_txnew":
        return "traffic_tx_bps"
    case "traffic_rx_new":
        return "traffic_rx_bps"
    case "traffic_tx_new":
        return "traffic_tx_bps"
    case "drop_traffic_rx":
        return "drop_traffic_rx_bps"
    case "drop_traffic_tx":
        return "drop_traffic_tx_bps"
    case "instance_traffic_rx_utilization":
        return "instance_traffic_rx_utilization_pct"
    case "instance_traffic_tx_utilization":
        return "instance_traffic_tx_utilization_pct"
    case "instance_qps_utilization":
        return "instance_qps_utilization_pct"
    case "instance_max_connection_utilization":
        return "instance_max_connection_utilization_pct"
    }
    return ml
}

func init() {
    metrics.RegisterNamespacePrefix("acs_slb_dashboard", "lb")
    metrics.RegisterNamespaceAliasFunc("acs_slb_dashboard", canonicalizeSLB)
    metrics.RegisterNamespaceHelp("acs_slb_dashboard", func(metric string) string {
        switch metric {
        case "active_connection":
            return " - LB 活跃连接数"
        case "inactive_connection":
            return " - LB 非活跃连接数"
        case "new_connection":
            return " - LB 新建连接数"
        case "max_connection":
            return " - LB 每秒最大并发连接数"
        case "packet_rx":
            return " - LB 入包速率"
        case "packet_tx":
            return " - LB 出包速率"
        case "drop_packet_rx":
            return " - LB 入向丢包速率"
        case "drop_packet_tx":
            return " - LB 出向丢包速率"
        case "traffic_rx_bps":
            return " - LB 入方向带宽（bit/s）"
        case "traffic_tx_bps":
            return " - LB 出方向带宽（bit/s）"
        case "drop_traffic_rx_bps":
            return " - LB 入向丢失带宽（bit/s）"
        case "drop_traffic_tx_bps":
            return " - LB 出向丢失带宽（bit/s）"
        case "qps":
            return " - LB 七层监听 QPS"
        case "rt":
            return " - LB 七层监听 RT"
        case "status_code_2xx":
            return " - LB 七层 2XX 状态码数量"
        case "status_code_3xx":
            return " - LB 七层 3XX 状态码数量"
        case "status_code_4xx":
            return " - LB 七层 4XX 状态码数量"
        case "status_code_5xx":
            return " - LB 七层 5XX 状态码数量"
        case "status_code_other":
            return " - LB 七层其它状态码数量"
        case "unhealthy_server_count":
            return " - LB 后端异常实例数"
        case "healthy_server_count_with_rule":
            return " - LB 七层规则后端健康实例数"
        case "instance_qps":
            return " - LB 七层实例 QPS"
        case "instance_rt":
            return " - LB 七层实例 RT"
        case "instance_packet_rx":
            return " - LB 实例入包速率"
        case "instance_packet_tx":
            return " - LB 实例出包速率"
        case "instance_traffic_rx_utilization_pct":
            return " - LB 实例入向带宽使用率"
        case "instance_traffic_tx_utilization_pct":
            return " - LB 实例出向带宽使用率"
        case "instance_status_code_2xx":
            return " - LB 七层实例 2XX 状态码数量"
        case "instance_status_code_3xx":
            return " - LB 七层实例 3XX 状态码数量"
        case "instance_status_code_4xx":
            return " - LB 七层实例 4XX 状态码数量"
        case "instance_status_code_5xx":
            return " - LB 七层实例 5XX 状态码数量"
        case "instance_status_code_other":
            return " - LB 七层实例其它状态码数量"
        case "instance_upstream_code_4xx":
            return " - LB 七层实例 Upstream 4XX 状态码数量"
        case "instance_upstream_code_5xx":
            return " - LB 七层实例 Upstream 5XX 状态码数量"
        case "instance_upstream_rt":
            return " - LB 七层实例 Upstream RT"
        }
        return " - 云产品指标"
    })
}
