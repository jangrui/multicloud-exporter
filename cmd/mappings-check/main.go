package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// 目标：
// - 让 configs/mappings/*.yaml 能自动对照 local/configs/products/<provider>/<product>.yaml 做一致性校验
// - 使用 product-map 把 prefix -> 各云产品文件名/命名空间 的映射显式固化，形成可执行规范

type metricDef struct {
	Metric      string   `yaml:"metric"`
	Dimensions  []string `yaml:"dimensions"`
	Unit        string   `yaml:"unit"`
	Scale       float64  `yaml:"scale"`
	Description string   `yaml:"description"`
}

type canonicalEntry struct {
	Description string    `yaml:"description"`
	Aliyun      metricDef `yaml:"aliyun"`
	Tencent     metricDef `yaml:"tencent"`
	AWS         metricDef `yaml:"aws"`
}

type mappingFile struct {
	Prefix     string                    `yaml:"prefix"`
	Namespaces map[string]string         `yaml:"namespaces"`
	Canonical  map[string]canonicalEntry `yaml:"canonical"`
}

type cwMetric struct {
	Namespace  string `yaml:"Namespace"`
	MetricName string `yaml:"MetricName"`
	Dimensions []struct {
		Name  string `yaml:"Name"`
		Value string `yaml:"Value"`
	} `yaml:"Dimensions"`
}

type aliyunMetric struct {
	Namespace  string `yaml:"Namespace"`
	MetricName string `yaml:"MetricName"`
	Dimensions string `yaml:"Dimensions"`
}

type tencentProducts struct {
	MetricSet []struct {
		Namespace  string `yaml:"Namespace"`
		MetricName string `yaml:"MetricName"`
		Dimensions []struct {
			Dimensions []string `yaml:"Dimensions"`
		} `yaml:"Dimensions"`
	} `yaml:"MetricSet"`
}

type productMap struct {
	// prefix -> provider -> localProductName
	Products map[string]map[string]string `yaml:"products"`
}

func main() {
	var (
		mappingsDir = flag.String("mappings-dir", "configs/mappings", "mappings directory")
		productsDir = flag.String("products-dir", "local/configs/products", "products directory")
		productMapF = flag.String("product-map", "configs/product-map.yaml", "prefix->provider product mapping")
		providers   = flag.String("providers", "aws", "comma separated providers to validate")
	)
	flag.Parse()

	pm, err := loadProductMap(*productMapF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load product-map failed: %v\n", err)
		os.Exit(2)
	}
	wantProviders := splitCSV(*providers)

	files, err := filepath.Glob(filepath.Join(*mappingsDir, "*.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "list mappings failed: %v\n", err)
		os.Exit(2)
	}
	var errs []string
	for _, f := range files {
		for _, p := range wantProviders {
			if e := checkMappingAgainstProducts(f, *productsDir, p, pm); e != nil {
				errs = append(errs, e.Error())
			}
		}
	}
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(errs, "\n"))
		os.Exit(1)
	}
}

