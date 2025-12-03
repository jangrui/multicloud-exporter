package aliyun

import (
    metrics "multicloud-exporter/internal/metrics"
    "strings"
    "unicode"
)

func camelToSnake(s string) string {
    var b []rune
    var prev rune
    for i, r := range s {
        if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
            b = append(b, '_')
        }
        b = append(b, unicode.ToLower(r))
        prev = r
    }
    out := string(b)
    out = strings.ReplaceAll(out, "_b_p_s", "_bps")
    out = strings.ReplaceAll(out, "_i_o_p_s", "_iops")
    return out
}

func canonicalizeECS(metric string) string {
    m := strings.ReplaceAll(metric, ".", "_")
    m = camelToSnake(m)
    ml := strings.ToLower(m)
    ml = strings.TrimPrefix(ml, "group")
    ml = strings.TrimPrefix(ml, "_")
    ml = strings.TrimPrefix(ml, "vm_")
    ml = strings.TrimPrefix(ml, "vpc_publicip_")
    ml = strings.TrimPrefix(ml, "eip_")
    switch ml {
    case "cpuutilization":
        return "cpu_utilization_pct"
    case "memoryutilization", "memoryusedutilization":
        return "memory_utilization_pct"
    case "load", "load_average":
        return "load"
    case "load_1m":
        return "load_1m"
    case "load_5m":
        return "load_5m"
    case "load_15m":
        return "load_15m"
    case "internetinrate", "intranetinrate":
        return "network_in_bps"
    case "internetoutrate", "intranetoutrate":
        return "network_out_bps"
    case "internetoutrate_percent":
        return "network_out_utilization_pct"
    case "networkin_rate":
        return "network_in_bps"
    case "networkout_rate":
        return "network_out_bps"
    case "networkin_packages":
        return "network_in_pps"
    case "networkout_packages":
        return "network_out_pps"
    case "disk_readbps", "diskreadbps":
        return "disk_read_bps"
    case "disk_writebps", "diskwritebps":
        return "disk_write_bps"
    case "disk_readiops", "diskreadiops":
        return "disk_read_iops"
    case "disk_writeiops", "diskwriteiops":
        return "disk_write_iops"
    case "diskreadbpsutilization":
        return "disk_read_bps_utilization_pct"
    case "diskwritebpsutilization":
        return "disk_write_bps_utilization_pct"
    case "diskreadwritebpsutilization":
        return "disk_rw_bps_utilization_pct"
    case "diskreadwriteiopsutilization":
        return "disk_rw_iops_utilization_pct"
    case "net_tcpconnection", "concurrentconnections", "tcpcount":
        return "tcp_connections"
    case "networkin_droppackages_percent":
        return "network_in_drop_pct"
    case "networkout_droppackages_percent":
        return "network_out_drop_pct"
    }
    return ml
}

func init() {
    metrics.RegisterNamespacePrefix("acs_ecs_dashboard", "ecs")
    metrics.RegisterNamespaceAliasFunc("acs_ecs_dashboard", canonicalizeECS)
    metrics.RegisterNamespaceHelp("acs_ecs_dashboard", func(metric string) string {
        switch metric {
        case "cpu_utilization_pct":
            return " - ECS CPU 利用率（百分比）"
        case "memory_utilization_pct":
            return " - ECS 内存利用率（百分比）"
        case "load":
            return " - ECS 系统负载"
        case "load_1m":
            return " - ECS 系统负载（1分钟）"
        case "load_5m":
            return " - ECS 系统负载（5分钟）"
        case "load_15m":
            return " - ECS 系统负载（15分钟）"
        case "network_in_bps":
            return " - ECS 入方向网络速率（bit/s）"
        case "network_out_bps":
            return " - ECS 出方向网络速率（bit/s）"
        case "network_in_pps":
            return " - ECS 入方向包速率（包/秒）"
        case "network_out_pps":
            return " - ECS 出方向包速率（包/秒）"
        case "network_out_utilization_pct":
            return " - ECS 出方向带宽利用率（百分比）"
        case "disk_read_bps":
            return " - ECS 磁盘读带宽（Byte/s）"
        case "disk_write_bps":
            return " - ECS 磁盘写带宽（Byte/s）"
        case "disk_read_iops":
            return " - ECS 磁盘读 IOPS"
        case "disk_write_iops":
            return " - ECS 磁盘写 IOPS"
        case "disk_read_bps_utilization_pct":
            return " - ECS 磁盘读带宽利用率（百分比）"
        case "disk_write_bps_utilization_pct":
            return " - ECS 磁盘写带宽利用率（百分比）"
        case "disk_rw_bps_utilization_pct":
            return " - ECS 磁盘读写带宽利用率（百分比）"
        case "disk_rw_iops_utilization_pct":
            return " - ECS 磁盘读写 IOPS 利用率（百分比）"
        case "tcp_connections":
            return " - ECS 并发 TCP 连接数"
        case "network_in_drop_pct":
            return " - ECS 入方向丢包率（百分比）"
        case "network_out_drop_pct":
            return " - ECS 出方向丢包率（百分比）"
        }
        return " - 云产品指标"
    })
}
