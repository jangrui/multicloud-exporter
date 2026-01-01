package discovery

import (
	"encoding/json"
	"multicloud-exporter/internal/config"
	"os"
)

type MetricMeta struct {
	Provider    string
	Namespace   string
	Name        string
	Unit        string
	Dimensions  []string
	Description string
	Canonical   string
	Similar     []string
}

func AnnotateCanonical(metas []MetricMeta, mapping config.MetricMapping) []MetricMeta {
	for i := range metas {
		m := &metas[i]
		// 遍历所有规范化指标，查找当前云厂商的指标映射
		for newName, entry := range mapping.Canonical {
			// 从 Providers map 中获取当前云厂商的指标定义
			if def, exists := entry.Providers[m.Provider]; exists && def.Metric == m.Name {
				m.Canonical = newName

				// 查找其他云厂商是否有相同的规范化指标（Similar）
				var xs []string
				for provider, providerDef := range entry.Providers {
					if provider != m.Provider && providerDef.Metric != "" {
						xs = append(xs, provider)
					}
				}
				m.Similar = xs
				break
			}
		}
	}
	return metas
}

func GetProductMetricMeta(provider, region, ak, sk, namespace, mappingPath string) ([]MetricMeta, error) {
	var metas []MetricMeta
	var err error
	switch provider {
	case "aliyun":
		metas, err = FetchAliyunMetricMeta(region, ak, sk, namespace)
	case "tencent":
		metas, err = FetchTencentMetricMeta(region, ak, sk, namespace)
	}
	if err != nil {
		return nil, err
	}
	mapping, merr := config.ParseMetricMappings(mappingPath)
	if merr == nil {
		metas = AnnotateCanonical(metas, mapping)
	}
	return metas, nil
}

func ExportMetricMetaJSON(metas []MetricMeta) ([]byte, error) {
	return json.MarshalIndent(metas, "", "  ")
}

func SaveMetricMetaJSON(path string, metas []MetricMeta) error {
	data, err := ExportMetricMetaJSON(metas)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
