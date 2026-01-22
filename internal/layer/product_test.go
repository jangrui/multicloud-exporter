package layer

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProductManagerBasicOperations(t *testing.T) {
	logger := zap.NewNop()

	pm := NewProductManager(ProductManagerConfig{
		AccountID: "test-account",
		ProductID: "ecs",
		Provider:  "aliyun",
		Region:    "cn-hangzhou",
		Logger:    logger,
	})

	defer pm.Cleanup()

	assert.Equal(t, ProductStatusActive, pm.GetProductStatus())
	assert.Equal(t, uint32(0), pm.GetRegionCount())
}

func TestProductManagerRegionLifecycle(t *testing.T) {
	logger := zap.NewNop()

	pm := NewProductManager(ProductManagerConfig{
		AccountID: "test-account",
		ProductID: "ecs",
		Provider:  "aliyun",
		Region:    "cn-hangzhou",
		Logger:    logger,
	})

	defer pm.Cleanup()

	rm, err := pm.GetRegionManager("cn-hangzhou")
	require.NoError(t, err)
	require.NotNil(t, rm)

	rm2, err := pm.GetRegionManager("cn-hangzhou")
	require.NoError(t, err)
	assert.Equal(t, rm, rm2)

	assert.Equal(t, uint32(1), pm.GetRegionCount())

	rm3, err := pm.GetRegionManager("cn-beijing")
	require.NoError(t, err)
	assert.NotEqual(t, rm, rm3)

	assert.Equal(t, uint32(2), pm.GetRegionCount())
}

func TestProductManagerStatusChanges(t *testing.T) {
	logger := zap.NewNop()

	pm := NewProductManager(ProductManagerConfig{
		AccountID: "test-account",
		ProductID: "ecs",
		Provider:  "aliyun",
		Region:    "cn-hangzhou",
		Logger:    logger,
	})

	defer pm.Cleanup()

	pm.UpdateProductStatus(ProductStatusDegraded, "test reason")
	assert.Equal(t, ProductStatusDegraded, pm.GetProductStatus())
	assert.True(t, pm.ShouldSkipProduct())

	pm.UpdateProductStatus(ProductStatusActive, "test reason")
	assert.Equal(t, ProductStatusActive, pm.GetProductStatus())
	assert.False(t, pm.ShouldSkipProduct())

	pm.UpdateProductStatus(ProductStatusDisabled, "test reason")
	assert.Equal(t, ProductStatusDisabled, pm.GetProductStatus())
	assert.True(t, pm.ShouldSkipProduct())
}

func TestProductManagerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()

	pm := NewProductManager(ProductManagerConfig{
		AccountID: "test-account",
		ProductID: "ecs",
		Provider:  "aliyun",
		Region:    "cn-hangzhou",
		Logger:    logger,
	})

	defer pm.Cleanup()

	const numGoroutines = 100
	const regionsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < regionsPerGoroutine; j++ {
				regionID := fmt.Sprintf("region-%d-%d", id, j)
				rm, err := pm.GetRegionManager(regionID)
				assert.NoError(t, err)
				assert.NotNil(t, rm)

				if j%2 == 0 {
					rm.UpdateRegionStatus(RegionStatusActive, "test")
				} else {
					rm.UpdateRegionStatus(RegionStatusDegraded, "test")
				}
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, uint32(numGoroutines*regionsPerGoroutine), pm.GetRegionCount())
}
