package tencent

import (
	"multicloud-exporter/internal/metrics"
	"testing"
)

func TestCLBHelp(t *testing.T) {
	// This triggers the help function internally to cover the switch cases in init()
	metrics.NamespaceGauge("QCE/LB", "traffic_rx_bps")
	metrics.NamespaceGauge("QCE/LB", "traffic_tx_bps")
	metrics.NamespaceGauge("QCE/LB", "packet_rx")
	metrics.NamespaceGauge("QCE/LB", "packet_tx")
	metrics.NamespaceGauge("QCE/LB", "drop_packet_rx")
	metrics.NamespaceGauge("QCE/LB", "drop_packet_tx")
	metrics.NamespaceGauge("QCE/LB", "drop_connection")
	metrics.NamespaceGauge("QCE/LB", "traffic_rx_utilization_pct")
	metrics.NamespaceGauge("QCE/LB", "traffic_tx_utilization_pct")
	metrics.NamespaceGauge("QCE/LB", "vip_new_connection")
	metrics.NamespaceGauge("QCE/LB", "new_connection")
	metrics.NamespaceGauge("QCE/LB", "active_connection")
	metrics.NamespaceGauge("QCE/LB", "unknown")
}
