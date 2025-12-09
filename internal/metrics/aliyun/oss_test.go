package aliyun

import (
	"testing"
)

func TestCanonicalizeOSS(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"BucketName", "bucket_name"},
		{"MeteringStorageUtilization", "metering_storage_utilization"},
		{"StandardStorage", "standard_storage"},
		{"A.B", "a_b"},
	}

	for _, c := range cases {
		got := canonicalizeOSS(c.in)
		if got != c.want {
			t.Errorf("OSS canonicalize %q: got %q, want %q", c.in, got, c.want)
		}
	}
}
