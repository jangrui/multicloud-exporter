package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"multicloud-exporter/internal/logger"
)

// RegionManagerInterface defines the interface for updating region status
// Using string for status to avoid circular dependency with common package
type RegionManagerInterface interface {
	// UpdateFromPeer updates the region status from a peer (without broadcasting)
	UpdateFromPeer(accountID, region string, resourceCount int, status string)
}

// SyncManager manages cluster synchronization
type SyncManager struct {
	discovery *PeerDiscovery
	secret    string
	managers  map[string]RegionManagerInterface
	mu        sync.RWMutex
	client    *http.Client
	logger    *logger.ContextLogger
}

// NewSyncManager creates a new synchronization manager
func NewSyncManager(serviceName, port, secret string) *SyncManager {
	if secret == "" {
		secret = os.Getenv("CLUSTER_SECRET")
	}
	return &SyncManager{
		discovery: NewPeerDiscovery(serviceName, port),
		secret:    secret,
		managers:  make(map[string]RegionManagerInterface),
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
		logger: logger.NewContextLogger("Cluster", "component", "SyncManager"),
	}
}

// Start starts the peer discovery
func (m *SyncManager) Start(ctx context.Context) {
	m.discovery.Start(ctx)
}

// RegisterRegionManager registers a region manager for a provider
func (m *SyncManager) RegisterRegionManager(provider string, rm RegionManagerInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.managers[provider] = rm
}

// BroadcastRegionStatus sends status update to all peers
func (m *SyncManager) BroadcastRegionStatus(provider, accountID, region, status string, resourceCount int) {
	peers := m.discovery.GetPeers()
	if len(peers) == 0 {
		return
	}

	payload := RegionStatusUpdate{
		Provider:      provider,
		AccountID:     accountID,
		Region:        region,
		Status:        status,
		ResourceCount: resourceCount,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Errorf("Failed to marshal update: %v", err)
		return
	}

	// Async broadcast
	go func() {
		var wg sync.WaitGroup
		for _, peer := range peers {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				m.sendUpdate(url, data)
			}(peer)
		}
		wg.Wait()
	}()
}

func (m *SyncManager) sendUpdate(peerURL string, data []byte) {
	url := peerURL + "/api/v1/cluster/sync"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if m.secret != "" {
		req.Header.Set("X-Cluster-Secret", m.secret)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		// Log at debug level to avoid spamming logs when peers are unstable
		m.logger.Debugf("Failed to send update to %s: %v", url, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		m.logger.Debugf("Peer %s returned status %d", url, resp.StatusCode)
	}
}

// HandleSync handles incoming sync requests
func (m *SyncManager) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.secret != "" {
		secret := r.Header.Get("X-Cluster-Secret")
		if secret != m.secret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var update RegionStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.RLock()
	rm, ok := m.managers[update.Provider]
	m.mu.RUnlock()

	if !ok {
		// Provider not found, ignore
		w.WriteHeader(http.StatusOK)
		return
	}

	rm.UpdateFromPeer(update.AccountID, update.Region, update.ResourceCount, update.Status)
	w.WriteHeader(http.StatusOK)
}
