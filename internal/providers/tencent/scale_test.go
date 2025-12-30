package tencent

import "testing"

func TestScaleCLBMetric(t *testing.T) {
	if scaleCLBMetric("QCE/LB", "VipIntraffic", 1.2) != 1200000 {
		t.Fatalf("clb in bps")
	}
	if scaleCLBMetric("QCE/LB", "VipOuttraffic", 2) != 2000000 {
		t.Fatalf("clb out bps")
	}
	if scaleCLBMetric("QCE/LB", "Conn", 3.3) != 3.3 {
		t.Fatalf("no scale")
	}
}

func TestScaleBWPMetric(t *testing.T) {
	if scaleBWPMetric("InTraffic", 1) != 1000000 {
		t.Fatalf("bwp in bps")
	}
	if scaleBWPMetric("OutTraffic", 0.5) != 500000 {
		t.Fatalf("bwp out bps")
	}
	if scaleBWPMetric("InPkg", 10) != 10 {
		t.Fatalf("no scale pps")
	}
}
