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
	metrics.RegisterNamespacePrefix("acs_oss_dashboard", "oss")
	metrics.RegisterNamespaceAliasFunc("acs_oss_dashboard", canonicalizeOSS)
}
