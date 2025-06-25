package tracker

import (
	"fmt"
	"net"
)

type PeerID [20]byte
type InfoHash [20]byte
type Port uint16

type Event string

const (
	EventStarted   Event = "started"
	EventStopped   Event = "stopped"
	EventCompleted Event = "completed"
	EventNone      Event = ""
)

type Peer struct {
	IP   net.IP
	Port Port
}

type TrackerRequest struct {
	AnnounceURL string
	InfoHash    InfoHash
	PeerID      PeerID
	Port        Port
	Uploaded    int64
	Downloaded  int64
	Left        int64
	Event       Event
	NumWant     int
}

type TrackerResponse struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"` // compact format only for now
}

// For string formatting
func (id PeerID) String() string {
	return fmt.Sprintf("%x", id[:])
}
