package aliyun

import metrics "multicloud-exporter/internal/metrics"

func init() {
    metrics.RegisterNamespacePrefix("acs_bandwidth_package", "bwp")
    metrics.RegisterNamespaceMetricAlias("acs_bandwidth_package", map[string]string{
        "in_bandwidth_utilization":  "in_utilization_pct",
        "out_bandwidth_utilization": "out_utilization_pct",
        "net_rx.rate":               "in_bps",
        "net_tx.rate":               "out_bps",
        "net_rx.Pkgs":               "in_pps",
        "net_tx.Pkgs":               "out_pps",
        "in_ratelimit_drop_pps":     "in_drop_pps",
        "out_ratelimit_drop_pps":    "out_drop_pps",
    })
    metrics.RegisterNamespaceHelp("acs_bandwidth_package", func(metric string) string {
        switch metric {
        case "in_utilization_pct":
            return " - 共享带宽入方向带宽利用率（百分比）"
        case "out_utilization_pct":
            return " - 共享带宽出方向带宽利用率（百分比）"
        case "in_bps":
            return " - 共享带宽入方向带宽速率（bit/s）"
        case "out_bps":
            return " - 共享带宽出方向带宽速率（bit/s）"
        case "in_pps":
            return " - 共享带宽入方向包速率（包/秒）"
        case "out_pps":
            return " - 共享带宽出方向包速率（包/秒）"
        case "in_drop_pps":
            return " - 共享带宽入方向因限速丢弃包速率（包/秒）"
        case "out_drop_pps":
            return " - 共享带宽出方向因限速丢弃包速率（包/秒）"
        }
        return " - 云产品指标"
    })
}

