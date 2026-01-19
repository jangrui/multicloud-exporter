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
	"multicloud-exporter/internal/metrics"
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

// RegisterProductRegionManager registers a region manager for a provider:product combination
func (m *SyncManager) RegisterProductRegionManager(provider, product string, rm RegionManagerInterface) {
	key := provider + ":" + product
	m.mu.Lock()
	defer m.mu.Unlock()
	m.managers[key] = rm
}

// BroadcastRegionStatus sends status update to all peers
func (m *SyncManager) BroadcastRegionStatus(provider, product, accountID, region, status string, resourceCount int) {
	peers := m.discovery.GetPeers()
	if len(peers) == 0 {
		return
	}

	payload := RegionStatusUpdate{
		Provider:      provider,
		Product:       product,
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

	// 为每个 peer 发送更新，带超时和重试
	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			// 最多重试 3 次
			const maxRetries = 3
			var lastErr error
			for attempt := 0; attempt < maxRetries; attempt++ {
				// 创建带超时的 context（5s 超时）
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// 发送请求
				err := m.sendUpdateWithContext(ctx, url, data)
				if err == nil {
					m.logger.Debugf("成功发送区域状态更新到 peer: %s (attempt=%d)", url, attempt+1)
					break
				}
				lastErr = err

				// 如果是最后一次尝试，记录错误
				if attempt == maxRetries-1 {
					m.logger.Warnf("发送区域状态更新到 peer 失败（已重试%d次）: %s 错误: %v", maxRetries, url, lastErr)
					metrics.BroadcastFailedTotal.WithLabelValues(url).Inc()
				}

				// 如果不是最后一次尝试，等待后重试（指数退避）
				if attempt < maxRetries-1 {
					sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
					if sleep > 1*time.Second {
						sleep = 1 * time.Second
					}
					time.Sleep(sleep)
				}
			}
		}(peer)
	}
	wg.Wait()
}

func (m *SyncManager) sendUpdateWithContext(ctx context.Context, peerURL string, data []byte) error {
	url := peerURL + "/api/v1/cluster/sync"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if m.secret != "" {
		req.Header.Set("X-Cluster-Secret", m.secret)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		m.logger.Debugf("Peer %s returned status %d", url, resp.StatusCode)
	}
	return nil
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

	// Build key as {provider}:{product}
	key := update.Provider
	if update.Product != "" {
		key += ":" + update.Product
	}

	m.mu.RLock()
	rm, ok := m.managers[key]
	m.mu.RUnlock()

	if !ok {
		// Provider:product not found, ignore
		w.WriteHeader(http.StatusOK)
		return
	}

	rm.UpdateFromPeer(update.AccountID, update.Region, update.ResourceCount, update.Status)
	w.WriteHeader(http.StatusOK)
}
