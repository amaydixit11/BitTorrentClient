package peer

import (
	"net"
	"time"
)

// Peer represents a connected peer
type Peer struct {
	Conn        net.Conn
	ID          [20]byte
	InfoHash    [20]byte
	Choked      bool
	Choking     bool
	Interested  bool
	Interesting bool
	Bitfield    []byte
}

// NewPeer creates a new peer connection
func NewPeer(conn net.Conn, infoHash [20]byte) *Peer {
	return &Peer{
		Conn:        conn,
		InfoHash:    infoHash,
		Choked:      true,
		Choking:     true,
		Interested:  false,
		Interesting: false,
	}
}

// Close closes the peer connection
func (p *Peer) Close() error {
	return p.Conn.Close()
}

// SetDeadline sets read/write deadline for the connection
func (p *Peer) SetDeadline(t time.Time) error {
	return p.Conn.SetDeadline(t)
}

// HasPiece checks if peer has a specific piece
func (p *Peer) HasPiece(index int) bool {
	if p.Bitfield == nil {
		return false
	}

	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(p.Bitfield) {
		return false
	}

	return p.Bitfield[byteIndex]&(1<<(7-bitIndex)) != 0
}

// SetPiece marks a piece as available in the bitfield
func (p *Peer) SetPiece(index int) {
	if p.Bitfield == nil {
		return
	}

	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(p.Bitfield) {
		return
	}

	p.Bitfield[byteIndex] |= 1 << (7 - bitIndex)
}
