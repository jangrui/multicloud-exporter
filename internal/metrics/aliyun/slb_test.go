package aliyun

import (
	"multicloud-exporter/internal/metrics"
	"testing"
)

func TestCamelToSnakeSLB(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TestMetric", "test_metric"},
		{"StatusCode2xx", "status_code2xx"},
		{"InstanceStatusCode4xx", "instance_status_code4xx"},
	}
	for _, tt := range tests {
		if got := camelToSnakeSLB(tt.input); got != tt.expected {
			t.Errorf("camelToSnakeSLB(%s) = %s; want %s", tt.input, got, tt.expected)
		}
	}
}

func TestCanonicalizeSLB(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TrafficRXNew", "traffic_rx_bps"},
		{"TrafficTXNew", "traffic_tx_bps"},
		{"TrafficRxNew", "traffic_rx_bps"},
		{"TrafficTxNew", "traffic_tx_bps"},
		{"DropTrafficRx", "drop_traffic_rx_bps"},
		{"DropTrafficTx", "drop_traffic_tx_bps"},
		{"StatusCode2xx", "status_code_2xx"},
		{"StatusCode3xx", "status_code_3xx"},
		{"StatusCode4xx", "status_code_4xx"},
		{"StatusCode5xx", "status_code_5xx"},
		{"StatusCodeOther", "status_code_other"},
		{"InstanceStatusCode2xx", "instance_status_code_2xx"},
		{"InstanceStatusCode3xx", "instance_status_code_3xx"},
		{"InstanceStatusCode4xx", "instance_status_code_4xx"},
		{"InstanceStatusCode5xx", "instance_status_code_5xx"},
		{"InstanceStatusCodeOther", "instance_status_code_other"},
		{"InstanceUpstreamCode4xx", "instance_upstream_code_4xx"},
		{"InstanceUpstreamCode5xx", "instance_upstream_code_5xx"},
		{"InstanceTrafficRxUtilization", "instance_traffic_rx_utilization_pct"},
		{"InstanceTrafficTxUtilization", "instance_traffic_tx_utilization_pct"},
		{"InstanceQpsUtilization", "instance_qps_utilization_pct"},
		{"InstanceMaxConnectionUtilization", "instance_max_connection_utilization_pct"},
		{"UnknownMetric", "unknown_metric"},
		{"Group_Test", "test"},
		{"active_connection", "active_connection"},
	}
	for _, tt := range tests {
		if got := canonicalizeSLB(tt.input); got != tt.expected {
			t.Errorf("canonicalizeSLB(%s) = %s; want %s", tt.input, got, tt.expected)
		}
	}
}

func TestSLBHelp(t *testing.T) {
	metrics.Reset()
	ns := "acs_slb_dashboard"
	cases := []string{
		"active_connection", "inactive_connection", "new_connection", "max_connection", "drop_connection",
		"packet_rx", "packet_tx", "drop_packet_rx", "drop_packet_tx",
		"traffic_rx_bps", "traffic_tx_bps", "drop_traffic_rx_bps", "drop_traffic_tx_bps",
		"qps", "rt",
		"status_code_2xx", "status_code_3xx", "status_code_4xx", "status_code_5xx", "status_code_other",
		"unhealthy_server_count", "healthy_server_count_with_rule",
		"instance_qps", "instance_rt", "instance_packet_rx", "instance_packet_tx",
		"instance_traffic_rx_utilization_pct", "instance_traffic_tx_utilization_pct",
		"instance_status_code_2xx", "instance_status_code_3xx", "instance_status_code_4xx", "instance_status_code_5xx", "instance_status_code_other",
		"instance_upstream_code_4xx", "instance_upstream_code_5xx", "instance_upstream_rt",
		"unknown",
	}
	for _, c := range cases {
		g, count := metrics.NamespaceGauge(ns, c)
		if g == nil {
			t.Errorf("expected gauge for %s, got nil", c)
		}
		if count < 8 {
			t.Errorf("expected at least 8 labels for %s, got %d", c, count)
		}
	}
}
