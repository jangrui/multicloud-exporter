package tencent

import (
	"multicloud-exporter/internal/metrics"
	"testing"
)

func TestLBHelp(t *testing.T) {
	// This triggers the help function internally to cover the switch cases in init()
	metrics.NamespaceGauge("QCE/CLB", "traffic_rx_bps")
	metrics.NamespaceGauge("QCE/CLB", "traffic_tx_bps")
	metrics.NamespaceGauge("QCE/CLB", "packet_rx")
	metrics.NamespaceGauge("QCE/CLB", "packet_tx")
	metrics.NamespaceGauge("QCE/CLB", "drop_packet_rx")
	metrics.NamespaceGauge("QCE/CLB", "drop_packet_tx")
	metrics.NamespaceGauge("QCE/CLB", "drop_connection")
	metrics.NamespaceGauge("QCE/CLB", "traffic_rx_utilization_pct")
	metrics.NamespaceGauge("QCE/CLB", "traffic_tx_utilization_pct")
	metrics.NamespaceGauge("QCE/CLB", "vip_new_connection")
	metrics.NamespaceGauge("QCE/CLB", "new_connection")
	metrics.NamespaceGauge("QCE/CLB", "active_connection")
	metrics.NamespaceGauge("QCE/CLB", "unknown")
}
