package help

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBWPHelp(t *testing.T) {
	tests := []struct {
		metric   string
		expected string
	}{
		{"in_utilization_pct", " - 共享带宽入方向带宽利用率（百分比）"},
		{"out_utilization_pct", " - 共享带宽出方向带宽利用率（百分比）"},
		{"in_bps", " - 共享带宽入方向带宽速率（bit/s）"},
		{"out_bps", " - 共享带宽出方向带宽速率（bit/s）"},
		{"in_pps", " - 共享带宽入方向包速率（包/秒）"},
		{"out_pps", " - 共享带宽出方向包速率（包/秒）"},
		{"in_drop_pps", " - 共享带宽入方向丢包速率（包/秒）"},
		{"out_drop_pps", " - 共享带宽出方向丢包速率（包/秒）"},
		{"unknown", " - 云产品指标"},
	}

	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			assert.Equal(t, tt.expected, BWPHelp(tt.metric))
		})
	}
}
