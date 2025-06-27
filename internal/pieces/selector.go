// internal/piece/selector.go
package piece

import (
	"math/rand"
	"time"
)

// PieceSelector handles piece selection strategies
type PieceSelector struct {
	rng *rand.Rand
}

// NewPieceSelector creates a new piece selector
func NewPieceSelector() *PieceSelector {
	return &PieceSelector{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectPiece selects the next piece to download based on strategy
func (ps *PieceSelector) SelectPiece(manager *Manager, peerBitfield []byte, isFirstPiece bool) *Piece {
	if isFirstPiece {
		return ps.selectRandomPiece(manager, peerBitfield)
	}
	return ps.selectRarestFirst(manager, peerBitfield)
}

// selectRandomPiece selects a random available piece
func (ps *PieceSelector) selectRandomPiece(manager *Manager, peerBitfield []byte) *Piece {
	var available []*Piece

	for i, piece := range manager.pieces {
		if manager.isPieceAvailable(i, peerBitfield) {
			available = append(available, piece)
		}
	}

	if len(available) == 0 {
		return nil
	}

	return available[ps.rng.Intn(len(available))]
}

// selectRarestFirst implements rarest first strategy (simplified version)
func (ps *PieceSelector) selectRarestFirst(manager *Manager, peerBitfield []byte) *Piece {
	// In a full implementation, this would track piece frequency across all peers
	// For now, we'll just select the first available piece
	for i, piece := range manager.pieces {
		if manager.isPieceAvailable(i, peerBitfield) {
			return piece
		}
	}
	return nil
}
