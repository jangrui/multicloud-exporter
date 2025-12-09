package tencent

import (
	"testing"
)

func TestCanonicalizeCOS(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"StdStorage", "std_storage"},
		{"InternetTraffic", "internet_traffic"},
		{"Requests", "requests"},
		{"A.B", "a_b"},
	}

	for _, c := range cases {
		got := canonicalizeCOS(c.in)
		if got != c.want {
			t.Errorf("COS canonicalize %q: got %q, want %q", c.in, got, c.want)
		}
	}
}
