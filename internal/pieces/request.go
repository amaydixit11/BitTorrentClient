package piece

import (
	"fmt"
	"sync"
	"time"
)

// RequestManager manages piece requests to peers
type RequestManager struct {
	mu             sync.RWMutex
	activeRequests map[string]*Request // key: "peerID:pieceIndex:begin"
	peerRequests   map[string]int      // track requests per peer
	maxRequests    int
}

// NewRequestManager creates a new request manager
func NewRequestManager(maxRequestsPerPeer int) *RequestManager {
	return &RequestManager{
		activeRequests: make(map[string]*Request),
		peerRequests:   make(map[string]int),
		maxRequests:    maxRequestsPerPeer,
	}
}

// CanRequestFromPeer checks if we can make more requests to a peer
func (rm *RequestManager) CanRequestFromPeer(peerID [20]byte) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	peerKey := string(peerID[:])
	return rm.peerRequests[peerKey] < rm.maxRequests
}

// AddRequest adds a new request
func (rm *RequestManager) AddRequest(peerID [20]byte, pieceIndex, begin int64, length int64) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	peerKey := string(peerID[:])

	// Check if peer has capacity
	if rm.peerRequests[peerKey] >= rm.maxRequests {
		return fmt.Errorf("peer has too many active requests")
	}

	// Create request
	req := &Request{
		PieceIndex: pieceIndex,
		Begin:      begin,
		Length:     length,
		Requested:  time.Now(),
		PeerID:     peerID,
	}

	// Store request
	key := fmt.Sprintf("%s:%d:%d", peerKey, pieceIndex, begin)
	rm.activeRequests[key] = req
	rm.peerRequests[peerKey]++

	return nil
}

// RemoveRequest removes a request
func (rm *RequestManager) RemoveRequest(peerID [20]byte, pieceIndex, begin int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	peerKey := string(peerID[:])
	key := fmt.Sprintf("%s:%d:%d", peerKey, pieceIndex, begin)

	if _, exists := rm.activeRequests[key]; exists {
		delete(rm.activeRequests, key)
		rm.peerRequests[peerKey]--

		// Clean up peer entry if no requests
		if rm.peerRequests[peerKey] <= 0 {
			delete(rm.peerRequests, peerKey)
		}
	}
}

// GetTimeoutRequests returns requests that have timed out
func (rm *RequestManager) GetTimeoutRequests(timeout time.Duration) []*Request {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var timeouts []*Request
	now := time.Now()

	for _, req := range rm.activeRequests {
		if now.Sub(req.Requested) > timeout {
			timeouts = append(timeouts, req)
		}
	}

	return timeouts
}

// ClearPeerRequests removes all requests for a peer (when peer disconnects)
func (rm *RequestManager) ClearPeerRequests(peerID [20]byte) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	peerKey := string(peerID[:])

	// Remove all requests for this peer
	for key, req := range rm.activeRequests {
		if string(req.PeerID[:]) == peerKey {
			delete(rm.activeRequests, key)
		}
	}

	// Clear peer request count
	delete(rm.peerRequests, peerKey)
}
