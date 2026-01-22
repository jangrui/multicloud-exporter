package huawei

import (
	"testing"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
)

func TestCollectELB_Basic(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
		disc:     discovery.NewManager(&config.Config{}),
	}

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
		Regions:         []string{"cn-north-4"},
		Resources:       []string{"elb"},
	}

	c.collectELB(account, "cn-north-4")
}

func TestCollectOBS_Basic(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
		disc:     discovery.NewManager(&config.Config{}),
	}

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
		Regions:         []string{"cn-north-4"},
		Resources:       []string{"obs"},
	}

	c.collectOBS(account, "cn-north-4")
}
