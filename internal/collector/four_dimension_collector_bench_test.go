package collector

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"multicloud-exporter/internal/cluster/four_dimension_sync"
	"multicloud-exporter/internal/config"

	"go.uber.org/zap"
)

// BenchmarkFourDimensionCollector_CollectRegion 测试区域级采集性能
func BenchmarkFourDimensionCollector_CollectRegion(b *testing.B) {
	logger := zap.NewNop()
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou", "cn-beijing"},
		Resources: []string{"slb", "cbwp"},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = collector.CollectRegion(account, "slb", "cn-hangzhou")
	}
}

// BenchmarkFourDimensionCollector_CollectResource 测试资源级采集性能
func BenchmarkFourDimensionCollector_CollectResource(b *testing.B) {
	logger := zap.NewNop()
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"slb"},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = collector.CollectResource(account, "slb", "cn-hangzhou", "resource-1")
	}
}

// BenchmarkFourDimensionCollector_ConcurrentAccountCollect 测试并发账号采集
func BenchmarkFourDimensionCollector_ConcurrentAccountCollect(b *testing.B) {
	logger := zap.NewNop()
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	accounts := make([]config.CloudAccount, 10)
	for i := 0; i < 10; i++ {
		accounts[i] = config.CloudAccount{
			Provider:  "aliyun",
			AccountID: fmt.Sprintf("test-account-%d", i),
			Regions:   []string{"cn-hangzhou"},
			Resources: []string{"slb"},
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		concurrency := 4

		for j := 0; j < concurrency; j++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_ = collector.CollectAccount(accounts[idx%len(accounts)])
			}(j)
		}

		wg.Wait()
	}
}

// BenchmarkFourDimensionCollector_LockContention 测试读写锁竞争
func BenchmarkFourDimensionCollector_LockContention(b *testing.B) {
	logger := zap.NewNop()
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"slb"},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		wg.Add(2)

		go func() {
			defer wg.Done()
			_ = collector.GetStatus()
		}()

		go func() {
			defer wg.Done()
			_ = collector.CollectAccount(account)
		}()

		wg.Wait()
	}
}

// BenchmarkFourDimensionCollector_DimensionSwitch 测试不同维度切换性能
func BenchmarkFourDimensionCollector_DimensionSwitch(b *testing.B) {
	logger := zap.NewNop()
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
		CollectionMode: "account",
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"slb"},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		switch i % 4 {
		case 0:
			_ = collector.CollectAccount(account)
		case 1:
			_ = collector.CollectProduct(account, "slb")
		case 2:
			_ = collector.CollectRegion(account, "slb", "cn-hangzhou")
		case 3:
			_ = collector.CollectResource(account, "slb", "cn-hangzhou", "resource-1")
		}
	}
}
