package tencent

import (
	"testing"

	"multicloud-exporter/internal/providers/common"
)

// 测试腾讯云 CLB 指标缩放（从 Mbps 转换为 bit/s）
func TestScaleCLBMetric(t *testing.T) {
	if got := scaleCLBMetric("QCE/LB", "VipIntraffic", 1.2); got != 1200000 {
		t.Fatalf("clb in bps: got %f, want 1200000", got)
	}
	if got := scaleCLBMetric("QCE/LB", "VipOuttraffic", 2); got != 2000000 {
		t.Fatalf("clb out bps: got %f, want 2000000", got)
	}
	// 无缩放的指标（如连接数）
	if got := scaleCLBMetric("QCE/LB", "Conn", 3.3); got != 3.3 {
		t.Fatalf("no scale: got %f, want 3.3", got)
	}
}

// 测试腾讯云 BWP 指标缩放（从 Mbps 转换为 bit/s）
// 注意：这些测试验证 ScaleBWPMetricForTest 的缩放逻辑，不涉及采集器方法
func TestScaleBWPMetric(t *testing.T) {
	// InTraffic/OutTraffic 从 Mbps 转换为 bit/s（× 1000000）
	if got := common.ScaleBWPMetricForTest("InTraffic", 1); got != 1000000 {
		t.Fatalf("bwp in bps: got %f, want 1000000", got)
	}
	if got := common.ScaleBWPMetricForTest("OutTraffic", 0.5); got != 500000 {
		t.Fatalf("bwp out bps: got %f, want 500000", got)
	}
	// 包速率指标，无缩放（如 InPkg）
	if got := common.ScaleBWPMetricForTest("InPkg", 10); got != 10 {
		t.Fatalf("no scale pps: got %f, want 10", got)
	}
}
