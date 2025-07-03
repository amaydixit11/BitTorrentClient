package peer

import (
	"fmt"
	"io"
	"net"
	"time"
)

const (
	ProtocolString = "BitTorrent protocol"
	HandshakeSize  = 49 + len(ProtocolString)
)

// Handshake represents the BitTorrent handshake message
type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

// NewHandshake creates a new handshake
func NewHandshake(infoHash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     ProtocolString,
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

// Serialize converts handshake to bytes
func (h *Handshake) Serialize() []byte {
	buf := make([]byte, HandshakeSize)
	curr := 0

	// Protocol string length
	buf[curr] = byte(len(h.Pstr))
	curr++

	// Protocol string
	copy(buf[curr:], h.Pstr)
	curr += len(h.Pstr)

	// Reserved bytes (8 zeros)
	curr += 8

	// Info hash
	copy(buf[curr:], h.InfoHash[:])
	curr += 20

	// Peer ID
	copy(buf[curr:], h.PeerID[:])

	return buf
}

// Deserialize parses handshake from bytes
func DeserializeHandshake(data []byte) (*Handshake, error) {
	if len(data) < HandshakeSize {
		return nil, fmt.Errorf("handshake too short: %d bytes", len(data))
	}

	curr := 0

	// Protocol string length
	pstrLen := int(data[curr])
	curr++

	if pstrLen != len(ProtocolString) {
		return nil, fmt.Errorf("invalid protocol string length: %d", pstrLen)
	}

	// Protocol string
	pstr := string(data[curr : curr+pstrLen])
	curr += pstrLen

	if pstr != ProtocolString {
		return nil, fmt.Errorf("invalid protocol string: %s", pstr)
	}

	// Skip reserved bytes
	curr += 8

	// Info hash
	var infoHash [20]byte
	copy(infoHash[:], data[curr:curr+20])
	curr += 20

	// Peer ID
	var peerID [20]byte
	copy(peerID[:], data[curr:curr+20])

	return &Handshake{
		Pstr:     pstr,
		InfoHash: infoHash,
		PeerID:   peerID,
	}, nil
}

// PerformHandshake performs handshake with a peer
func PerformHandshake(conn net.Conn, infoHash, peerID [20]byte) (*Handshake, error) {
	// Set deadline for handshake
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// Send our handshake
	req := NewHandshake(infoHash, peerID)
	_, err := conn.Write(req.Serialize())
	if err != nil {
		return nil, fmt.Errorf("failed to send handshake: %w", err)
	}

	// Read peer's handshake - first read pstrlen to determine total size
	pstrLenBuf := make([]byte, 1)
	_, err = io.ReadFull(conn, pstrLenBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read pstrlen: %w", err)
	}

	pstrLen := int(pstrLenBuf[0])
	if pstrLen != 19 {
		return nil, fmt.Errorf("invalid protocol string length: %d", pstrLen)
	}

	// Read the rest of the handshake
	remaining := make([]byte, pstrLen+8+20+20) // pstr + reserved + info_hash + peer_id
	_, err = io.ReadFull(conn, remaining)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake: %w", err)
	}

	// Combine for deserialization
	buf := append(pstrLenBuf, remaining...)
	// _, err = io.ReadFull(conn, buf)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to read handshake: %w", err)
	// }

	// Deserialize peer's handshake
	res, err := DeserializeHandshake(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse handshake: %w", err)
	}

	// Verify info hash matches
	if res.InfoHash != infoHash {
		return nil, fmt.Errorf("info hash mismatch")
	}

	return res, nil
}
