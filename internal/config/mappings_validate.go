package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func ValidateMappingStructure(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("error parsing %s: %v", path, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid document structure in %s", path)
	}
	top := root.Content[0]
	allowedTop := map[string]bool{"prefix": true, "namespaces": true, "canonical": true}
	for i := 0; i+1 < len(top.Content); i += 2 {
		k := top.Content[i].Value
		v := top.Content[i+1]
		if !allowedTop[k] {
			return fmt.Errorf("unexpected top-level key %q in %s", k, path)
		}
		switch k {
		case "prefix":
			if v.Kind != yaml.ScalarNode || v.Value == "" {
				return fmt.Errorf("invalid prefix in %s", path)
			}
		case "namespaces":
			if v.Kind != yaml.MappingNode {
				return fmt.Errorf("namespaces must be a mapping in %s", path)
			}
			for j := 0; j+1 < len(v.Content); j += 2 {
				vendor := v.Content[j].Value
				val := v.Content[j+1]
				if vendor != "aliyun" && vendor != "tencent" && vendor != "aws" && vendor != "huawei" {
					return fmt.Errorf("invalid namespaces key %q in %s", vendor, path)
				}
				if val.Kind != yaml.ScalarNode || val.Value == "" {
					return fmt.Errorf("namespace for %s must be a non-empty string in %s", vendor, path)
				}
			}
		case "canonical":
			if v.Kind != yaml.MappingNode {
				return fmt.Errorf("canonical must be a mapping in %s", path)
			}
			for j := 0; j+1 < len(v.Content); j += 2 {
				entryKey := v.Content[j].Value
				entryVal := v.Content[j+1]
				if entryVal.Kind != yaml.MappingNode {
					return fmt.Errorf("canonical entry %q must be a mapping in %s", entryKey, path)
				}
				allowedCanonical := map[string]bool{"description": true, "aliyun": true, "tencent": true, "aws": true, "huawei": true}
				for kidx := 0; kidx+1 < len(entryVal.Content); kidx += 2 {
					ck := entryVal.Content[kidx].Value
					cv := entryVal.Content[kidx+1]
					if ck == "dimensions" {
						return fmt.Errorf("dimensions must be defined under vendor entries, not canonical root in %s (entry %q)", path, entryKey)
					}
					if !allowedCanonical[ck] {
						return fmt.Errorf("unexpected key %q in canonical entry %q in %s", ck, entryKey, path)
					}
					if ck == "aliyun" || ck == "tencent" || ck == "aws" || ck == "huawei" {
						if cv.Kind != yaml.MappingNode {
							return fmt.Errorf("vendor entry %q must be a mapping in %s (entry %q)", ck, path, entryKey)
						}
						allowedVendor := map[string]bool{"metric": true, "unit": true, "scale": true, "dimensions": true}
						for vidx := 0; vidx+1 < len(cv.Content); vidx += 2 {
							vk := cv.Content[vidx].Value
							vv := cv.Content[vidx+1]
							if !allowedVendor[vk] {
								return fmt.Errorf("unexpected key %q under vendor %q in %s (entry %q)", vk, ck, path, entryKey)
							}
							if vk == "metric" {
								if vv.Kind != yaml.ScalarNode || vv.Value == "" {
									return fmt.Errorf("metric must be non-empty under vendor %q in %s (entry %q)", ck, path, entryKey)
								}
							}
							if vk == "unit" {
								if vv.Kind != yaml.ScalarNode || vv.Value == "" {
									return fmt.Errorf("unit must be non-empty under vendor %q in %s (entry %q)", ck, path, entryKey)
								}
							}
							if vk == "scale" {
								if vv.Kind != yaml.ScalarNode {
									return fmt.Errorf("scale must be a number under vendor %q in %s (entry %q)", ck, path, entryKey)
								}
							}
							if vk == "dimensions" {
								if vv.Kind != yaml.SequenceNode {
									return fmt.Errorf("dimensions must be a sequence under vendor %q in %s (entry %q)", ck, path, entryKey)
								}
								for _, dn := range vv.Content {
									if dn.Kind != yaml.ScalarNode || dn.Value == "" {
										return fmt.Errorf("dimension must be non-empty string under vendor %q in %s (entry %q)", ck, path, entryKey)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func ValidateAllMappings(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to list mapping files in %s: %v", dir, err)
	}
	var errs []error
	for _, f := range files {
		if err := ValidateMappingStructure(f); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		msg := ""
		for _, e := range errs {
			if msg == "" {
				msg = e.Error()
			} else {
				msg += "\n" + e.Error()
			}
		}
		return errors.New(msg)
	}
	return nil
}
