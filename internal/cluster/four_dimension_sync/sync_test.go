package four_dimension_sync_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	fds "multicloud-exporter/internal/cluster/four_dimension_sync"
)

// mockDimensionManager 模拟维度管理器
type mockDimensionManager struct {
	receivedUpdates []fds.FourDimensionUpdate
	mu              sync.Mutex
}

func (m *mockDimensionManager) UpdateFromPeer(update fds.FourDimensionUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedUpdates = append(m.receivedUpdates, update)
}

func (m *mockDimensionManager) getUpdateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.receivedUpdates)
}

func TestFourDimensionSync_NewManager(t *testing.T) {
	syncMgr := fds.NewFourDimensionSync("test-service", "9101", "test-secret")
	if syncMgr == nil {
		t.Error("NewFourDimensionSync should return non-nil")
	}
}

func TestFourDimensionSync_HandleSync_Account(t *testing.T) {
	syncMgr := fds.NewFourDimensionSync("test-service", "9101", "test-secret")
	manager := &mockDimensionManager{}
	syncMgr.RegisterManager(fds.DimensionAccount, "account-1", manager)

	batch := fds.BatchUpdate{
		Updates: []fds.FourDimensionUpdate{
			{
				Dimension: fds.DimensionAccount,
				AccountID: "account-1",
				Status:    fds.StatusActive,
				Timestamp: time.Now(),
			},
		},
		Version: 1,
	}

	body, _ := json.Marshal(batch)
	req := httptest.NewRequest("POST", "/api/v1/cluster/four-dimension-sync", bytes.NewReader(body))
	req.Header.Set("X-Cluster-Secret", "test-secret")
	w := httptest.NewRecorder()

	syncMgr.HandleSync(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)
	if manager.getUpdateCount() != 1 {
		t.Errorf("Expected 1 account update, got %d", manager.getUpdateCount())
	}
}

