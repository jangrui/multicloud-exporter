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
    case "traffic_rx_new":
        return "traffic_rx_bps"
    case "traffic_tx_new":
        return "traffic_tx_bps"
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
    metrics.RegisterNamespacePrefix("acs_slb_dashboard", "slb")
    metrics.RegisterNamespaceAliasFunc("acs_slb_dashboard", canonicalizeSLB)
    metrics.RegisterNamespaceHelp("acs_slb_dashboard", func(metric string) string {
        switch metric {
        case "active_connection":
            return " - SLB 活跃连接数"
        case "inactive_connection":
            return " - SLB 非活跃连接数"
        case "packet_rx":
            return " - SLB 入包速率"
        case "packet_tx":
            return " - SLB 出包速率"
        case "traffic_rx_bps":
            return " - SLB 入方向带宽（bit/s）"
        case "traffic_tx_bps":
            return " - SLB 出方向带宽（bit/s）"
        case "qps":
            return " - SLB 七层监听 QPS"
        case "rt":
            return " - SLB 七层监听 RT"
        case "status_code_2xx":
            return " - SLB 七层 2XX 状态码数量"
        case "status_code_3xx":
            return " - SLB 七层 3XX 状态码数量"
        case "status_code_4xx":
            return " - SLB 七层 4XX 状态码数量"
        case "status_code_5xx":
            return " - SLB 七层 5XX 状态码数量"
        case "unhealthy_server_count":
            return " - SLB 后端异常实例数"
        case "healthy_server_count_with_rule":
            return " - SLB 七层规则后端健康实例数"
        case "instance_qps":
            return " - SLB 七层实例 QPS"
        case "instance_rt":
            return " - SLB 七层实例 RT"
        case "instance_packet_rx":
            return " - SLB 实例入包速率"
        case "instance_packet_tx":
            return " - SLB 实例出包速率"
        case "instance_traffic_rx_utilization_pct":
            return " - SLB 实例入向带宽使用率"
        case "instance_traffic_tx_utilization_pct":
            return " - SLB 实例出向带宽使用率"
        }
        return " - 云产品指标"
    })
}

