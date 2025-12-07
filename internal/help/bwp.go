package help

func BWPHelp(metric string) string {
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
		return " - 共享带宽入方向丢包速率（包/秒）"
	case "out_drop_pps":
		return " - 共享带宽出方向丢包速率（包/秒）"
	}
	return " - 云产品指标"
}
