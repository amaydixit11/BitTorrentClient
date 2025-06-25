package tracker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"bittorrentclient/internal/bencode" // replace with actual import
)

// ParseTrackerResponse parses the tracker HTTP response into interval and compact peer list string
func ParseTrackerResponse(data []byte) (int, string, error) {
	root, err := bencode.Decode(data)
	if err != nil {
		return 0, "", fmt.Errorf("failed to decode bencoded response: %w", err)
	}

	dict, ok := root.(map[string]interface{})
	if !ok {
		return 0, "", errors.New("expected top-level dictionary in tracker response")
	}

	// Required field: interval
	intervalVal, ok := dict["interval"]
	if !ok {
		return 0, "", errors.New("missing 'interval' in tracker response")
	}

	interval, ok := intervalVal.(int64)
	if !ok {
		return 0, "", fmt.Errorf("invalid type for 'interval': %T", intervalVal)
	}

	// Required field: peers (compact string)
	peersVal, ok := dict["peers"]
	if !ok {
		return 0, "", errors.New("missing 'peers' in tracker response")
	}

	peers, ok := peersVal.(string)
	if !ok {
		return 0, "", fmt.Errorf("invalid type for 'peers': %T", peersVal)
	}

	return int(interval), peers, nil
}

// DecodePeers parses a compact peers string into a list of Peer structs
func DecodePeers(compact string) ([]Peer, error) {
	raw := []byte(compact)
	if len(raw)%6 != 0 {
		return nil, fmt.Errorf("invalid compact peers length: %d", len(raw))
	}

	var peers []Peer
	for i := 0; i < len(raw); i += 6 {
		ip := net.IPv4(raw[i], raw[i+1], raw[i+2], raw[i+3])
		port := binary.BigEndian.Uint16(raw[i+4 : i+6])
		peers = append(peers, Peer{IP: ip, Port: Port(port)})
	}
	return peers, nil
}
