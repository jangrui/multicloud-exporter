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
		if m.Provider == "aliyun" {
			for newName, entry := range mapping.Canonical {
				def := entry.Aliyun
				if def.Metric == m.Name {
					m.Canonical = newName
					var xs []string
					if entry.Tencent.Metric != "" {
						xs = append(xs, "tencent")
					}
					m.Similar = xs
					break
				}
			}
		} else if m.Provider == "tencent" {
			for newName, entry := range mapping.Canonical {
				def := entry.Tencent
				if def.Metric == m.Name {
					m.Canonical = newName
					var xs []string
					if entry.Aliyun.Metric != "" {
						xs = append(xs, "aliyun")
					}
					m.Similar = xs
					break
				}
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
