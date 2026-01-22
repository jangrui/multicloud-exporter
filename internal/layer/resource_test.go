package layer

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestResourceManagerBasicOperations(t *testing.T) {
	logger := zap.NewNop()

	rm := NewResourceManager(ResourceManagerConfig{
		AccountID:  "test-account",
		ProductID:  "ecs",
		Region:     "cn-hangzhou",
		ResourceID: "i-12345678",
		Provider:   "aliyun",
		Logger:     logger,
	})

	defer rm.Cleanup()

	assert.Equal(t, ResourceStatusActive, rm.GetResourceStatus())
	assert.False(t, rm.ShouldSkipResource())
}

func TestResourceManagerStatusChanges(t *testing.T) {
	logger := zap.NewNop()

	rm := NewResourceManager(ResourceManagerConfig{
		AccountID:  "test-account",
		ProductID:  "ecs",
		Region:     "cn-hangzhou",
		ResourceID: "i-12345678",
		Provider:   "aliyun",
		Logger:     logger,
	})

	defer rm.Cleanup()

	rm.UpdateResourceStatus(ResourceStatusDegraded, "test reason")
	assert.Equal(t, ResourceStatusDegraded, rm.GetResourceStatus())
	assert.True(t, rm.ShouldSkipResource())

	rm.UpdateResourceStatus(ResourceStatusActive, "test reason")
	assert.Equal(t, ResourceStatusActive, rm.GetResourceStatus())
	assert.False(t, rm.ShouldSkipResource())

	rm.UpdateResourceStatus(ResourceStatusDisabled, "test reason")
	assert.Equal(t, ResourceStatusDisabled, rm.GetResourceStatus())
	assert.True(t, rm.ShouldSkipResource())
}

func TestResourceManagerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()

	rm := NewResourceManager(ResourceManagerConfig{
		AccountID:  "test-account",
		ProductID:  "ecs",
		Region:     "cn-hangzhou",
		ResourceID: "i-12345678",
		Provider:   "aliyun",
		Logger:     logger,
	})

	defer rm.Cleanup()

	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			if id%2 == 0 {
				rm.UpdateResourceStatus(ResourceStatusActive, fmt.Sprintf("update %d", id))
			} else {
				rm.UpdateResourceStatus(ResourceStatusDegraded, fmt.Sprintf("update %d", id))
			}
		}(i)
	}

	wg.Wait()

	status := rm.GetResourceStatus()
	assert.True(t, status == ResourceStatusActive || status == ResourceStatusDegraded || status == ResourceStatusDisabled)
}
