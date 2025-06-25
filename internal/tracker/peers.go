package tracker

import (
	"encoding/binary"
	"fmt"
	"net"
)

// parsePeers handles both dictionary and binary peer formats
func (tc *TrackerClient) parsePeers(peersData interface{}) ([]Peer, error) {
	switch data := peersData.(type) {
	case string:
		// Binary model: 6 bytes per peer (4 bytes IP + 2 bytes port)
		return tc.parseBinaryPeers([]byte(data))
	case []interface{}:
		// Dictionary model: list of dictionaries
		return tc.parseDictPeers(data)
	default:
		return nil, fmt.Errorf("unsupported peers format")
	}
}

// parseBinaryPeers parses peers in binary format
func (tc *TrackerClient) parseBinaryPeers(data []byte) ([]Peer, error) {
	if len(data)%6 != 0 {
		return nil, fmt.Errorf("invalid binary peers data length: %d", len(data))
	}

	numPeers := len(data) / 6
	peers := make([]Peer, numPeers)

	for i := 0; i < numPeers; i++ {
		offset := i * 6

		// Parse IP (4 bytes, network byte order)
		ip := net.IPv4(data[offset], data[offset+1], data[offset+2], data[offset+3])

		// Parse port (2 bytes, network byte order)
		port := binary.BigEndian.Uint16(data[offset+4 : offset+6])

		peers[i] = Peer{
			IP:   ip,
			Port: int(port),
		}
	}

	return peers, nil
}

// parseDictPeers parses peers in dictionary format
func (tc *TrackerClient) parseDictPeers(data []interface{}) ([]Peer, error) {
	peers := make([]Peer, len(data))

	for i, peerData := range data {
		peerDict, ok := peerData.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("peer %d is not a dictionary", i)
		}

		// Parse IP
		ipStr, ok := peerDict["ip"].(string)
		if !ok {
			return nil, fmt.Errorf("peer %d missing IP", i)
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("peer %d has invalid IP: %s", i, ipStr)
		}

		// Parse port
		portInt, ok := peerDict["port"].(int64)
		if !ok {
			return nil, fmt.Errorf("peer %d missing or invalid port", i)
		}

		peers[i] = Peer{
			IP:   ip,
			Port: int(portInt),
		}

		// Parse peer ID if present
		if peerID, ok := peerDict["peer id"].(string); ok {
			peers[i].ID = []byte(peerID)
		}
	}

	return peers, nil
}

// GetPeers is a convenience method for getting peers for a torrent
func (tc *TrackerClient) GetPeers(announceURL string, infoHash []byte, left int64) ([]Peer, error) {
	req := &TrackerRequest{
		InfoHash:   infoHash,
		PeerID:     tc.peerID,
		Port:       tc.port,
		Uploaded:   0,
		Downloaded: 0,
		Left:       left,
		Compact:    true, // Request compact format for efficiency
		Event:      "started",
		NumWant:    50,
	}

	resp, err := tc.Announce(announceURL, req)
	if err != nil {
		return nil, err
	}

	if resp.FailureReason != "" {
		return nil, fmt.Errorf("tracker error: %s", resp.FailureReason)
	}

	return resp.Peers, nil
}
