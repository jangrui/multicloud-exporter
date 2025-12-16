package aliyun

import (
	metrics "multicloud-exporter/internal/metrics"
	"strings"
	"unicode"
)

// camelToSnake converts CamelCase to snake_case.
// Duplicated from ecs.go to avoid public API changes.
func camelToSnakeOSS(s string) string {
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

func canonicalizeOSS(metric string) string {
	m := strings.ReplaceAll(metric, ".", "_")
	m = camelToSnakeOSS(m)
	return strings.ToLower(m)
}

func init() {
	// 注意：映射配置已迁移到 configs/mappings/s3.metrics.yaml
	// 这里保留硬编码前缀作为向后兼容，但会被配置文件覆盖（配置文件使用 s3 前缀）
	metrics.RegisterNamespacePrefix("acs_oss_dashboard", "oss")
	metrics.RegisterNamespaceAliasFunc("acs_oss_dashboard", canonicalizeOSS)
}