func loadProductMap(path string) (*productMap, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pm productMap
	if err := yaml.Unmarshal(bs, &pm); err != nil {
		return nil, err
	}
	return &pm, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func checkMappingAgainstProducts(mappingPath, productsRoot, provider string, pm *productMap) error {
	data, err := os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("read mapping %s: %w", mappingPath, err)
	}
	var mf mappingFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return fmt.Errorf("parse mapping %s: %w", mappingPath, err)
	}
	if mf.Prefix == "" || mf.Namespaces == nil || mf.Canonical == nil {
		return nil
	}
	ns, ok := mf.Namespaces[provider]
	if !ok || strings.TrimSpace(ns) == "" {
		// mapping 未声明该 provider 命名空间，不校验
		return nil
	}

	localName := ""
	if pm != nil && pm.Products != nil {
		if m, ok := pm.Products[mf.Prefix]; ok {
			localName = m[provider]
		}
	}
	if localName == "" {
		// 未配置映射则回退：prefix == local 文件名
		localName = mf.Prefix
	}
	productFile := filepath.Join(productsRoot, provider, localName+".yaml")
	if _, err := os.Stat(productFile); err != nil {
		// 没有离线产品文件就跳过（但不报错，避免阻塞）
		return nil
	}

	pbs, err := os.ReadFile(productFile)
	if err != nil {
		return fmt.Errorf("read products %s: %w", productFile, err)
	}
	observed, err := parseObserved(provider, ns, pbs, productFile)
	if err != nil {
		return err
	}

	var errs []string
	for canonical, entry := range mf.Canonical {
		var def metricDef
		switch provider {
		case "aws":
			def = entry.AWS
		case "aliyun":
			def = entry.Aliyun
		case "tencent":
			def = entry.Tencent
		default:
			continue
		}
		if strings.TrimSpace(def.Metric) == "" {
			continue
		}
		metricName := strings.TrimSpace(def.Metric)
		sets := observed[metricName]
		if len(sets) == 0 {
			errs = append(errs, fmt.Sprintf("%s: %s metric not found in products: canonical=%q metric=%q namespace=%q products=%s",
				mappingPath, provider, canonical, metricName, ns, productFile))
			continue
		}
		required := make(map[string]struct{}, len(def.Dimensions))
		for _, d := range def.Dimensions {
			d = strings.TrimSpace(d)
			if d != "" {
				required[d] = struct{}{}
			}
		}
		// required 为空则不校验维度
		if len(required) > 0 {
			ok := false
			for _, s := range sets {
				if isSubset(required, s) {
					ok = true
					break
				}
			}
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: %s dimensions mismatch: canonical=%q metric=%q required=%v namespace=%q products=%s",
					mappingPath, provider, canonical, metricName, keys(required), ns, productFile))
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func parseObserved(provider, namespace string, pbs []byte, productFile string) (map[string][]map[string]struct{}, error) {
	observed := make(map[string][]map[string]struct{})
	switch provider {
	case "aws":
		var items []cwMetric
		if err := yaml.Unmarshal(pbs, &items); err != nil {
			return nil, fmt.Errorf("parse products %s: %w", productFile, err)
		}
		for _, it := range items {
			if it.Namespace != namespace || strings.TrimSpace(it.MetricName) == "" {
				continue
			}
			set := make(map[string]struct{}, len(it.Dimensions))
			for _, d := range it.Dimensions {
				n := strings.TrimSpace(d.Name)
				if n != "" {
					set[n] = struct{}{}
				}
			}
			observed[it.MetricName] = append(observed[it.MetricName], set)
		}
	case "aliyun":
		var items []aliyunMetric
		if err := yaml.Unmarshal(pbs, &items); err != nil {
			return nil, fmt.Errorf("parse products %s: %w", productFile, err)
		}
		for _, it := range items {
			if it.Namespace != namespace || strings.TrimSpace(it.MetricName) == "" {
				continue
			}
			set := make(map[string]struct{})
			for _, d := range strings.Split(it.Dimensions, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					set[d] = struct{}{}
				}
			}
			observed[it.MetricName] = append(observed[it.MetricName], set)
		}
	case "tencent":
		var tp tencentProducts
		if err := yaml.Unmarshal(pbs, &tp); err != nil {
			return nil, fmt.Errorf("parse products %s: %w", productFile, err)
		}
		for _, it := range tp.MetricSet {
			if it.Namespace != namespace || strings.TrimSpace(it.MetricName) == "" {
				continue
			}
			for _, dd := range it.Dimensions {
				set := make(map[string]struct{})
				for _, d := range dd.Dimensions {
					d = strings.TrimSpace(d)
					if d != "" {
						set[d] = struct{}{}
					}
				}
				if len(set) > 0 {
					observed[it.MetricName] = append(observed[it.MetricName], set)
				}
			}
		}
	}
	return observed, nil
}

func isSubset(need, have map[string]struct{}) bool {
	for k := range need {
		if _, ok := have[k]; !ok {
			return false
		}
	}
	return true
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
