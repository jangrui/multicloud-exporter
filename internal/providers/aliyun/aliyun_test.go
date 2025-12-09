package aliyun

import (
	"multicloud-exporter/internal/utils"
	"testing"
)

func TestChooseDimKeyForNamespace(t *testing.T) {
	if v := chooseDimKeyForNamespace("acs_bandwidth_package", []string{"instance_id"}); v == "" {
		t.Fatalf("cbwp dim")
	}
	if v := chooseDimKeyForNamespace("acs_slb_dashboard", []string{"port", "InstanceId"}); v == "" {
		t.Fatalf("slb dim")
	}
	if v := chooseDimKeyForNamespace("acs_oss_dashboard", []string{"BucketName"}); v == "" {
		t.Fatalf("oss dim")
	}
}

func TestHasAnyDim(t *testing.T) {
	if !hasAnyDim([]string{"a", "b", "c"}, []string{"B"}) {
		t.Fatalf("has")
	}
	if hasAnyDim([]string{"a"}, []string{"x"}) {
		t.Fatalf("no")
	}
}

func TestResourceTypeForNamespace(t *testing.T) {
	if resourceTypeForNamespace("acs_bandwidth_package") != "cbwp" {
		t.Fatalf("cbwp")
	}
	if resourceTypeForNamespace("acs_slb_dashboard") != "slb" {
		t.Fatalf("slb")
	}
	if resourceTypeForNamespace("acs_oss_dashboard") != "oss" {
		t.Fatalf("oss")
	}
}

func TestClassifyAliyunError(t *testing.T) {
	if classifyAliyunError(errStr("InvalidAccessKeyId")) != "auth_error" {
		t.Fatalf("auth")
	}
	if classifyAliyunError(errStr("Throttling")) != "limit_error" {
		t.Fatalf("limit")
	}
	if classifyAliyunError(errStr("InvalidRegionId")) != "region_skip" {
		t.Fatalf("region")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }

func TestAssignRegion(t *testing.T) {
	total := 4
	hits := 0
	for i := 0; i < total; i++ {
		key := "acc" + "|" + "cn-hangzhou"
		if utils.ShouldProcess(key, total, i) {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("assign single shard")
	}
}
