package utils

import (
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses a duration string, supporting "d" for days in addition to standard time units.
// Supported units:
//   - ns: nanosecond
//   - us/µs: microsecond
//   - ms: millisecond
//   - s: second
//   - m: minute
//   - h: hour
//   - d: day (1d = 24h)
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
