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

	// Hardcoded namespaces for now as they are not in the yaml
	// TODO: Support dynamic namespace mapping or multiple files
	aliyunNS := "acs_slb_dashboard"
	tencentNS := "QCE/CLB"

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
	metrics.RegisterNamespacePrefix(aliyunNS, "lb")
	metrics.RegisterNamespaceMetricAlias(aliyunNS, aliyunAliases)
	metrics.RegisterNamespaceMetricScale(aliyunNS, aliyunScales)

	metrics.RegisterNamespacePrefix(tencentNS, "lb")
	metrics.RegisterNamespaceMetricAlias(tencentNS, tencentAliases)
	metrics.RegisterNamespaceMetricScale(tencentNS, tencentScales)

	return nil
}
