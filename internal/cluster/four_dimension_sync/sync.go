package four_dimension_sync

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
)

// Dimension represents four dimension layers
type Dimension string

const (
	DimensionAccount  Dimension = "account"
	DimensionProduct  Dimension = "product"
	DimensionRegion   Dimension = "region"
	DimensionResource Dimension = "resource"
)

// Status represents four-dimension status
type Status string

const (
	StatusActive   Status = "active"
	StatusDegraded Status = "degraded"
	StatusDisabled Status = "disabled"
	StatusUnknown  Status = "unknown"
)

// FourDimensionUpdate represents a four-dimension status update
type FourDimensionUpdate struct {
	Dimension     Dimension `json:"dimension"`
	AccountID     string    `json:"account_id"`
	ProductID     string    `json:"product_id,omitempty"`
	Region        string    `json:"region,omitempty"`
	ResourceID    string    `json:"resource_id,omitempty"`
	Status        Status    `json:"status"`
	ResourceCount int       `json:"resource_count,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// BatchUpdate represents a batch of four-dimension updates
type BatchUpdate struct {
	Updates []FourDimensionUpdate `json:"updates"`
	Version int64                 `json:"version"`
}

// DimensionManagerInterface defines interface for receiving updates from peers
type DimensionManagerInterface interface {
	UpdateFromPeer(update FourDimensionUpdate)
}

// FourDimensionSync manages four-dimension cluster synchronization
type FourDimensionSync struct {
	discover *cluster.PeerDiscovery
	secret   string
	managers map[Dimension]map[string]DimensionManagerInterface
	mu       sync.RWMutex
	client   *http.Client
	logger   *logger.ContextLogger

	// Aggregation for reducing message count
	updateBuffer   []FourDimensionUpdate
	bufferMu       sync.Mutex
	bufferSize     int
	flushInterval  time.Duration
	lastFlush      time.Time
	versionCounter int64

	// Compression settings
	compressThreshold int // bytes
}

// NewFourDimensionSync creates a new four-dimension synchronization manager
func NewFourDimensionSync(serviceName, port, secret string) *FourDimensionSync {
	if secret == "" {
		secret = "default-cluster-secret"
	}
	return &FourDimensionSync{
		discover: cluster.NewPeerDiscovery(serviceName, port),
		secret:   secret,
		managers: make(map[Dimension]map[string]DimensionManagerInterface),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger:            logger.NewContextLogger("Cluster", "component", "FourDimensionSync"),
		updateBuffer:      make([]FourDimensionUpdate, 0, 100),
		bufferSize:        100,
		flushInterval:     2 * time.Second,
		lastFlush:         time.Now(),
		compressThreshold: 1024, // Compress if payload >1KB
	}
}

// Start starts four-dimension synchronization
func (f *FourDimensionSync) Start(ctx context.Context) {
	f.discover.Start(ctx)
	go f.flushBufferLoop(ctx)
}

// RegisterManager registers a manager for a dimension
func (f *FourDimensionSync) RegisterManager(dimension Dimension, key string, manager DimensionManagerInterface) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.managers[dimension] == nil {
		f.managers[dimension] = make(map[string]DimensionManagerInterface)
	}
	f.managers[dimension][key] = manager
}

// BroadcastAccountStatus broadcasts account status update
func (f *FourDimensionSync) BroadcastAccountStatus(accountID string, status Status, resourceCount int) {
	update := FourDimensionUpdate{
		Dimension:     DimensionAccount,
		AccountID:     accountID,
		Status:        status,
		ResourceCount: resourceCount,
		Timestamp:     time.Now(),
	}
	f.bufferUpdate(update)
}

// BroadcastProductStatus broadcasts product status update
func (f *FourDimensionSync) BroadcastProductStatus(accountID, productID string, status Status, resourceCount int) {
	update := FourDimensionUpdate{
		Dimension:     DimensionProduct,
		AccountID:     accountID,
		ProductID:     productID,
		Status:        status,
		ResourceCount: resourceCount,
		Timestamp:     time.Now(),
	}
	f.bufferUpdate(update)
}

// BroadcastRegionStatus broadcasts region status update
func (f *FourDimensionSync) BroadcastRegionStatus(accountID, productID, region string, status Status, resourceCount int) {
	update := FourDimensionUpdate{
		Dimension:     DimensionRegion,
		AccountID:     accountID,
		ProductID:     productID,
		Region:        region,
		Status:        status,
		ResourceCount: resourceCount,
		Timestamp:     time.Now(),
	}
	f.bufferUpdate(update)
}

// BroadcastResourceStatus broadcasts resource status update
func (f *FourDimensionSync) BroadcastResourceStatus(accountID, productID, region, resourceID string, status Status) {
	update := FourDimensionUpdate{
		Dimension:  DimensionResource,
		AccountID:  accountID,
		ProductID:  productID,
		Region:     region,
		ResourceID: resourceID,
		Status:     status,
		Timestamp:  time.Now(),
	}
	f.bufferUpdate(update)
}

// bufferUpdate adds update to buffer for aggregation
func (f *FourDimensionSync) bufferUpdate(update FourDimensionUpdate) {
	f.bufferMu.Lock()
	defer f.bufferMu.Unlock()

	f.updateBuffer = append(f.updateBuffer, update)

	if len(f.updateBuffer) >= f.bufferSize {
		go f.flushBuffer()
	}
}

// flushBufferLoop periodically flushes buffer
func (f *FourDimensionSync) flushBufferLoop(ctx context.Context) {
	ticker := time.NewTicker(f.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			f.flushBuffer()
			return
		case <-ticker.C:
			f.flushBuffer()
		}
	}
}

// flushBuffer sends all buffered updates to peers
func (f *FourDimensionSync) flushBuffer() {
	f.bufferMu.Lock()
	if len(f.updateBuffer) == 0 {
		f.bufferMu.Unlock()
		return
	}

	updates := make([]FourDimensionUpdate, len(f.updateBuffer))
	copy(updates, f.updateBuffer)
	f.updateBuffer = f.updateBuffer[:0]
	f.bufferMu.Unlock()

	if len(updates) == 0 {
		return
	}

	batch := BatchUpdate{
		Updates: updates,
		Version: f.versionCounter,
	}
	f.versionCounter++

	f.broadcastBatch(batch)
}

// broadcastBatch sends batch updates to all peers
func (f *FourDimensionSync) broadcastBatch(batch BatchUpdate) {
	peers := f.discover.GetPeers()
	if len(peers) == 0 {
		return
	}

	data, err := json.Marshal(batch)
	if err != nil {
		f.logger.Errorf("Failed to marshal batch: %v", err)
		return
	}

	compressed := false
	if len(data) > f.compressThreshold {
		compressedData := new(bytes.Buffer)
		gz := gzip.NewWriter(compressedData)
		if _, err := gz.Write(data); err == nil {
			if err := gz.Close(); err == nil {
				data = compressedData.Bytes()
				compressed = true
			}
		}
	}

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			_ = f.sendBatchWithContext(context.Background(), url, data, compressed)
		}(peer)
	}
	wg.Wait()

	if metrics.FourDimensionSyncTotal != nil {
		metrics.FourDimensionSyncTotal.WithLabelValues("batch").Inc()
	}
}

// sendBatchWithContext sends batch update to a peer with context and retry
func (f *FourDimensionSync) sendBatchWithContext(ctx context.Context, peerURL string, data []byte, compressed bool) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, "POST", peerURL+"/api/v1/cluster/four-dimension-sync", bytes.NewReader(data))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		if f.secret != "" {
			req.Header.Set("X-Cluster-Secret", f.secret)
		}
		if compressed {
			req.Header.Set("Content-Encoding", "gzip")
		}

		start := time.Now()
		resp, err := f.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if metrics.FourDimensionSyncDurationSeconds != nil {
					metrics.FourDimensionSyncDurationSeconds.WithLabelValues("").Observe(time.Since(start).Seconds())
				}
				return nil
			}
		} else {
			lastErr = err
		}

		if attempt < maxRetries-1 {
			sleep := time.Duration(100*(1<<attempt)) * time.Millisecond
			if sleep > time.Second {
				sleep = time.Second
			}
			time.Sleep(sleep)
		}
	}

	if lastErr != nil && metrics.BroadcastFailedTotal != nil {
		metrics.BroadcastFailedTotal.WithLabelValues("four-dimension").Inc()
	}
	return lastErr
}

// HandleSync handles incoming four-dimension sync requests
func (f *FourDimensionSync) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if f.secret != "" {
		secret := r.Header.Get("X-Cluster-Secret")
		if secret != f.secret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	data, err := f.readRequestBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var batch BatchUpdate
	if err := json.Unmarshal(data, &batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, update := range batch.Updates {
		f.processUpdate(update)
	}

	w.WriteHeader(http.StatusOK)
}

// readRequestBody reads and potentially decompresses request body
func (f *FourDimensionSync) readRequestBody(r *http.Request) ([]byte, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()

	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gz.Close() }()
		return io.ReadAll(gz)
	}

	return data, nil
}

// processUpdate processes a single update
func (f *FourDimensionSync) processUpdate(update FourDimensionUpdate) {
	f.mu.RLock()
	managers, ok := f.managers[update.Dimension]
	f.mu.RUnlock()

	if !ok {
		return
	}

	var key string
	switch update.Dimension {
	case DimensionAccount, DimensionProduct:
		key = update.AccountID
	case DimensionRegion:
		key = update.AccountID + ":" + update.ProductID
	case DimensionResource:
		key = update.AccountID + ":" + update.ProductID + ":" + update.Region
	}

	f.mu.RLock()
	manager, ok := managers[key]
	f.mu.RUnlock()

	if ok {
		manager.UpdateFromPeer(update)
	}
}
