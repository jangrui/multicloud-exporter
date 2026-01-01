package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// mapping 文件语法校验工具
// 只检查 YAML 文件的语法正确性和基本结构，不依赖 products 文件

type metricDef struct {
	Metric      string   `yaml:"metric"`
	Dimensions  []string `yaml:"dimensions"`
	Unit        string   `yaml:"unit"`
	Scale       float64  `yaml:"scale"`
	Description string   `yaml:"description"`
}

type canonicalEntry struct {
	Description string               `yaml:"description"`
	Providers   map[string]metricDef `yaml:",inline"` // 动态解析所有云厂商配置
}

type mappingFile struct {
	Prefix     string                    `yaml:"prefix"`
	Namespaces map[string]string         `yaml:"namespaces"`
	Canonical  map[string]canonicalEntry `yaml:"canonical"`
}

func main() {
	var (
		mappingsDir = flag.String("mappings-dir", "configs/mappings", "mappings directory")
	)
	flag.Parse()

	files, err := filepath.Glob(filepath.Join(*mappingsDir, "*.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出映射文件失败: %v\n", err)
		os.Exit(2)
	}

	var errs []string
	for _, f := range files {
		if e := validateMappingFile(f); e != nil {
			errs = append(errs, e.Error())
		} else {
			fmt.Printf("✓ %s\n", filepath.Base(f))
		}
	}

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(errs, "\n"))
		os.Exit(1)
	}
	fmt.Printf("\n✓ 所有映射文件验证通过，共检查 %d 个文件\n", len(files))
}

func validateMappingFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取映射文件失败 %s: %w", path, err)
	}

	var mf mappingFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return fmt.Errorf("解析 YAML 失败 %s: %w", path, err)
	}

	// 基本结构校验
	if mf.Prefix == "" {
		return fmt.Errorf("%s: 缺少 prefix 字段", path)
	}

	if len(mf.Namespaces) == 0 {
		return fmt.Errorf("%s: namespaces 为空", path)
	}

	if len(mf.Canonical) == 0 {
		return fmt.Errorf("%s: canonical 映射为空", path)
	}

	// 检查是否有重复的规范名称
	dupCheck := make(map[string]bool)
	for canonical := range mf.Canonical {
		if dupCheck[canonical] {
			return fmt.Errorf("%s: 重复的规范名称 '%s'", path, canonical)
		}
		dupCheck[canonical] = true
	}

	// 检查每个条目至少有一个云厂商的指标
	for canonical, entry := range mf.Canonical {
		hasMetric := false
		for _, def := range entry.Providers {
			if def.Metric != "" {
				hasMetric = true
				break
			}
		}
		if !hasMetric {
			return fmt.Errorf("%s: 规范名称 '%s' 没有定义任何云厂商的指标", path, canonical)
		}
	}

	return nil
}
