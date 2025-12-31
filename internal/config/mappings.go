package config

import (
	"fmt"
	"multicloud-exporter/internal/metrics"
	"os"

	"gopkg.in/yaml.v3"
)

// MetricDef 指标定义
type MetricDef struct {
	Metric     string   `yaml:"metric"`
	Unit       string   `yaml:"unit"`
	Scale      float64  `yaml:"scale"`
	Dimensions []string `yaml:"dimensions,omitempty"`
}

// CanonicalEntry 规范化指标条目
type CanonicalEntry struct {
	Description string    `yaml:"description"`
	Aliyun      MetricDef `yaml:"aliyun"`
	Tencent     MetricDef `yaml:"tencent"`
	AWS         MetricDef `yaml:"aws"`
}

// MetricMapping 指标映射配置
type MetricMapping struct {
	Prefix     string                    `yaml:"prefix"`
	Namespaces map[string]string         `yaml:"namespaces"`
	Canonical  map[string]CanonicalEntry `yaml:"canonical"`
}

// LoadMetricMappings 加载指标映射配置并注册到 metrics 包
func LoadMetricMappings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("从 %s 读取指标映射失败: %v", path, err)
	}
	var mapping MetricMapping
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("解析指标映射错误: %v", err)
	}

	// 使用配置的命名空间
	aliyunNS := mapping.Namespaces["aliyun"]
	tencentNS := mapping.Namespaces["tencent"]
	awsNS := mapping.Namespaces["aws"]
	prefix := mapping.Prefix

	if aliyunNS == "" && tencentNS == "" && awsNS == "" {
		return fmt.Errorf("映射文件 %s 中未定义命名空间", path)
	}

	if prefix == "" {
		return fmt.Errorf("映射文件 %s 中未定义前缀", path)
	}

	aliyunAliases := make(map[string]string)
	aliyunScales := make(map[string]float64)
	tencentAliases := make(map[string]string)
	tencentScales := make(map[string]float64)
	awsAliases := make(map[string]string)
	awsScales := make(map[string]float64)

	// 处理规范化指标
	for newName, entry := range mapping.Canonical {
		if entry.Aliyun.Metric != "" {
			aliyunAliases[entry.Aliyun.Metric] = newName
			if entry.Aliyun.Scale != 0 {
				aliyunScales[newName] = entry.Aliyun.Scale  // 使用规范名称作为键
			}
		}
		if entry.Tencent.Metric != "" {
			tencentAliases[entry.Tencent.Metric] = newName
			if entry.Tencent.Scale != 0 {
				tencentScales[newName] = entry.Tencent.Scale  // 使用规范名称作为键
			}
		}
		if entry.AWS.Metric != "" {
			awsAliases[entry.AWS.Metric] = newName
			if entry.AWS.Scale != 0 {
				awsScales[newName] = entry.AWS.Scale  // 使用规范名称作为键
			}
		}
	}

	// 注册到 metrics 包
	if aliyunNS != "" {
		metrics.RegisterNamespacePrefix(aliyunNS, prefix)
		metrics.RegisterNamespaceMetricAlias(aliyunNS, aliyunAliases)
		metrics.RegisterNamespaceMetricScale(aliyunNS, aliyunScales)
	}

	if tencentNS != "" {
		metrics.RegisterNamespacePrefix(tencentNS, prefix)
		metrics.RegisterNamespaceMetricAlias(tencentNS, tencentAliases)
		metrics.RegisterNamespaceMetricScale(tencentNS, tencentScales)
	}

	if awsNS != "" {
		metrics.RegisterNamespacePrefix(awsNS, prefix)
		metrics.RegisterNamespaceMetricAlias(awsNS, awsAliases)
		metrics.RegisterNamespaceMetricScale(awsNS, awsScales)
	}

	return nil
}

// ParseMetricMappings 解析指标映射配置文件
func ParseMetricMappings(path string) (MetricMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MetricMapping{}, fmt.Errorf("从 %s 读取指标映射失败: %v", path, err)
	}
	var mapping MetricMapping
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return MetricMapping{}, fmt.Errorf("解析指标映射错误: %v", err)
	}
	return mapping, nil
}
