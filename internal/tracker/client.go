package tracker

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"
	// Adjust import path
)

// NewTrackerClient creates a new tracker client
func NewTrackerClient(port int) *TrackerClient {
	return &TrackerClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		peerID: generatePeerID(),
		port:   port,
	}
}

// generatePeerID generates a random 20-byte peer ID
// Format: -GT0001-<12 random bytes> (GT = Go Torrent, 0001 = version)
func generatePeerID() []byte {
	peerID := make([]byte, 20)
	copy(peerID, "-GT0001-")

	randomBytes := make([]byte, 12)
	rand.Read(randomBytes)
	copy(peerID[8:], randomBytes)

	return peerID
}

// parseTrackerResponse parses the bencode dictionary from the tracker
func (tc *TrackerClient) parseTrackerResponse(dict map[string]interface{}) (*TrackerResponse, error) {
	resp := &TrackerResponse{}

	// Check for failure reason
	if failureReason, ok := dict["failure reason"].(string); ok {
		resp.FailureReason = failureReason
		return resp, nil // Don't parse other fields if there's a failure
	}

	// Warning message (optional)
	if warningMsg, ok := dict["warning message"].(string); ok {
		resp.WarningMessage = warningMsg
	}

	// Interval (required)
	if interval, ok := dict["interval"].(int64); ok {
		resp.Interval = int(interval)
	} else {
		return nil, fmt.Errorf("missing or invalid interval in tracker response")
	}

	// Min interval (optional)
	if minInterval, ok := dict["min interval"].(int64); ok {
		resp.MinInterval = int(minInterval)
	}

	// Tracker ID (optional)
	if trackerID, ok := dict["tracker id"].(string); ok {
		resp.TrackerID = trackerID
	}

	// Complete (seeders)
	if complete, ok := dict["complete"].(int64); ok {
		resp.Complete = int(complete)
	}

	// Incomplete (leechers)
	if incomplete, ok := dict["incomplete"].(int64); ok {
		resp.Incomplete = int(incomplete)
	}

	// Parse peers
	if peersData, ok := dict["peers"]; ok {
		peers, err := tc.parsePeers(peersData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse peers: %v", err)
		}
		resp.Peers = peers
		resp.RawPeers = peersData
	}

	return resp, nil
}

// Close cleans up the tracker client
func (tc *TrackerClient) Close() {
	if tc.httpClient != nil {
		tc.httpClient.CloseIdleConnections()
	}
}
