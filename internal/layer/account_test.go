package layer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/infrastructure/lock_free"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAccountManagerBasicOperations(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	// 测试初始状态
	assert.Equal(t, AccountStatusActive, am.GetAccountStatus())
	assert.Equal(t, uint32(0), am.GetProductCount())
}

func TestAccountManagerProductLifecycle(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	// 测试创建产品管理器
	pm, err := am.GetProductManager("ecs")
	require.NoError(t, err)
	require.NotNil(t, pm)

	// 测试重复获取（应返回相同实例）
	pm2, err := am.GetProductManager("ecs")
	require.NoError(t, err)
	assert.Equal(t, pm, pm2)

	// 测试产品数量
	assert.Equal(t, uint32(1), am.GetProductCount())

	// 测试创建第二个产品管理器
	pm3, err := am.GetProductManager("oss")
	require.NoError(t, err)
	require.NotNil(t, pm3)
	assert.NotEqual(t, pm, pm3)

	// 测试产品数量增加
	assert.Equal(t, uint32(2), am.GetProductCount())
}

func TestAccountManagerStatusChanges(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	// 测试降级
	am.DegradateAccount("test reason")
	assert.Equal(t, AccountStatusDegraded, am.GetAccountStatus())

	// 测试降级后跳过
	assert.True(t, am.ShouldSkipAccount())

	// 测试恢复
	am.RecoverAccount("test reason")
	assert.Equal(t, AccountStatusActive, am.GetAccountStatus())

	// 测试恢复后不跳过
	assert.False(t, am.ShouldSkipAccount())

	// 测试禁用
	am.DisableAccount("test reason")
	assert.Equal(t, AccountStatusDisabled, am.GetAccountStatus())
	assert.True(t, am.ShouldSkipAccount())
}

func TestAccountManagerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	const numGoroutines = 100
	const productsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < productsPerGoroutine; j++ {
				productID := fmt.Sprintf("product-%d-%d", id, j)
				pm, err := am.GetProductManager(productID)
				assert.NoError(t, err)
				assert.NotNil(t, pm)

				// 并发更新状态
				if j%2 == 0 {
					pm.UpdateProductStatus(ProductStatusActive, "test")
				} else {
					pm.UpdateProductStatus(ProductStatusDegraded, "test")
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证产品数量（去重）
	assert.Equal(t, uint32(numGoroutines*productsPerGoroutine), am.GetProductCount())
}

func TestAccountManagerWorkerPool(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	wp := am.CreateWorkerPool(&config.FourDimensionConfig{
		ConcurrencyMode: "standard",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wp.Start(ctx)

	const numTasks = 100
	counter := lock_free.NewLockFreeManager()
	counter.StoreInt64(0)

	for i := 0; i < numTasks; i++ {
		wp.Submit(func() {
			counter.AddInt64(1)
		})
	}

	time.Sleep(1 * time.Second)
	wp.Stop()

	assert.Equal(t, int64(numTasks), counter.LoadInt64())
}

func TestAccountManagerLRUCache(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	// 测试访问追踪
	pm, err := am.GetProductManager("ecs")
	require.NoError(t, err)
	require.NotNil(t, pm)

	// 多次访问以触发 LRU 更新
	for i := 0; i < 10; i++ {
		_, _ = am.GetProductManager("ecs")
	}

	// 获取 LRU 缓存统计
	stats := am.GetLRUCacheStats()
	assert.Greater(t, stats.Size, 0)
	assert.Greater(t, stats.Capacity, uint64(0))
}

func TestAccountManagerStats(t *testing.T) {
	logger := zap.NewNop()

	am := NewAccountManager(AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})

	defer am.Stop()
	defer am.Cleanup()

	// 创建产品以增加统计
	pm, err := am.GetProductManager("ecs")
	require.NoError(t, err)
	require.NotNil(t, pm)

	stats := am.GetStats()
	assert.Greater(t, stats.TotalRequests, int64(0))
}
