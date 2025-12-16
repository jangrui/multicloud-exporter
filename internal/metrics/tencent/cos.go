package tencent

import (
	metrics "multicloud-exporter/internal/metrics"
	"strings"
	"unicode"
)

// camelToSnakeCOS converts CamelCase to snake_case.
func camelToSnakeCOS(s string) string {
	var b []rune
	var prev rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b = append(b, '_')
		}
		b = append(b, unicode.ToLower(r))
		prev = r
	}
	out := string(b)
	return out
}

func canonicalizeCOS(metric string) string {
	m := strings.ReplaceAll(metric, ".", "_")
	m = camelToSnakeCOS(m)
	return strings.ToLower(m)
}

func init() {
	// 注意：映射配置已迁移到 configs/mappings/s3.metrics.yaml
	// 这里保留硬编码前缀作为向后兼容，但会被配置文件覆盖（配置文件使用 s3 前缀）
	metrics.RegisterNamespacePrefix("QCE/COS", "cos")
	metrics.RegisterNamespaceAliasFunc("QCE/COS", canonicalizeCOS)
}
