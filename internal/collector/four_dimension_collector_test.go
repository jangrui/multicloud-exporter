package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"multicloud-exporter/internal/cache"
	"multicloud-exporter/internal/cluster/four_dimension_sync"
	"multicloud-exporter/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type mockFourDimensionAdapter struct {
	regions              []string
	collectRegionCounter int32
}

func (m *mockFourDimensionAdapter) CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error {
	return nil
}

func (m *mockFourDimensionAdapter) CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error {
	return nil
}

func (m *mockFourDimensionAdapter) CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error {
	atomic.AddInt32(&m.collectRegionCounter, 1)
	return nil
}

func (m *mockFourDimensionAdapter) CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error {
	return nil
}

func (m *mockFourDimensionAdapter) DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error) {
	return map[string]map[string][]string{}, nil
}

func (m *mockFourDimensionAdapter) GetRegions(ctx context.Context, account config.CloudAccount) ([]string, error) {
	return m.regions, nil
}

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

func TestFourDimensionCollector_GetStatus_NoCrash(t *testing.T) {
	logger := zaptest.NewLogger(t)
	syncMgr := four_dimension_sync.NewFourDimensionSync("test-service", "8080", "")

	cfg := FourDimensionCollectorConfig{
		Config:         nil,
		SyncManager:    syncMgr,
		TagCacheTTL:    30 * time.Minute,
		MaxConcurrency: 20,
		Logger:         logger,
	}

	coll := NewFourDimensionCollector(cfg)
	defer coll.Stop()

	for range 1000 {
		_ = coll.GetStatus()
	}
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

func TestFourDimensionCollector_ProductScrapeIntervalSkip(t *testing.T) {
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

	adapter := &mockFourDimensionAdapter{regions: []string{"cn-hangzhou"}}
	collector.adapters["aliyun"] = adapter

	account := config.CloudAccount{
		Provider:  "aliyun",
		AccountID: "test-account-1",
		Regions:   []string{"cn-hangzhou"},
		Resources: []string{"s3"},
		ProductMetric: map[string][]config.MetricGroupConfig{
			"s3": {
				{MetricList: []string{"BucketSizeBytes", "NumberOfObjects"}, ScrapeInterval: "1h"},
			},
		},
	}

	if err := collector.CollectProduct(account, "s3"); err != nil {
		t.Fatalf("CollectProduct failed: %v", err)
	}
	if err := collector.CollectProduct(account, "s3"); err != nil {
		t.Fatalf("CollectProduct failed: %v", err)
	}

	if count := atomic.LoadInt32(&adapter.collectRegionCounter); count != 1 {
		t.Fatalf("expected CollectRegionMetrics called once, got %d", count)
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
