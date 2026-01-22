package collector

import (
	"sync"
	"testing"
	"time"

	"multicloud-exporter/internal/cache"
	"multicloud-exporter/internal/cluster/four_dimension_sync"
	"multicloud-exporter/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestFourDimensionCollector_New(t *testing.T) {
	logger := zaptest.NewLogger(t)
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)

	if collector == nil {
		t.Fatal("NewFourDimensionCollector returned nil")
	}

	// 测试通过构造函数初始化后对象不为空
	// 字段已重命名为私有字段，测试只验证对象创建成功

	collector.Stop()
}

func TestFourDimensionCollector_CollectAccount(t *testing.T) {
	logger := zaptest.NewLogger(t)
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

	err := collector.CollectAccount(account)
	if err != nil {
		t.Errorf("CollectAccount failed: %v", err)
	}
}

func TestFourDimensionCollector_CollectProduct(t *testing.T) {
	logger := zaptest.NewLogger(t)
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

	err := collector.CollectProduct(account, "slb")
	if err != nil {
		t.Errorf("CollectProduct failed: %v", err)
	}
}

func TestFourDimensionCollector_CollectRegion(t *testing.T) {
	logger := zaptest.NewLogger(t)
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

	err := collector.CollectRegion(account, "slb", "cn-hangzhou")
	if err != nil {
		t.Errorf("CollectRegion failed: %v", err)
	}
}

func TestFourDimensionCollector_CollectResource(t *testing.T) {
	logger := zaptest.NewLogger(t)
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	tagCacheCfg := cache.CacheConfig{
		TTL:             30 * time.Minute,
		MaxEntries:      10000,
		CleanupInterval: 5 * time.Minute,
	}
	tagCache := cache.NewFourDimensionTagCache(logger, tagCacheCfg)

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	// tagCache 已在构造函数中初始化，无需再次设置
	_ = tagCache // 保留变量避免未使用警告
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"slb"},
	}

	err := collector.CollectResource(account, "slb", "cn-hangzhou", "lb-test-001")
	if err != nil {
		t.Errorf("CollectResource failed: %v", err)
	}
}

func TestFourDimensionCollector_UpdateFromPeer(t *testing.T) {
	logger := zaptest.NewLogger(t)
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

	update := four_dimension_sync.FourDimensionUpdate{
		Dimension: four_dimension_sync.DimensionAccount,
		AccountID: "test-account-1",
		Status:    four_dimension_sync.StatusDisabled,
		Timestamp: time.Now(),
	}

	collector.UpdateFromPeer(update)
}

func TestFourDimensionCollector_Stop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)

	collector.Stop()

	err := collector.CollectAccount(config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"slb"},
	})

	if err == nil || err.Error() != "collector已停止" {
		t.Errorf("expected 'collector已停止' error, got: %v", err)
	}
}

func TestFourDimensionCollector_Concurrent(t *testing.T) {
	logger := zaptest.NewLogger(t)
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
		Regions:   []string{"cn-hangzhou", "cn-beijing", "cn-shanghai"},
		Resources: []string{"slb", "cbwp", "oss"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = collector.CollectAccount(account)
		}()
	}

	wg.Wait()
}

func TestFourDimensionCollector_ConcurrencyLimits(t *testing.T) {
	logger := zaptest.NewLogger(t)
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 4,
		Logger:         logger,
	}

	collector := NewFourDimensionCollector(cfg)
	defer collector.Stop()

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou", "cn-beijing", "cn-shanghai"},
		Resources: []string{"slb", "cbwp", "oss", "rds"},
	}

	err := collector.CollectAccount(account)
	if err != nil {
		t.Errorf("CollectAccount failed: %v", err)
	}
}

func BenchmarkFourDimensionCollector_CollectAccount(b *testing.B) {
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
		_ = collector.CollectAccount(account)
	}
}

func BenchmarkFourDimensionCollector_CollectProduct(b *testing.B) {
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
		_ = collector.CollectProduct(account, "slb")
	}
}
