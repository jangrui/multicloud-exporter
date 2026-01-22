package huawei

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"multicloud-exporter/internal/config"
	providerscommon "multicloud-exporter/internal/providers/common"

	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

func TestListOBSBuckets_Success(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	bucket1 := "test-bucket-1"
	bucket2 := "test-bucket-2"
	loc1 := "cn-north-4"
	loc2 := "cn-north-4"

	// 设置 mock 客户端
	mockOBS := &mockOBSClient{
		listBucketsFunc: func(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
			return &obs.ListBucketsOutput{
				Buckets: []obs.Bucket{
					{Name: bucket1, Location: loc1},
					{Name: bucket2, Location: loc2},
				},
			}, nil
		},
		closeFunc: func() {},
	}

	mockFactory := &mockClientFactory{
		newOBSClientFunc: func(region, ak, sk string) (OBSClient, error) {
			return mockOBS, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	buckets := c.listOBSBuckets(account, "cn-north-4")

	assert.Len(t, buckets, 2)
	assert.Equal(t, "test-bucket-1", buckets[0].Name)
	assert.Equal(t, "cn-north-4", buckets[0].Location)
	assert.Equal(t, "test-bucket-2", buckets[1].Name)

	// 验证缓存已设置
	key := c.cacheKey(account, "cn-north-4", "SYS.OBS", "obs")
	entry, ok := c.resCache[key]
	assert.True(t, ok)
	assert.Len(t, entry.IDs, 2)
}

func TestListOBSBuckets_Empty(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	// 设置 mock 客户端返回空列表
	mockOBS := &mockOBSClient{
		listBucketsFunc: func(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
			return &obs.ListBucketsOutput{
				Buckets: []obs.Bucket{},
			}, nil
		},
		closeFunc: func() {},
	}

	mockFactory := &mockClientFactory{
		newOBSClientFunc: func(region, ak, sk string) (OBSClient, error) {
			return mockOBS, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	buckets := c.listOBSBuckets(account, "cn-north-4")

	assert.Empty(t, buckets)
}

func TestListOBSBuckets_FilterByRegion(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	// 设置 mock 客户端返回多个区域的 bucket
	mockOBS := &mockOBSClient{
		listBucketsFunc: func(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
			return &obs.ListBucketsOutput{
				Buckets: []obs.Bucket{
					{Name: "bucket-cn-north-4", Location: "cn-north-4"},
					{Name: "bucket-cn-south-1", Location: "cn-south-1"},
					{Name: "bucket-cn-east-3", Location: "cn-east-3"},
				},
			}, nil
		},
		closeFunc: func() {},
	}

	mockFactory := &mockClientFactory{
		newOBSClientFunc: func(region, ak, sk string) (OBSClient, error) {
			return mockOBS, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	buckets := c.listOBSBuckets(account, "cn-north-4")

	// 应该只返回 cn-north-4 区域的 bucket
	assert.Len(t, buckets, 1)
	assert.Equal(t, "bucket-cn-north-4", buckets[0].Name)
	assert.Equal(t, "cn-north-4", buckets[0].Location)
}

func TestListOBSBuckets_Cached(t *testing.T) {
	c := &Collector{
		cfg:      &config.Config{},
		resCache: make(map[string]resCacheEntry),
	}

	account := config.CloudAccount{
		AccountID: "test-account",
		Provider:  "huawei",
	}

	// 预设缓存
	c.setCachedIDs(account, "cn-north-4", "SYS.OBS", "obs", []string{"cached-bucket-1", "cached-bucket-2"})

	// mock 客户端不应该被调用
	called := false
	mockOBS := &mockOBSClient{
		listBucketsFunc: func(input *obs.ListBucketsInput) (*obs.ListBucketsOutput, error) {
			called = true
			return nil, nil
		},
		closeFunc: func() {},
	}

	mockFactory := &mockClientFactory{
		newOBSClientFunc: func(region, ak, sk string) (OBSClient, error) {
			return mockOBS, nil
		},
	}

	c.setClientFactoryForTesting(mockFactory)

	buckets := c.listOBSBuckets(account, "cn-north-4")

	assert.Len(t, buckets, 2)
	assert.Equal(t, "cached-bucket-1", buckets[0].Name)
	assert.Equal(t, "cached-bucket-2", buckets[1].Name)
	assert.False(t, called, "不应调用 API（缓存命中）")
}

func TestListOBSBuckets_RegionSkip(t *testing.T) {
	c := &Collector{
		cfg:                   &config.Config{},
		resCache:              make(map[string]resCacheEntry),
		productRegionManagers: make(map[string]providerscommon.RegionManager),
	}

	// 设置 RegionManager 跳过指定区域
	mockRM := &mockRegionManager{
		shouldSkipRegionFunc: func(accountID, region string) bool {
			return true
		},
	}

	c.productRegionManagers["obs"] = mockRM

	account := config.CloudAccount{
		AccountID:       "test-account",
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-sk",
		Provider:        "huawei",
	}

	buckets := c.listOBSBuckets(account, "cn-north-4")

	assert.Empty(t, buckets)
}
