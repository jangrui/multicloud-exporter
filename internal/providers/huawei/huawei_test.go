package huawei

import (
    "testing"
    "multicloud-exporter/internal/config"
)

func TestHuaweiCollectSwitches(t *testing.T) {
    h := NewCollector()
    acc := config.CloudAccount{AccountID: "x", Regions: []string{"cn-north-4"}, Resources: []string{"rds","redis","elb","eip","unknown"}}
    h.Collect(acc)
}
