package cluster

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mockRegionManager struct {
	mu            sync.Mutex
	lastAccountID string
	lastRegion    string
	lastStatus    string
}

func (m *mockRegionManager) UpdateFromPeer(accountID, region string, resourceCount int, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAccountID = accountID
	m.lastRegion = region
	m.lastStatus = status
}

func (m *mockRegionManager) getLast() (string, string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastAccountID, m.lastRegion, m.lastStatus
}

func TestSyncManager_BroadcastAndReceive(t *testing.T) {
	// 1. Setup Mock Peer
	mockRM := &mockRegionManager{}

	peerMux := http.NewServeMux()
	peerSync := NewSyncManager("test", "0", "secret")
	peerSync.RegisterRegionManager("aliyun", mockRM)
	peerMux.HandleFunc("/api/v1/cluster/sync", peerSync.HandleSync)

	peerServer := httptest.NewServer(peerMux)
	defer peerServer.Close()

	// 2. Setup Sender
	senderSync := NewSyncManager("test", "0", "secret")
	// Hack: manually set peer list
	senderSync.discovery.mu.Lock()
	senderSync.discovery.peers = []string{peerServer.URL}
	senderSync.discovery.mu.Unlock()

	// 3. Broadcast
	senderSync.BroadcastRegionStatus("aliyun", "acc1", "us-east-1", "empty", 0)

	// 4. Wait for async processing
	time.Sleep(200 * time.Millisecond)

	// 5. Verify
	lastAccountID, lastRegion, lastStatus := mockRM.getLast()
	if lastAccountID != "acc1" {
		t.Errorf("Expected acc1, got %s", lastAccountID)
	}
	if lastRegion != "us-east-1" {
		t.Errorf("Expected us-east-1, got %s", lastRegion)
	}
	if lastStatus != "empty" {
		t.Errorf("Expected empty, got %s", lastStatus)
	}
}
