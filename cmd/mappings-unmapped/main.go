package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// 目的：
// - 在“Option 1”策略下，将无法/不适合统一映射的厂商原始指标纳入暂存区（configs/unmapped）
// - 从离线 products 快照 + mapping 文件自动生成“未覆盖指标清单”
// - 输出只包含维度名称，不包含具体维度值（避免泄露资源信息）

type mappingFile struct {
	Prefix     string            `yaml:"prefix"`
	Namespaces map[string]string `yaml:"namespaces"`
	Canonical  map[string]struct {
		AWS struct {
			Metric string `yaml:"metric"`
		} `yaml:"aws"`
	} `yaml:"canonical"`
}

type cwMetric struct {
	Namespace  string `yaml:"Namespace"`
	MetricName string `yaml:"MetricName"`
	Dimensions []struct {
		Name  string `yaml:"Name"`
		Value string `yaml:"Value"`
	} `yaml:"Dimensions"`
}

type unmappedEntry struct {
	MetricName  string   `yaml:"metric"`
	Dimensions  []string `yaml:"dimensions,omitempty"`
	Stability   string   `yaml:"stability,omitempty"`
	Reason      string   `yaml:"reason,omitempty"`
	SampleCount int      `yaml:"sample_count,omitempty"`
}

type unmappedFile struct {
	Provider  string          `yaml:"provider"`
	Prefix    string          `yaml:"prefix"`
	Namespace string          `yaml:"namespace"`
	Unmapped  []unmappedEntry `yaml:"unmapped"`
}

func main() {
	var (
		provider     = flag.String("provider", "aws", "provider (currently only aws)")
		prefix       = flag.String("prefix", "s3", "mapping prefix (e.g. s3)")
		productsRoot = flag.String("products-root", "local/configs/products", "products root dir")
		mappingPath  = flag.String("mapping", "configs/mappings/s3.metrics.yaml", "mapping file path")
		outPath      = flag.String("out", "configs/unmapped/s3.aws.yaml", "output file path")
		rulesPath    = flag.String("rules", "configs/unmapped/rules.yaml", "classification rules path")
	)
	flag.Parse()

	if *provider != "aws" {
		fmt.Fprintf(os.Stderr, "only provider=aws is supported currently\n")
		os.Exit(2)
	}

	mbs, err := os.ReadFile(*mappingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read mapping failed: %v\n", err)
		os.Exit(2)
	}
	var mf mappingFile
	if err := yaml.Unmarshal(mbs, &mf); err != nil {
		fmt.Fprintf(os.Stderr, "parse mapping failed: %v\n", err)
		os.Exit(2)
	}
	ns := mf.Namespaces[*provider]
	if strings.TrimSpace(ns) == "" {
		fmt.Fprintf(os.Stderr, "mapping has no namespaces.%s\n", *provider)
		os.Exit(2)
	}

	classifier := &classifier{provider: *provider, namespace: ns}
	if bs, err := os.ReadFile(*rulesPath); err == nil {
		if r, err := parseRules(bs); err == nil {
			classifier.rules = r.Rules
		}
	}

	// 已映射的原始指标集合（按 metric name）
	mapped := make(map[string]struct{})
	for _, c := range mf.Canonical {
		if strings.TrimSpace(c.AWS.Metric) != "" {
			mapped[strings.TrimSpace(c.AWS.Metric)] = struct{}{}
		}
	}

	productFile := fmt.Sprintf("%s/%s/%s.yaml", strings.TrimRight(*productsRoot, "/"), *provider, *prefix)
	pbs, err := os.ReadFile(productFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read products failed: %v\n", err)
		os.Exit(2)
	}
	var items []cwMetric
	if err := yaml.Unmarshal(pbs, &items); err != nil {
		fmt.Fprintf(os.Stderr, "parse products failed: %v\n", err)
		os.Exit(2)
	}

	// metric -> (dimsKey -> sampleCount)
	type dimInfo struct {
		dims  []string
		count int
	}
	unmappedByMetric := make(map[string]map[string]*dimInfo)
	for _, it := range items {
		if it.Namespace != ns || strings.TrimSpace(it.MetricName) == "" {
			continue
		}
		mn := strings.TrimSpace(it.MetricName)
		if _, ok := mapped[mn]; ok {
			continue
		}
		var dims []string
		seen := make(map[string]struct{})
		for _, d := range it.Dimensions {
			n := strings.TrimSpace(d.Name)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			dims = append(dims, n)
		}
		sort.Strings(dims)
		key := strings.Join(dims, ",")
		if unmappedByMetric[mn] == nil {
			unmappedByMetric[mn] = make(map[string]*dimInfo)
		}
		if unmappedByMetric[mn][key] == nil {
			unmappedByMetric[mn][key] = &dimInfo{dims: dims}
		}
		unmappedByMetric[mn][key].count++
	}

	var out unmappedFile
	out.Provider = *provider
	out.Prefix = *prefix
	out.Namespace = ns

	// 稳定输出：按 metricName 排序
	metricNames := make([]string, 0, len(unmappedByMetric))
	for mn := range unmappedByMetric {
		metricNames = append(metricNames, mn)
	}
	sort.Strings(metricNames)
	for _, mn := range metricNames {
		dimsKeys := make([]string, 0, len(unmappedByMetric[mn]))
		for k := range unmappedByMetric[mn] {
			dimsKeys = append(dimsKeys, k)
		}
		sort.Strings(dimsKeys)
		for _, dk := range dimsKeys {
			info := unmappedByMetric[mn][dk]
			stab, reason := classifier.classify(mn, info.dims)
			out.Unmapped = append(out.Unmapped, unmappedEntry{
				MetricName:  mn,
				Dimensions:  info.dims,
				Stability:   stab,
				Reason:      reason,
				SampleCount: info.count,
			})
		}
	}

	obs, err := yaml.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal output failed: %v\n", err)
		os.Exit(2)
	}
	if err := os.WriteFile(*outPath, obs, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output failed: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("wrote %s\n", *outPath)
}

