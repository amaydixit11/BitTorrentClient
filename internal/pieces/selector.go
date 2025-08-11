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

// selectRarestFirst implements rarest first strategy
func (ps *PieceSelector) selectRarestFirst(manager *Manager, peerBitfield []byte) *Piece {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	// Track piece availability counts
	pieceAvailability := make(map[int]int)
	var availablePieces []int

	// Count availability across all peers
	for i := 0; i < manager.totalPieces; i++ {
		if manager.isPieceAvailable(i, peerBitfield) {
			availablePieces = append(availablePieces, i)
			// In a real implementation, you'd track this across all connected peers
			// For now, we'll simulate rarity by using piece index as a proxy
			pieceAvailability[i] = 1 + (i % 3) // Simulate varying availability
		}
	}

	if len(availablePieces) == 0 {
		return nil
	}

	// Find the rarest pieces (lowest availability count)
	minAvailability := int(^uint(0) >> 1) // Max int
	for _, pieceIndex := range availablePieces {
		if pieceAvailability[pieceIndex] < minAvailability {
			minAvailability = pieceAvailability[pieceIndex]
		}
	}

	// Collect all pieces with minimum availability
	var rarestPieces []int
	for _, pieceIndex := range availablePieces {
		if pieceAvailability[pieceIndex] == minAvailability {
			rarestPieces = append(rarestPieces, pieceIndex)
		}
	}

	// Randomly select from the rarest pieces to break ties
	selectedIndex := rarestPieces[ps.rng.Intn(len(rarestPieces))]
	return manager.pieces[selectedIndex]
}
