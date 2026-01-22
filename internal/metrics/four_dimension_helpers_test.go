package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRecordFourDimensionAccountStatus(t *testing.T) {
	// 清理指标
	AccountStatusTotal.Reset()

	// 记录账号状态
	RecordFourDimensionAccountStatus("test-account", "aliyun", "active")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	AccountStatusTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		// 检查标签值
		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["cloud_provider"] != "aliyun" {
			t.Errorf("Expected cloud_provider label to be 'aliyun', got '%s'", labelMap["cloud_provider"])
		}
		if labelMap["status"] != "active" {
			t.Errorf("Expected status label to be 'active', got '%s'", labelMap["status"])
		}

		// 验证指标值为 1
		if pb.GetGauge().GetValue() != 1 {
			t.Errorf("Expected gauge value to be 1, got %f", pb.GetGauge().GetValue())
		}

		break // 只检查第一个指标
	}
}

func TestRecordFourDimensionProductStatus(t *testing.T) {
	// 清理指标
	ProductStatusTotal.Reset()

	// 记录产品状态
	RecordFourDimensionProductStatus("test-account", "slb", "active")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	ProductStatusTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["product_id"] != "slb" {
			t.Errorf("Expected product_id label to be 'slb', got '%s'", labelMap["product_id"])
		}
		if labelMap["status"] != "active" {
			t.Errorf("Expected status label to be 'active', got '%s'", labelMap["status"])
		}

		if pb.GetGauge().GetValue() != 1 {
			t.Errorf("Expected gauge value to be 1, got %f", pb.GetGauge().GetValue())
		}

		break
	}
}

func TestRecordFourDimensionRegionStatus(t *testing.T) {
	// 清理指标
	RegionStatusTotal.Reset()

	// 记录区域状态
	RecordFourDimensionRegionStatus("test-account", "slb", "cn-hangzhou", "active")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	RegionStatusTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["product_id"] != "slb" {
			t.Errorf("Expected product_id label to be 'slb', got '%s'", labelMap["product_id"])
		}
		if labelMap["region"] != "cn-hangzhou" {
			t.Errorf("Expected region label to be 'cn-hangzhou', got '%s'", labelMap["region"])
		}
		if labelMap["status"] != "active" {
			t.Errorf("Expected status label to be 'active', got '%s'", labelMap["status"])
		}

		if pb.GetGauge().GetValue() != 1 {
			t.Errorf("Expected gauge value to be 1, got %f", pb.GetGauge().GetValue())
		}

		break
	}
}

func TestRecordFourDimensionResourceStatus(t *testing.T) {
	// 清理指标
	ResourceStatusTotal.Reset()

	// 记录资源状态
	RecordFourDimensionResourceStatus("test-account", "slb", "cn-hangzhou", "lb-123", "active")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	ResourceStatusTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["product_id"] != "slb" {
			t.Errorf("Expected product_id label to be 'slb', got '%s'", labelMap["product_id"])
		}
		if labelMap["region"] != "cn-hangzhou" {
			t.Errorf("Expected region label to be 'cn-hangzhou', got '%s'", labelMap["region"])
		}
		if labelMap["resource_id"] != "lb-123" {
			t.Errorf("Expected resource_id label to be 'lb-123', got '%s'", labelMap["resource_id"])
		}
		if labelMap["status"] != "active" {
			t.Errorf("Expected status label to be 'active', got '%s'", labelMap["status"])
		}

		if pb.GetGauge().GetValue() != 1 {
			t.Errorf("Expected gauge value to be 1, got %f", pb.GetGauge().GetValue())
		}

		break
	}
}

func TestRecordFourDimensionAccountSkip(t *testing.T) {
	// 清理指标
	AccountSkipTotal.Reset()

	// 记录账号跳过
	RecordFourDimensionAccountSkip("test-account", "aliyun", "disabled")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	AccountSkipTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["cloud_provider"] != "aliyun" {
			t.Errorf("Expected cloud_provider label to be 'aliyun', got '%s'", labelMap["cloud_provider"])
		}
		if labelMap["reason"] != "disabled" {
			t.Errorf("Expected reason label to be 'disabled', got '%s'", labelMap["reason"])
		}

		if pb.GetCounter().GetValue() != 1 {
			t.Errorf("Expected counter value to be 1, got %f", pb.GetCounter().GetValue())
		}

		break
	}
}

func TestRecordFourDimensionAccountDegraded(t *testing.T) {
	// 清理指标
	AccountDegradedTotal.Reset()

	// 记录账号降级
	RecordFourDimensionAccountDegraded("test-account", "aliyun", "api_limit")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	AccountDegradedTotal.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["cloud_provider"] != "aliyun" {
			t.Errorf("Expected cloud_provider label to be 'aliyun', got '%s'", labelMap["cloud_provider"])
		}
		if labelMap["reason"] != "api_limit" {
			t.Errorf("Expected reason label to be 'api_limit', got '%s'", labelMap["reason"])
		}

		if pb.GetCounter().GetValue() != 1 {
			t.Errorf("Expected counter value to be 1, got %f", pb.GetCounter().GetValue())
		}

		break
	}
}

func TestRecordFourDimensionAccountStatusChange(t *testing.T) {
	// 清理指标
	AccountStatusChange.Reset()

	// 记录账号状态变化
	RecordFourDimensionAccountStatusChange("test-account", "aliyun", "active", "degraded", "api_error")

	// 验证指标是否记录
	metricChan := make(chan prometheus.Metric, 10)
	AccountStatusChange.Collect(metricChan)
	close(metricChan)

	for metric := range metricChan {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Errorf("Failed to write metric: %v", err)
			continue
		}

		labels := pb.GetLabel()
		if labels == nil {
			continue
		}

		labelMap := make(map[string]string)
		for _, label := range labels {
			labelMap[label.GetName()] = label.GetValue()
		}

		if labelMap["account_id"] != "test-account" {
			t.Errorf("Expected account_id label to be 'test-account', got '%s'", labelMap["account_id"])
		}
		if labelMap["cloud_provider"] != "aliyun" {
			t.Errorf("Expected cloud_provider label to be 'aliyun', got '%s'", labelMap["cloud_provider"])
		}
		if labelMap["old_status"] != "active" {
			t.Errorf("Expected old_status label to be 'active', got '%s'", labelMap["old_status"])
		}
		if labelMap["new_status"] != "degraded" {
			t.Errorf("Expected new_status label to be 'degraded', got '%s'", labelMap["new_status"])
		}
		if labelMap["reason"] != "api_error" {
			t.Errorf("Expected reason label to be 'api_error', got '%s'", labelMap["reason"])
		}

		if pb.GetCounter().GetValue() != 1 {
			t.Errorf("Expected counter value to be 1, got %f", pb.GetCounter().GetValue())
		}

		break
	}
}
