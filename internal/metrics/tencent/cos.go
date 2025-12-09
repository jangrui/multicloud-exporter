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
	metrics.RegisterNamespacePrefix("QCE/COS", "cos")
	metrics.RegisterNamespaceAliasFunc("QCE/COS", canonicalizeCOS)
}