type rulesFile struct {
	Rules []rule `yaml:"rules"`
}

type rule struct {
	Match ruleMatch `yaml:"match"`
	Set   ruleSet   `yaml:"set"`
}

type ruleMatch struct {
	Provider      string   `yaml:"provider"`
	Namespace     string   `yaml:"namespace"`
	Metric        string   `yaml:"metric"`
	MetricRegex   string   `yaml:"metric_regex"`
	HasDimensions []string `yaml:"has_dimensions"`
}

type ruleSet struct {
	Stability string `yaml:"stability"`
	Reason    string `yaml:"reason"`
}

func parseRules(bs []byte) (rulesFile, error) {
	var rf rulesFile
	if err := yaml.Unmarshal(bs, &rf); err != nil {
		return rulesFile{}, err
	}
	return rf, nil
}

type classifier struct {
	provider  string
	namespace string
	rules     []rule
}

func (c *classifier) classify(metric string, dims []string) (string, string) {
	for _, r := range c.rules {
		if !c.match(r.Match, metric, dims) {
			continue
		}
		return defaultStr(r.Set.Stability, "experimental"), defaultStr(r.Set.Reason, "")
	}
	// fallback（兼容无规则文件/读取失败）
	return "experimental", "暂未纳入统一映射：需评估跨云口径/单位/维度一致性"
}

func (c *classifier) match(m ruleMatch, metric string, dims []string) bool {
	if m.Provider != "" && m.Provider != c.provider {
		return false
	}
	if m.Namespace != "" && m.Namespace != c.namespace {
		return false
	}
	if m.Metric != "" && m.Metric != metric {
		return false
	}
	if m.MetricRegex != "" {
		re, err := regexp.Compile(m.MetricRegex)
		if err != nil || !re.MatchString(metric) {
			return false
		}
	}
	if len(m.HasDimensions) > 0 {
		set := make(map[string]struct{}, len(dims))
		for _, d := range dims {
			set[d] = struct{}{}
		}
		for _, need := range m.HasDimensions {
			if _, ok := set[need]; !ok {
				return false
			}
		}
	}
	return true
}

func defaultStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
