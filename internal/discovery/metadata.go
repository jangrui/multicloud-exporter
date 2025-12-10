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
			for newName, providers := range mapping.Canonical {
				def, ok := providers["aliyun"]
				if ok && def.Metric == m.Name {
					m.Canonical = newName
					var xs []string
					for p, d := range providers {
						if p != "aliyun" && d.Metric != "" {
							xs = append(xs, p)
						}
					}
					m.Similar = xs
					break
				}
			}
			if m.Canonical == "" {
				for newName, def := range mapping.AliyunOnly {
					if def.Metric == m.Name {
						m.Canonical = newName
						break
					}
				}
			}
		} else if m.Provider == "tencent" {
			for newName, providers := range mapping.Canonical {
				def, ok := providers["tencent"]
				if ok && def.Metric == m.Name {
					m.Canonical = newName
					var xs []string
					for p, d := range providers {
						if p != "tencent" && d.Metric != "" {
							xs = append(xs, p)
						}
					}
					m.Similar = xs
					break
				}
			}
			if m.Canonical == "" {
				for newName, def := range mapping.TencentOnly {
					if def.Metric == m.Name {
						m.Canonical = newName
						break
					}
				}
			}
		}
	}
	return metas
}

func GetProductMetricMeta(provider, region, ak, sk, namespace, mappingPath string) ([]MetricMeta, error) {
	var metas []MetricMeta
	var err error
	if provider == "aliyun" {
		metas, err = FetchAliyunMetricMeta(region, ak, sk, namespace)
	} else if provider == "tencent" {
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
