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
// 使用 map 动态支持所有云厂商，避免硬编码
type CanonicalEntry struct {
	Description string               `yaml:"description"`
	Providers   map[string]MetricDef `yaml:",inline"` // 动态解析所有云厂商配置
}

// MetricMapping 指标映射配置
type MetricMapping struct {
	Prefix     string                    `yaml:"prefix"`
	Namespaces map[string]string         `yaml:"namespaces"`
	Canonical  map[string]CanonicalEntry `yaml:"canonical"`
}

// LoadMetricMappings 加载指标映射配置并注册到 metrics 包
//
// 本函数实现动态云厂商支持，从 YAML 配置中自动发现并注册所有云厂商的指标映射。
// 无需修改代码即可支持新的云厂商，只需在 YAML 配置中添加相应的映射定义。
//
// 工作流程：
//  1. 读取并解析 YAML 配置文件
//  2. 验证必需字段（prefix, namespaces）
//  3. 遍历 mapping.Namespaces 获取所有云厂商
//  4. 对每个云厂商：
//     a. 遍历 mapping.Canonical 获取所有规范化指标
//     b. 从 entry.Providers[provider] 提取云厂商特定配置
//     c. 构建指标别名映射（原生指标名 -> 规范化指标名）
//     d. 构建缩放因子映射（规范化指标名 -> 缩放因子）
//  5. 注册到 metrics 包：
//     a. RegisterNamespacePrefix：注册命名空间前缀
//     b. RegisterNamespaceMetricAlias：注册指标别名映射
//     c. RegisterNamespaceMetricScale：注册缩放因子映射
//
// 扩展方法：
// 要添加新的云厂商（例如 Google GCS），只需在 YAML 配置中：
//  1. 在 namespaces 中添加：google: GCS
//  2. 在 canonical 的每个指标下添加 google 配置：
//     storage_usage_bytes:
//     description: "存储空间使用量"
//     google:
//     metric: storage_total_bytes
//     unit: Bytes
//     scale: 1
//
// 参数：
//   - path: 指标映射配置文件路径（例如：configs/mappings/s3.metrics.yaml）
//
// 返回：
//   - error: 配置文件读取、解析或验证失败时返回错误
//
// 示例：
//
//	err := LoadMetricMappings("configs/mappings/s3.metrics.yaml")
//	if err != nil {
//	    log.Fatalf("加载 S3 指标映射失败: %v", err)
//	}
func LoadMetricMappings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("从 %s 读取指标映射失败: %v", path, err)
	}
	var mapping MetricMapping
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("解析指标映射错误: %v", err)
	}

	// 验证必需字段
	if len(mapping.Namespaces) == 0 {
		return fmt.Errorf("映射文件 %s 中未定义命名空间", path)
	}

	if mapping.Prefix == "" {
		return fmt.Errorf("映射文件 %s 中未定义前缀", path)
	}

	// 动态遍历所有云厂商
	for provider, namespace := range mapping.Namespaces {
		if namespace == "" {
			continue // 跳过空命名空间
		}

		aliases := make(map[string]string)
		scales := make(map[string]float64)

		// 遍历所有规范化指标
		for canonicalName, entry := range mapping.Canonical {
			// 从 Providers map 中获取云厂商特定配置
			if metricDef, exists := entry.Providers[provider]; exists {
				if metricDef.Metric != "" {
					// 注册指标别名：原生指标名 -> 规范化指标名
					aliases[metricDef.Metric] = canonicalName

					// 注册缩放因子（如果非零）
					if metricDef.Scale != 0 {
						scales[canonicalName] = metricDef.Scale
					}
				}
			}
		}

		// 注册到 metrics 包
		metrics.RegisterNamespacePrefix(namespace, mapping.Prefix)
		metrics.RegisterNamespaceMetricAlias(namespace, aliases)
		metrics.RegisterNamespaceMetricScale(namespace, scales)
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
