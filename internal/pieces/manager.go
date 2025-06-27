// internal/piece/manager.go
package piece

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Manager manages all pieces for a torrent
type Manager struct {
	mu             sync.RWMutex
	pieces         []*Piece
	totalPieces    int
	pieceLength    int64
	totalLength    int64
	downloaded     int
	pendingPieces  map[int]*Piece      // Pieces currently being downloaded
	completePieces map[int]bool        // Completed pieces
	requests       map[string]*Request // Outstanding requests (key: "pieceIndex:begin")

	// Statistics
	downloadedBytes int64
	startTime       time.Time
}

func (m *Manager) GetTotalPieces() int {
	return m.totalPieces
}

func (m *Manager) GetDownloaded() int {
	return m.downloaded
}

func (m *Manager) GetPieces() []*Piece {
	return m.pieces
}

// NewManager creates a new piece manager
func NewManager(pieces [][20]byte, pieceLength int64, totalLength int64) *Manager {
	manager := &Manager{
		totalPieces:    len(pieces),
		pieceLength:    pieceLength,
		totalLength:    totalLength,
		pieces:         make([]*Piece, len(pieces)),
		pendingPieces:  make(map[int]*Piece),
		completePieces: make(map[int]bool),
		requests:       make(map[string]*Request),
		startTime:      time.Now(),
	}

	// Initialize pieces
	for i, hash := range pieces {
		length := pieceLength
		// Last piece might be smaller
		if i == len(pieces)-1 {
			lastPieceLength := totalLength % pieceLength
			if lastPieceLength != 0 {
				length = lastPieceLength
			}
		}

		manager.pieces[i] = NewPiece(i, hash, length)
	}

	return manager
}

// GetPieceToRequest returns the next piece that should be requested
func (m *Manager) GetPieceToRequest(peerBitfield []byte) *Piece {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If this is our first piece, pick randomly for faster start
	if m.downloaded == 0 {
		return m.getRandomAvailablePiece(peerBitfield)
	}

	// Use rarest first strategy
	return m.getRarestPiece(peerBitfield)
}

// getRandomAvailablePiece gets a random piece for first download
func (m *Manager) getRandomAvailablePiece(peerBitfield []byte) *Piece {
	var available []int

	for i := 0; i < m.totalPieces; i++ {
		if m.isPieceAvailable(i, peerBitfield) {
			available = append(available, i)
		}
	}

	if len(available) == 0 {
		return nil
	}

	index := available[rand.Intn(len(available))]
	piece := m.pieces[index]
	m.pendingPieces[index] = piece
	return piece
}

// getRarestPiece implements rarest first strategy (simplified)
func (m *Manager) getRarestPiece(peerBitfield []byte) *Piece {
	// For now, just return the first available piece
	// In a full implementation, you'd track piece rarity across all peers
	for i := 0; i < m.totalPieces; i++ {
		if m.isPieceAvailable(i, peerBitfield) {
			piece := m.pieces[i]
			m.pendingPieces[i] = piece
			return piece
		}
	}
	return nil
}

// isPieceAvailable checks if a piece can be requested
func (m *Manager) isPieceAvailable(index int, peerBitfield []byte) bool {
	// Check if we already have this piece
	if m.completePieces[index] {
		return false
	}

	// Check if piece is already being downloaded
	if _, exists := m.pendingPieces[index]; exists {
		return false
	}

	// Check if peer has this piece
	if !m.peerHasPiece(index, peerBitfield) {
		return false
	}

	return true
}

// peerHasPiece checks if peer has a specific piece
func (m *Manager) peerHasPiece(index int, bitfield []byte) bool {
	if bitfield == nil {
		return false
	}

	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(bitfield) {
		return false
	}

	return bitfield[byteIndex]&(1<<(7-bitIndex)) != 0
}

// AddRequest tracks a new request
func (m *Manager) AddRequest(pieceIndex, begin, length int, peerID [20]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%d:%d", pieceIndex, begin)
	m.requests[key] = &Request{
		PieceIndex: int64(pieceIndex),
		Begin:      int64(begin),
		Length:     int64(length),
		Requested:  time.Now(),
		PeerID:     peerID,
	}
}

// RemoveRequest removes a tracked request
func (m *Manager) RemoveRequest(pieceIndex, begin int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%d:%d", pieceIndex, begin)
	delete(m.requests, key)
}

// HandlePieceMessage processes incoming piece data
func (m *Manager) HandlePieceMessage(pieceIndex int, begin int64, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove the request
	key := fmt.Sprintf("%d:%d", pieceIndex, begin)
	delete(m.requests, key)

	// Get the piece
	if pieceIndex >= len(m.pieces) {
		return fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	piece := m.pieces[pieceIndex]

	// Set the block data
	err := piece.SetBlock(begin, data)
	if err != nil {
		return fmt.Errorf("failed to set block: %w", err)
	}

	// Check if piece is complete
	if piece.Complete {
		// Validate the piece
		if piece.Validate() {
			m.completePieces[pieceIndex] = true
			m.downloaded++
			m.downloadedBytes += int64(piece.Length)
			delete(m.pendingPieces, pieceIndex)

			fmt.Printf("Piece %d completed and validated! Progress: %d/%d (%.1f%%)\n",
				pieceIndex, m.downloaded, m.totalPieces, m.GetProgress())
		} else {
			// Hash validation failed, reset piece
			fmt.Printf("Piece %d failed validation, retrying...\n", pieceIndex)
			piece.Reset()
			delete(m.pendingPieces, pieceIndex)
		}
	}

	return nil
}

// GetTimeoutRequests returns requests that have timed out
func (m *Manager) GetTimeoutRequests() []*Request {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var timeouts []*Request
	now := time.Now()

	for _, req := range m.requests {
		if now.Sub(req.Requested) > RequestTimeout {
			timeouts = append(timeouts, req)
		}
	}

	return timeouts
}

// GetProgress returns download progress as percentage
func (m *Manager) GetProgress() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.totalPieces == 0 {
		return 0
	}
	return float64(m.downloaded) / float64(m.totalPieces) * 100
}

// IsComplete returns true if all pieces are downloaded
func (m *Manager) IsComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.downloaded == m.totalPieces
}

// GetDownloadSpeed returns current download speed in bytes/second
func (m *Manager) GetDownloadSpeed() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(m.downloadedBytes) / elapsed
}

// GetCompletedPieces returns a copy of completed pieces map
func (m *Manager) GetCompletedPieces() map[int]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	completed := make(map[int]bool)
	for k, v := range m.completePieces {
		completed[k] = v
	}
	return completed
}
