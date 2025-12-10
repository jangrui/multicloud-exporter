package config

import (
	"fmt"
	"multicloud-exporter/internal/metrics"
	"os"

	"gopkg.in/yaml.v3"
)

type MetricDef struct {
	Metric     string   `yaml:"metric"`
	Unit       string   `yaml:"unit"`
	Scale      float64  `yaml:"scale"`
	Dimensions []string `yaml:"dimensions,omitempty"`
}

type MetricMapping struct {
    Prefix      string                          `yaml:"prefix"`
    Namespaces  map[string]string               `yaml:"namespaces"`
    Canonical   map[string]map[string]MetricDef `yaml:"canonical"`
    AliyunOnly  map[string]MetricDef            `yaml:"aliyun_only"`
    TencentOnly map[string]MetricDef            `yaml:"tencent_only"`
}

func LoadMetricMappings(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("failed to read metric mappings from %s: %v", path, err)
    }
    var mapping MetricMapping
    if err := yaml.Unmarshal(data, &mapping); err != nil {
        return fmt.Errorf("error parsing metric mappings: %v", err)
    }

	// Use configured namespaces
	aliyunNS := mapping.Namespaces["aliyun"]
	tencentNS := mapping.Namespaces["tencent"]
	prefix := mapping.Prefix

	if aliyunNS == "" && tencentNS == "" {
		return fmt.Errorf("no namespaces defined in mapping file %s", path)
	}

	if prefix == "" {
		return fmt.Errorf("no prefix defined in mapping file %s", path)
	}

	aliyunAliases := make(map[string]string)
	aliyunScales := make(map[string]float64)
	tencentAliases := make(map[string]string)
	tencentScales := make(map[string]float64)

	// Canonical
	for newName, providers := range mapping.Canonical {
		if def, ok := providers["aliyun"]; ok && def.Metric != "" {
			aliyunAliases[def.Metric] = newName
			if def.Scale != 0 {
				aliyunScales[def.Metric] = def.Scale
			}
		}
		if def, ok := providers["tencent"]; ok && def.Metric != "" {
			tencentAliases[def.Metric] = newName
			if def.Scale != 0 {
				tencentScales[def.Metric] = def.Scale
			}
		}
	}

	// Aliyun Only
	for newName, def := range mapping.AliyunOnly {
		if def.Metric != "" {
			aliyunAliases[def.Metric] = newName
			if def.Scale != 0 {
				aliyunScales[def.Metric] = def.Scale
			}
		}
	}

	// Tencent Only
	for newName, def := range mapping.TencentOnly {
		if def.Metric != "" {
			tencentAliases[def.Metric] = newName
			if def.Scale != 0 {
				tencentScales[def.Metric] = def.Scale
			}
		}
	}

	// Register
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

    return nil
}

func ParseMetricMappings(path string) (MetricMapping, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return MetricMapping{}, fmt.Errorf("failed to read metric mappings from %s: %v", path, err)
    }
    var mapping MetricMapping
    if err := yaml.Unmarshal(data, &mapping); err != nil {
        return MetricMapping{}, fmt.Errorf("error parsing metric mappings: %v", err)
    }
    return mapping, nil
}
