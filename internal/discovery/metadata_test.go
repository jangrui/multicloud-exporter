package discovery

import (
	"multicloud-exporter/internal/config"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnnotateCanonical(t *testing.T) {
	mapping := config.MetricMapping{
		Canonical: map[string]config.CanonicalEntry{
			"canonical_metric": {
				Providers: map[string]config.MetricDef{
					"aliyun":  {Metric: "AliyunMetric"},
					"tencent": {Metric: "TencentMetric"},
				},
			},
			"aliyun_only": {
				Providers: map[string]config.MetricDef{
					"aliyun": {Metric: "AliyunOnlyMetric"},
				},
			},
			"tencent_only": {
				Providers: map[string]config.MetricDef{
					"tencent": {Metric: "TencentOnlyMetric"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		metas    []MetricMeta
		expected []MetricMeta
	}{
		{
			name: "Aliyun Canonical",
			metas: []MetricMeta{
				{Provider: "aliyun", Name: "AliyunMetric"},
			},
			expected: []MetricMeta{
				{Provider: "aliyun", Name: "AliyunMetric", Canonical: "canonical_metric", Similar: []string{"tencent"}},
			},
		},
		{
			name: "Aliyun Only Canonical",
			metas: []MetricMeta{
				{Provider: "aliyun", Name: "AliyunOnlyMetric"},
			},
			expected: []MetricMeta{
				{Provider: "aliyun", Name: "AliyunOnlyMetric", Canonical: "aliyun_only"},
			},
		},
		{
			name: "Tencent Canonical",
			metas: []MetricMeta{
				{Provider: "tencent", Name: "TencentMetric"},
			},
			expected: []MetricMeta{
				{Provider: "tencent", Name: "TencentMetric", Canonical: "canonical_metric", Similar: []string{"aliyun"}},
			},
		},
		{
			name: "Tencent Only Canonical",
			metas: []MetricMeta{
				{Provider: "tencent", Name: "TencentOnlyMetric"},
			},
			expected: []MetricMeta{
				{Provider: "tencent", Name: "TencentOnlyMetric", Canonical: "tencent_only"},
			},
		},
		{
			name: "No Match",
			metas: []MetricMeta{
				{Provider: "aliyun", Name: "Unknown"},
			},
			expected: []MetricMeta{
				{Provider: "aliyun", Name: "Unknown"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnnotateCanonical(tt.metas, mapping)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestExportMetricMetaJSON(t *testing.T) {
	metas := []MetricMeta{
		{Provider: "test", Name: "metric"},
	}
	data, err := ExportMetricMetaJSON(metas)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "metric")
}

func TestGetProductMetricMeta(t *testing.T) {
	// Case 1: Unknown provider
	_, err := GetProductMetricMeta("unknown", "reg", "ak", "sk", "ns", "")
	// GetProductMetricMeta doesn't return error for unknown provider if it just falls through switch?
	// The code:
	// switch provider { ... }
	// if err != nil { return nil, err }
	// return metas, nil
	// So for unknown provider, it returns empty metas and nil error.
	assert.NoError(t, err)

	// Case 2: Aliyun provider with error
	originalNewAliyunCMSClient := newAliyunCMSClient
	defer func() { newAliyunCMSClient = originalNewAliyunCMSClient }()

	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return nil, nil // Return nil client to simulate error in FetchAliyunMetricMeta if checks are robust,
		// or panic if not.
		// FetchAliyunMetricMeta: client, err := newAliyunCMSClient... if err!=nil return err
		// client.DescribeMetricMetaList(req) -> panic if client nil
	}
	// We need to return error from factory to test error path safely
	newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
		return nil, assert.AnError
	}
	_, err = GetProductMetricMeta("aliyun", "reg", "ak", "sk", "ns", "")
	assert.Error(t, err)

	// Case 3: Success path (reuse previous mock concepts if needed, or rely on other tests covering Fetch*)
	// Since we covered Fetch* in other files, we just need to ensure GetProductMetricMeta wires it up.
	// The above error test confirms "aliyun" branch is taken.
}

func TestSaveMetricMetaJSON(t *testing.T) {
	// Use a temp file
	metas := []MetricMeta{{Name: "test"}}
	err := SaveMetricMetaJSON("temp_meta.json", metas)
	assert.NoError(t, err)
	// Clean up
	_ = os.Remove("temp_meta.json")
}