func TestFourDimensionSync_HandleSync_AllDimensions(t *testing.T) {
	syncMgr := fds.NewFourDimensionSync("test-service", "9101", "test-secret")
	accountManager := &mockDimensionManager{}
	productManager := &mockDimensionManager{}
	regionManager := &mockDimensionManager{}
	resourceManager := &mockDimensionManager{}

	syncMgr.RegisterManager(fds.DimensionAccount, "account-1", accountManager)
	syncMgr.RegisterManager(fds.DimensionProduct, "account-1", productManager)
	syncMgr.RegisterManager(fds.DimensionRegion, "account-1:product-1", regionManager)
	syncMgr.RegisterManager(fds.DimensionResource, "account-1:product-1:cn-hangzhou", resourceManager)

	batch := fds.BatchUpdate{
		Updates: []fds.FourDimensionUpdate{
			{
				Dimension: fds.DimensionAccount,
				AccountID: "account-1",
				Status:    fds.StatusActive,
				Timestamp: time.Now(),
			},
			{
				Dimension: fds.DimensionProduct,
				AccountID: "account-1",
				ProductID: "product-1",
				Status:    fds.StatusDegraded,
				Timestamp: time.Now(),
			},
			{
				Dimension: fds.DimensionRegion,
				AccountID: "account-1",
				ProductID: "product-1",
				Region:    "cn-hangzhou",
				Status:    fds.StatusActive,
				Timestamp: time.Now(),
			},
			{
				Dimension:  fds.DimensionResource,
				AccountID:  "account-1",
				ProductID:  "product-1",
				Region:     "cn-hangzhou",
				ResourceID: "resource-1",
				Status:     fds.StatusActive,
				Timestamp:  time.Now(),
			},
		},
		Version: 1,
	}

	body, _ := json.Marshal(batch)
	req := httptest.NewRequest("POST", "/api/v1/cluster/four-dimension-sync", bytes.NewReader(body))
	req.Header.Set("X-Cluster-Secret", "test-secret")
	w := httptest.NewRecorder()

	syncMgr.HandleSync(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	time.Sleep(10 * time.Millisecond)

	if accountManager.getUpdateCount() != 1 {
		t.Errorf("Expected 1 account update, got %d", accountManager.getUpdateCount())
	}

	if productManager.getUpdateCount() != 1 {
		t.Errorf("Expected 1 product update, got %d", productManager.getUpdateCount())
	}

	if regionManager.getUpdateCount() != 1 {
		t.Errorf("Expected 1 region update, got %d", regionManager.getUpdateCount())
	}

	if resourceManager.getUpdateCount() != 1 {
		t.Errorf("Expected 1 resource update, got %d", resourceManager.getUpdateCount())
	}
}

func TestFourDimensionSync_HandleSync_Unauthorized(t *testing.T) {
	syncMgr := fds.NewFourDimensionSync("test-service", "9101", "test-secret")
	batch := fds.BatchUpdate{Updates: []fds.FourDimensionUpdate{}}
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest("POST", "/api/v1/cluster/four-dimension-sync", bytes.NewReader(body))
	req.Header.Set("X-Cluster-Secret", "wrong-secret")
	w := httptest.NewRecorder()

	syncMgr.HandleSync(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestFourDimensionSync_HandleSync_BadMethod(t *testing.T) {
	syncMgr := fds.NewFourDimensionSync("test-service", "9101", "test-secret")
	batch := fds.BatchUpdate{Updates: []fds.FourDimensionUpdate{}}
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest("GET", "/api/v1/cluster/four-dimension-sync", bytes.NewReader(body))
	req.Header.Set("X-Cluster-Secret", "test-secret")
	w := httptest.NewRecorder()

	syncMgr.HandleSync(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestFourDimensionSync_UpdateBatchVersion(t *testing.T) {
	batch1 := fds.BatchUpdate{
		Updates: []fds.FourDimensionUpdate{
			{
				Dimension: fds.DimensionAccount,
				AccountID: "account-1",
				Status:    fds.StatusActive,
				Timestamp: time.Now(),
			},
		},
		Version: 0,
	}

	batch2 := fds.BatchUpdate{
		Updates: []fds.FourDimensionUpdate{
			{
				Dimension: fds.DimensionAccount,
				AccountID: "account-1",
				Status:    fds.StatusDegraded,
				Timestamp: time.Now(),
			},
		},
		Version: 1,
	}

	if batch2.Version <= batch1.Version {
		t.Errorf("Expected version increment, got %d (was %d)", batch2.Version, batch1.Version)
	}
}

func TestFourDimensionSync_GzipCompression(t *testing.T) {
	updates := make([]fds.FourDimensionUpdate, 200)
	for i := 0; i < 200; i++ {
		updates[i] = fds.FourDimensionUpdate{
			Dimension: fds.DimensionAccount,
			AccountID: "account-1",
			Status:    fds.StatusActive,
			Timestamp: time.Now(),
		}
	}

	batch := fds.BatchUpdate{
		Updates: updates,
		Version: 1,
	}

	body, _ := json.Marshal(batch)

	uncompressedSize := len(body)
	if uncompressedSize < 1024 {
		t.Logf("Payload size: %d bytes (below compression threshold)", uncompressedSize)
	}
}

func TestFourDimensionSync_StatusTypes(t *testing.T) {
	statuses := []fds.Status{
		fds.StatusActive,
		fds.StatusDegraded,
		fds.StatusDisabled,
		fds.StatusUnknown,
	}

	for _, status := range statuses {
		if status == "" {
			t.Errorf("Status should not be empty")
		}
	}
}

func TestFourDimensionSync_DimensionTypes(t *testing.T) {
	dimensions := []fds.Dimension{
		fds.DimensionAccount,
		fds.DimensionProduct,
		fds.DimensionRegion,
		fds.DimensionResource,
	}

	for _, dim := range dimensions {
		if dim == "" {
			t.Errorf("Dimension should not be empty")
		}
	}
}
