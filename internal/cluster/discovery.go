package cluster

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"multicloud-exporter/internal/logger"
)

// PeerDiscovery handles the discovery of other nodes in the cluster
type PeerDiscovery struct {
	serviceName string
	port        string
	localIPs    map[string]bool
	peers       []string
	mu          sync.RWMutex
	logger      *logger.ContextLogger
}

// NewPeerDiscovery creates a new peer discovery manager
func NewPeerDiscovery(serviceName, port string) *PeerDiscovery {
	if serviceName == "" {
		serviceName = "multicloud-exporter"
	}
	if port == "" {
		port = "9101"
	}

	pd := &PeerDiscovery{
		serviceName: serviceName,
		port:        port,
		localIPs:    make(map[string]bool),
		peers:       make([]string, 0),
		logger:      logger.NewContextLogger("Cluster", "component", "Discovery"),
	}

	pd.refreshLocalIPs()
	return pd
}

// refreshLocalIPs gets all local IP addresses to identify self
func (d *PeerDiscovery) refreshLocalIPs() {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		d.logger.Warnf("Failed to get interface addresses: %v", err)
		return
	}

	d.localIPs = make(map[string]bool)
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.To4() != nil {
				d.localIPs[ipnet.IP.String()] = true
			}
		}
	}
}

// Start starts the discovery loop
func (d *PeerDiscovery) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial discovery
	d.discover()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.refreshLocalIPs() // Interfaces might change
			d.discover()
		}
	}
}

// discover performs DNS lookup to find peers
func (d *PeerDiscovery) discover() {
	// Lookup the service name
	// In Kubernetes, a headless service will return multiple A records
	addrs, err := net.LookupHost(d.serviceName)
	if err != nil {
		// Only log debug/info on failure, as we might be in single-node mode
		// check if error is "no such host"
		d.logger.Debugf("Service lookup failed (%s): %v. Assuming single-node mode.", d.serviceName, err)

		d.mu.Lock()
		if len(d.peers) > 0 {
			d.logger.Infof("Lost all peers (lookup failed)")
		}
		d.peers = make([]string, 0)
		d.mu.Unlock()
		return
	}

	newPeers := make([]string, 0)
	for _, addr := range addrs {
		// Skip self
		if d.localIPs[addr] {
			continue
		}

		// Skip loopback unless we are testing (but localIPs should handle it if correctly detected)
		if addr == "127.0.0.1" || addr == "::1" {
			continue
		}

		peerURL := fmt.Sprintf("http://%s:%s", addr, d.port)
		newPeers = append(newPeers, peerURL)
	}

	d.mu.Lock()
	oldPeers := d.peers
	d.peers = newPeers
	d.mu.Unlock()

	// Log if peers changed
	if len(newPeers) != len(oldPeers) || !stringSlicesEqual(oldPeers, newPeers) {
		d.logger.Infof("Cluster peers updated: found %d peers %v", len(newPeers), newPeers)
	}
}

// GetPeers returns the current list of peer URLs
func (d *PeerDiscovery) GetPeers() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Return a copy
	peers := make([]string, len(d.peers))
	copy(peers, d.peers)
	return peers
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// O(n^2) is fine for small number of peers
	for _, v1 := range a {
		found := false
		for _, v2 := range b {
			if v1 == v2 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
