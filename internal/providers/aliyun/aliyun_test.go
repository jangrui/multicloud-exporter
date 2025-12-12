package aliyun

import (
	"encoding/json"
	"multicloud-exporter/internal/utils"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
)

func TestPickStatisticValue(t *testing.T) {
	// Case 1: float64
	p1 := map[string]interface{}{"Average": 1.5}
	if v := pickStatisticValue(p1, []string{"Average"}); v != 1.5 {
		t.Errorf("expected 1.5, got %f", v)
	}

	// Case 2: int
	p2 := map[string]interface{}{"Average": 10}
	if v := pickStatisticValue(p2, []string{"Average"}); v != 10.0 {
		t.Errorf("expected 10.0, got %f", v)
	}

	// Case 3: json.Number
	p3 := map[string]interface{}{"Average": json.Number("20.5")}
	if v := pickStatisticValue(p3, []string{"Average"}); v != 20.5 {
		t.Errorf("expected 20.5, got %f", v)
	}

	// Case 4: Default order
	p4 := map[string]interface{}{"Maximum": 30.0}
	if v := pickStatisticValue(p4, nil); v != 30.0 {
		t.Errorf("expected 30.0, got %f", v)
	}

	// Case 5: Missing
	p5 := map[string]interface{}{"Other": 1.0}
	if v := pickStatisticValue(p5, []string{"Average"}); v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

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
	if resourceTypeForNamespace("acs_bandwidth_package") != "bwp" {
		t.Fatalf("cbwp")
	}
	if resourceTypeForNamespace("acs_slb_dashboard") != "clb" {
		t.Fatalf("slb")
	}
	if resourceTypeForNamespace("acs_oss_dashboard") != "s3" {
		t.Fatalf("oss")
	}
	if resourceTypeForNamespace("acs_alb") != "alb" {
		t.Fatalf("alb")
	}
	if resourceTypeForNamespace("acs_nlb") != "nlb" {
		t.Fatalf("nlb")
	}
	if resourceTypeForNamespace("acs_gwlb") != "gwlb" {
		t.Fatalf("gwlb")
	}
}

func TestListIDsByCMS_ALB(t *testing.T) {
	mc := &mockCMSClient{
		DescribeMetricListFunc: func(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error) {
			return &cms.DescribeMetricListResponse{Datapoints: `[{"loadBalancerId":"alb-1"},{"loadBalancerId":"alb-2"},{"loadBalancerId":"alb-1"}]`}, nil
		},
	}
	c := &Collector{}
	ids := c.listIDsByCMS(mc, "cn-hangzhou", "acs_alb", "LoadBalancerActiveConnection", "loadBalancerId")
	if len(ids) != 2 {
		t.Fatalf("alb ids expected 2 got %d", len(ids))
	}
}

func TestListIDsByCMS_NLB(t *testing.T) {
	mc := &mockCMSClient{
		DescribeMetricListFunc: func(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error) {
			return &cms.DescribeMetricListResponse{Datapoints: `[{"instanceId":"nlb-1"},{"instanceId":"nlb-2"}]`}, nil
		},
	}
	c := &Collector{}
	ids := c.listIDsByCMS(mc, "cn-hangzhou", "acs_nlb", "InstanceActiveConnection", "instanceId")
	if len(ids) != 2 {
		t.Fatalf("nlb ids expected 2 got %d", len(ids))
	}
}

func TestListIDsByCMS_GWLB(t *testing.T) {
	mc := &mockCMSClient{
		DescribeMetricListFunc: func(request *cms.DescribeMetricListRequest) (response *cms.DescribeMetricListResponse, err error) {
			return &cms.DescribeMetricListResponse{Datapoints: `[{"instanceId":"gw-1"}]`}, nil
		},
	}
	c := &Collector{}
	ids := c.listIDsByCMS(mc, "cn-hangzhou", "acs_gwlb", "ActiveConnection", "instanceId")
	if len(ids) != 1 || ids[0] != "gw-1" {
		t.Fatalf("gwlb ids expected [gw-1] got %v", ids)
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
