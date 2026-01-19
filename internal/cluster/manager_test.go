package cluster

import (
	"bytes"
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
	peerSync.RegisterProductRegionManager("aliyun", "slb", mockRM)
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
	senderSync.BroadcastRegionStatus("aliyun", "slb", "acc1", "us-east-1", "empty", 0)

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

func TestSyncManager_RegisterProductRegionManager(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "")

	if _, exists := syncMgr.managers["aliyun:slb"]; exists {
		t.Errorf("Expected no manager to exist before registration")
	}

	mockRM := &mockRegionManager{}
	syncMgr.RegisterProductRegionManager("aliyun", "slb", mockRM)

	syncMgr.mu.RLock()
	defer syncMgr.mu.RUnlock()
	if _, exists := syncMgr.managers["aliyun:slb"]; !exists {
		t.Errorf("Expected manager to exist after registration")
	}
}

func TestSyncManager_BroadcastNoPeers(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "")

	// Should not panic when no peers
	syncMgr.BroadcastRegionStatus("aliyun", "slb", "acc1", "us-east-1", "empty", 0)
}

func TestSyncManager_HandleSync_MethodNotAllowed(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "")

	req := httptest.NewRequest("GET", "/api/v1/cluster/sync", nil)
	rr := httptest.NewRecorder()

	syncMgr.HandleSync(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestSyncManager_HandleSync_Unauthorized(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "secret")

	req := httptest.NewRequest("POST", "/api/v1/cluster/sync", nil)
	rr := httptest.NewRecorder()

	syncMgr.HandleSync(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestSyncManager_HandleSync_InvalidBody(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "secret")

	req := httptest.NewRequest("POST", "/api/v1/cluster/sync", nil)
	req.Header.Set("X-Cluster-Secret", "secret")
	rr := httptest.NewRecorder()

	syncMgr.HandleSync(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestSyncManager_HandleSync_ProviderNotFound(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "secret")
	mockRM := &mockRegionManager{}
	syncMgr.RegisterProductRegionManager("aliyun", "slb", mockRM)

	body := `{"provider":"unknown","product":"slb","account_id":"acc1","region":"us-east-1","status":"empty","resource_count":0}`
	req := httptest.NewRequest("POST", "/api/v1/cluster/sync", bytes.NewBufferString(body))
	req.Header.Set("X-Cluster-Secret", "secret")
	rr := httptest.NewRecorder()

	syncMgr.HandleSync(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestSyncManager_NewSyncManager(t *testing.T) {
	syncMgr := NewSyncManager("test", "0", "secret")

	if syncMgr.discovery == nil {
		t.Errorf("Expected discovery to be initialized")
	}
	if syncMgr.secret != "secret" {
		t.Errorf("Expected secret to be set")
	}
	if syncMgr.managers == nil {
		t.Errorf("Expected managers map to be initialized")
	}
	if syncMgr.client == nil {
		t.Errorf("Expected HTTP client to be initialized")
	}
	if syncMgr.client.Timeout != 2*time.Second {
		t.Errorf("Expected timeout to be 2s")
	}
}

func TestPeerDiscovery_NewPeerDiscovery(t *testing.T) {
	pd := NewPeerDiscovery("test", "9101")

	if pd.serviceName != "test" {
		t.Errorf("Expected serviceName to be 'test', got %s", pd.serviceName)
	}
	if pd.port != "9101" {
		t.Errorf("Expected port to be '9101', got %s", pd.port)
	}
	if pd.peers == nil {
		t.Errorf("Expected peers to be initialized")
	}
	if pd.localIPs == nil {
		t.Errorf("Expected localIPs to be initialized")
	}
}

func TestPeerDiscovery_NewPeerDiscovery_EmptyValues(t *testing.T) {
	pd := NewPeerDiscovery("", "")

	if pd.serviceName != "multicloud-exporter" {
		t.Errorf("Expected default serviceName, got %s", pd.serviceName)
	}
	if pd.port != "9101" {
		t.Errorf("Expected default port, got %s", pd.port)
	}
}

func TestPeerDiscovery_GetPeers(t *testing.T) {
	pd := NewPeerDiscovery("test", "0")

	// Initially no peers
	if peers := pd.GetPeers(); len(peers) != 0 {
		t.Errorf("Expected no peers initially, got %d", len(peers))
	}

	// Add some peers
	pd.mu.Lock()
	pd.peers = []string{"http://peer1:9101", "http://peer2:9101"}
	pd.mu.Unlock()

	peers := pd.GetPeers()
	if len(peers) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(peers))
	}
}

func TestPeerDiscovery_GetPeers_ReturnsCopy(t *testing.T) {
	pd := NewPeerDiscovery("test", "0")

	pd.mu.Lock()
	pd.peers = []string{"http://peer1:9101"}
	pd.mu.Unlock()

	peers1 := pd.GetPeers()
	peers1[0] = "modified"

	peers2 := pd.GetPeers()
	if peers2[0] != "http://peer1:9101" {
		t.Errorf("Expected original peer URL, got %s", peers2[0])
	}
}

func TestStringSlicesEqual(t *testing.T) {
	tests := []struct {
		a        []string
		b        []string
		expected bool
	}{
		{nil, nil, true},
		{[]string{}, []string{}, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a"}, []string{"b"}, false},
		{[]string{"a", "b"}, []string{"a"}, false},
		{[]string{"a"}, []string{"a", "b"}, false},
	}

	for i, tt := range tests {
		result := stringSlicesEqual(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Test %d: stringSlicesEqual(%v, %v) = %v, want %v", i, tt.a, tt.b, result, tt.expected)
		}
	}
}
