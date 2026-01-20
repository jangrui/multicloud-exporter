package common

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockBroadcaster 模拟广播器
type MockBroadcaster struct {
	mu             sync.Mutex
	broadcastCount int
	messages       []BroadcastMessage
}

type BroadcastMessage struct {
	Provider      string
	Product       string
	AccountID     string
	Region        string
	Status        string
	ResourceCount int
}

func (m *MockBroadcaster) BroadcastRegionStatus(provider, product, accountID, region, status string, resourceCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcastCount++
	m.messages = append(m.messages, BroadcastMessage{
		Provider:      provider,
		Product:       product,
		AccountID:     accountID,
		Region:        region,
		Status:        status,
		ResourceCount: resourceCount,
	})
}

func (m *MockBroadcaster) GetBroadcastCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.broadcastCount
}

func (m *MockBroadcaster) GetMessages() []BroadcastMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

func TestNewRegionManager(t *testing.T) {
	config := RegionDiscoveryConfig{
		Enabled:              true,
		DiscoveryInterval:    1 * time.Minute,
		EmptyThreshold:       3,
		MaxAccounts:          10,
		CleanupInterval:      5 * time.Minute,
		MaxRegionsPerAccount: 50,
	}

	mgr := NewRegionManager(config)

	assert.NotNil(t, mgr)
	rm := mgr.(*SmartRegionManager)
	assert.Equal(t, config, rm.config)
	assert.NotNil(t, rm.regionMap)
	assert.NotNil(t, rm.stopChan)
}

func TestRegionManager_SetBroadcaster(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})
	broadcaster := &MockBroadcaster{}

	mgr.SetBroadcaster(broadcaster, "aliyun", "")

	rm := mgr.(*SmartRegionManager)
	assert.Equal(t, broadcaster, rm.broadcaster)
	assert.Equal(t, "aliyun", rm.providerName)
}

func TestRegionManager_GetActiveRegions_Unknown(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:        true,
		EmptyThreshold: 3,
	})
	accountID := "test-account"
	allRegions := []string{"cn-north-4", "cn-south-1", "cn-east-2"}

	active := mgr.GetActiveRegions(accountID, allRegions)

	assert.Equal(t, allRegions, active)
}

func TestRegionManager_GetActiveRegions_WithStatus(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:        true,
		EmptyThreshold: 1,
	})
	accountID := "test-account"
	allRegions := []string{"cn-north-4", "cn-south-1", "cn-east-2"}

	mgr.UpdateRegionStatus(accountID, "cn-north-4", 5, RegionStatusActive)
	mgr.UpdateRegionStatus(accountID, "cn-south-1", 0, RegionStatusEmpty)
	mgr.UpdateRegionStatus(accountID, "cn-east-2", 0, RegionStatusEmpty)

	active := mgr.GetActiveRegions(accountID, allRegions)

	assert.Equal(t, []string{"cn-north-4"}, active)
}

func TestRegionManager_GetActiveRegions_EmptyBelowThreshold(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:        true,
		EmptyThreshold: 3,
	})
	accountID := "test-account"
	allRegions := []string{"cn-north-4", "cn-south-1", "cn-east-2"}

	mgr.UpdateRegionStatus(accountID, "cn-north-4", 5, RegionStatusActive)
	for i := 0; i < 2; i++ {
		mgr.UpdateRegionStatus(accountID, "cn-south-1", 0, RegionStatusEmpty)
		mgr.UpdateRegionStatus(accountID, "cn-east-2", 0, RegionStatusEmpty)
	}

	active := mgr.GetActiveRegions(accountID, allRegions)

	assert.ElementsMatch(t, []string{"cn-north-4", "cn-south-1", "cn-east-2"}, active)
}

func TestRegionManager_UpdateRegionStatus(t *testing.T) {
	broadcaster := &MockBroadcaster{}
	mgr := NewRegionManager(RegionDiscoveryConfig{})
	mgr.SetBroadcaster(broadcaster, "aliyun", "")

	accountID := "test-account"
	region := "cn-north-4"

	mgr.UpdateRegionStatus(accountID, region, 5, RegionStatusActive)

	info, ok := mgr.GetRegionInfo(accountID, region)
	assert.True(t, ok)
	assert.Equal(t, RegionStatusActive, info.Status)
	assert.Equal(t, 5, info.ResourceCount)
	assert.False(t, info.LastSeen.IsZero())
	assert.False(t, info.LastActive.IsZero())
	assert.Equal(t, 0, info.EmptyCount)

	messages := broadcaster.GetMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, string(RegionStatusActive), messages[0].Status)
	assert.Equal(t, 5, messages[0].ResourceCount)
}

func TestRegionManager_UpdateRegionStatus_EmptyCount(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})

	accountID := "test-account"
	region := "cn-north-4"

	for i := 0; i < 5; i++ {
		mgr.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
	}

	info, ok := mgr.GetRegionInfo(accountID, region)
	assert.True(t, ok)
	assert.Equal(t, 5, info.EmptyCount)
}

func TestRegionManager_UpdateFromPeer(t *testing.T) {
	broadcaster := &MockBroadcaster{}
	mgr := NewRegionManager(RegionDiscoveryConfig{})
	mgr.SetBroadcaster(broadcaster, "aliyun", "")

	accountID := "test-account"
	region := "cn-north-4"

	mgr.UpdateFromPeer(accountID, region, 5, string(RegionStatusActive))

	info, ok := mgr.GetRegionInfo(accountID, region)
	assert.True(t, ok)
	assert.Equal(t, RegionStatusActive, info.Status)
	assert.Equal(t, 5, info.ResourceCount)

	assert.Equal(t, 0, broadcaster.GetBroadcastCount())
}

