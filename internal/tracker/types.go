package tracker

import (
	"fmt"
	"net"
	"net/http"
)

// TrackerClient handles communication with BitTorrent trackers
type TrackerClient struct {
	httpClient *http.Client
	peerID     []byte
	port       int
}

// TrackerRequest represents the parameters sent to the tracker
type TrackerRequest struct {
	InfoHash   []byte
	PeerID     []byte
	Port       int
	Uploaded   int64
	Downloaded int64
	Left       int64
	Compact    bool
	NoPeerID   bool
	Event      string // "started", "stopped", "completed", or empty
	IP         string // Optional
	NumWant    int    // Optional, defaults to 50
	Key        string // Optional
	TrackerID  string // Optional
}

// TrackerResponse represents the response from the tracker
type TrackerResponse struct {
	FailureReason  string      `bencode:"failure reason,omitempty"`
	WarningMessage string      `bencode:"warning message,omitempty"`
	Interval       int         `bencode:"interval"`
	MinInterval    int         `bencode:"min interval,omitempty"`
	TrackerID      string      `bencode:"tracker id,omitempty"`
	Complete       int         `bencode:"complete"`
	Incomplete     int         `bencode:"incomplete"`
	Peers          []Peer      `bencode:"-"`     // Parsed peers
	RawPeers       interface{} `bencode:"peers"` // Raw peers data
}

// Peer represents a peer in the swarm
type Peer struct {
	ID   []byte // May be empty if no_peer_id was set
	IP   net.IP
	Port int
}

// String returns a string representation of the peer
func (p Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP.String(), p.Port)
}
