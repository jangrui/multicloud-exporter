package tencent

import (
    "testing"
    "multicloud-exporter/internal/config"
)

func TestMinPeriod_PeriodsList(t *testing.T) {
    // Stub response with Periods list
    describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
        return []byte(`{"MetricSet":[{"MetricName":"InTraffic","Periods":[60,300]}]}`), nil
    }
    acc := config.CloudAccount{AccessKeyID: "ak", AccessKeySecret: "sk"}
    p := minPeriodForMetric("ap-guangzhou", acc, "QCE/BWP", "InTraffic")
    if p != 60 { t.Fatalf("expected 60, got %d", p) }
}

func TestMinPeriod_SinglePeriod(t *testing.T) {
    describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
        return []byte(`{"MetricSet":[{"MetricName":"VipOuttraffic","Period":300}]}`), nil
    }
    acc := config.CloudAccount{}
    p := minPeriodForMetric("ap-guangzhou", acc, "QCE/LB", "VipOuttraffic")
    if p != 300 { t.Fatalf("expected 300, got %d", p) }
}

func TestMinPeriod_EmptyFallback(t *testing.T) {
    describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
        return []byte(`{"MetricSet":[]}`), nil
    }
    acc := config.CloudAccount{}
    p := minPeriodForMetric("ap-guangzhou", acc, "QCE/LB", "VipIntraffic")
    if p != 60 { t.Fatalf("expected 60 fallback, got %d", p) }
}