func TestRegionManager_MarkRegionForRediscovery(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})

	accountID := "test-account"
	region := "cn-north-4"

	mgr.UpdateRegionStatus(accountID, region, 5, RegionStatusActive)

	mgr.MarkRegionForRediscovery(accountID, region)

	info, ok := mgr.GetRegionInfo(accountID, region)
	assert.True(t, ok)
	assert.Equal(t, RegionStatusUnknown, info.Status)
	assert.Equal(t, 5, info.ResourceCount)
	assert.Equal(t, 0, info.EmptyCount)
}

func TestRegionManager_ShouldSkipRegion(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:        true,
		EmptyThreshold: 3,
	})
	accountID := "test-account"
	region := "cn-north-4"

	skip := mgr.ShouldSkipRegion(accountID, region)
	assert.False(t, skip)

	mgr.UpdateRegionStatus(accountID, region, 5, RegionStatusActive)
	skip = mgr.ShouldSkipRegion(accountID, region)
	assert.False(t, skip)

	for i := 0; i < 2; i++ {
		mgr.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
	}
	skip = mgr.ShouldSkipRegion(accountID, region)
	assert.False(t, skip)

	mgr.UpdateRegionStatus(accountID, region, 0, RegionStatusEmpty)
	skip = mgr.ShouldSkipRegion(accountID, region)
	assert.True(t, skip)
}

func TestRegionManager_GetRegionInfo_NotFound(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})

	info, ok := mgr.GetRegionInfo("unknown-account", "unknown-region")
	assert.False(t, ok)
	assert.Equal(t, RegionInfo{}, info)
}

func TestRegionManager_GetStats(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})

	mgr.UpdateRegionStatus("account1", "cn-north-4", 5, RegionStatusActive)
	mgr.UpdateRegionStatus("account1", "cn-south-1", 0, RegionStatusEmpty)
	mgr.UpdateRegionStatus("account1", "cn-east-2", 0, RegionStatusEmpty)
	mgr.UpdateRegionStatus("account2", "cn-north-4", 10, RegionStatusActive)

	stats := mgr.GetStats()

	assert.Equal(t, 2, stats.TotalAccounts)
	assert.Equal(t, 4, stats.TotalRegions)
	assert.Equal(t, 2, stats.ActiveRegions)
	assert.Equal(t, 2, stats.EmptyRegions)
	assert.Equal(t, 0, stats.UnknownRegions)
	assert.NotZero(t, stats.UpdateCount)
}

func TestRegionManager_StartRediscoveryScheduler(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:           true,
		DiscoveryInterval: 50 * time.Millisecond,
	})

	mgr.UpdateRegionStatus("account1", "cn-north-4", 5, RegionStatusActive)
	mgr.UpdateRegionStatus("account1", "cn-south-1", 0, RegionStatusEmpty)

	rm := mgr.(*SmartRegionManager)

	time.Sleep(100 * time.Millisecond)
	rm.performPeriodicTasks()

	stats := mgr.GetStats()
	assert.NotZero(t, stats.RediscoveryCount)

	mgr.StartRediscoveryScheduler()
	mgr.Stop()
}

func TestRegionManager_Stop(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		DiscoveryInterval: 100 * time.Millisecond,
	})

	mgr.StartRediscoveryScheduler()

	time.Sleep(50 * time.Millisecond)

	mgr.Stop()

	rm := mgr.(*SmartRegionManager)
	select {
	case <-time.After(100 * time.Millisecond):
	case <-rm.stopChan:
	}
}

func TestRegionManager_PerformPeriodicTasks(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{
		Enabled:              true,
		DiscoveryInterval:    50 * time.Millisecond,
		CleanupInterval:      100 * time.Millisecond,
		MaxAccounts:          2,
		MaxRegionsPerAccount: 3,
	})

	for i := 0; i < 5; i++ {
		region := "region" + string(rune('a'+i))
		mgr.UpdateRegionStatus("account1", region, 5, RegionStatusActive)
		mgr.UpdateRegionStatus("account2", region, 5, RegionStatusActive)
	}

	rm := mgr.(*SmartRegionManager)
	rm.performPeriodicTasks()

	stats := mgr.GetStats()
	assert.NotZero(t, stats.LastCleanupTime)
}

func TestRegionManager_ConcurrentSafety(t *testing.T) {
	mgr := NewRegionManager(RegionDiscoveryConfig{})
	accountID := "test-account"
	region := "cn-north-4"

	var wg sync.WaitGroup
	var errors int32

	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func(idx int) {
			defer wg.Done()
			mgr.UpdateRegionStatus(accountID, region, idx, RegionStatusActive)
		}(i)

		go func() {
			defer wg.Done()
			mgr.GetRegionInfo(accountID, region)
		}()

		go func() {
			defer wg.Done()
			mgr.ShouldSkipRegion(accountID, region)
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(0), atomic.LoadInt32(&errors))

	info, ok := mgr.GetRegionInfo(accountID, region)
	assert.True(t, ok)
	assert.NotNil(t, info)
}
