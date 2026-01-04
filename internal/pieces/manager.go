package piece

import (
	"bittorrentclient/internal/file"
	"fmt"
	"math/rand"
	"strings"
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

	// File system integration - Add these fields
	fileWriter *file.Writer
	fileMapper *file.Mapper
	resumeData map[int]bool // For resume capability

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
func NewManager(pieces [][20]byte, pieceLength int64, totalLength int64, fileInfos []file.FileInfo, outputDir string) *Manager {
	// Create file mapper
	mapper := file.NewMapper(fileInfos, pieceLength, totalLength)

	// Create file writer
	writer := file.NewWriter(mapper, outputDir)

	manager := &Manager{
		totalPieces:    len(pieces),
		pieceLength:    pieceLength,
		totalLength:    totalLength,
		pieces:         make([]*Piece, len(pieces)),
		pendingPieces:  make(map[int]*Piece),
		completePieces: make(map[int]bool),
		requests:       make(map[string]*Request),
		fileWriter:     writer,
		fileMapper:     mapper,
		resumeData:     make(map[int]bool),
		startTime:      time.Now(),
	}

	// Initialize pieces
	for i, hash := range pieces {
		length := pieceLength
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

// Initialize sets up the file system
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize file writer
	err := m.fileWriter.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize file writer: %w", err)
	}

	// Check for existing files and resume data
	err = m.loadResumeData()
	if err != nil {
		fmt.Printf("No resume data found, starting fresh download\n")
	}

	return nil
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

	key := fmt.Sprintf("%d:%d", pieceIndex, begin)
	delete(m.requests, key)

	if pieceIndex >= len(m.pieces) {
		return fmt.Errorf("invalid piece index: %d", pieceIndex)
	}
	piece := m.pieces[pieceIndex]

	// Don't process blocks for already completed pieces
	if piece.IsComplete() {
		return nil
	}

	err := piece.SetBlock(begin, data)
	if err != nil {
		return fmt.Errorf("failed to set block: %w", err)
	}

	// Check if the piece is now fully downloaded (all blocks received)
	if piece.IsComplete() {
		// Validate the piece hash
		if piece.Validate() {
			fmt.Printf("✅ Piece %d validated successfully!\n", pieceIndex)
			err := m.fileWriter.WritePiece(pieceIndex, piece.Data)
			if err != nil {
				fmt.Printf("❌ Failed to write piece %d to file: %v\n", pieceIndex, err)
				piece.Reset() // Reset piece to re-download
				delete(m.pendingPieces, pieceIndex)
				return fmt.Errorf("failed to write piece to file: %w", err)
			}

			// Mark as complete and update stats
			m.completePieces[pieceIndex] = true
			m.downloaded++
			m.downloadedBytes += piece.Length
			delete(m.pendingPieces, pieceIndex)

			fmt.Printf("Piece %d completed! Progress: %d/%d (%.1f%%)\n",
				pieceIndex, m.downloaded, m.totalPieces, m.GetProgress())

			if m.downloaded%10 == 0 {
				m.saveResumeData()
			}
		} else {
			// If validation fails, reset the piece so it can be downloaded again.
			fmt.Printf("Piece %d failed validation, retrying...\n", pieceIndex)
			piece.Reset()
			m.cleanupPieceRequests(pieceIndex)
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

// saveResumeData saves current progress for resume capability
func (m *Manager) saveResumeData() {
	// Simple implementation - in production you'd save to a file
	m.resumeData = make(map[int]bool)
	for k, v := range m.completePieces {
		m.resumeData[k] = v
	}
}

// loadResumeData loads previous progress
func (m *Manager) loadResumeData() error {
	// Simply return an error to indicate no resume data
	// This will force a fresh download
	return fmt.Errorf("no resume data available")
}

// Close closes the file writer
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fileWriter != nil {
		return m.fileWriter.Close()
	}
	return nil
}

// GetFileProgress returns file writing progress
func (m *Manager) GetFileProgress() *file.Progress {
	if m.fileWriter != nil {
		return m.fileWriter.GetProgress()
	}
	return nil
}
func (m *Manager) cleanupPieceRequests(pieceIndex int) {
	for key := range m.requests {
		if strings.HasPrefix(key, fmt.Sprintf("%d:", pieceIndex)) {
			delete(m.requests, key)
		}
	}
}
func (p *Piece) IsComplete() bool {
	for _, r := range p.Downloaded {
		if !r {
			return false
		}
	}
	return true
}

// Add this new function to internal/pieces/manager.go

// MarkPieceAsPending adds a piece to the pending map in a thread-safe way.
func (m *Manager) MarkPieceAsPending(piece *Piece) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Only mark as pending if it's not already complete
	if _, exists := m.completePieces[piece.Index]; !exists {
		m.pendingPieces[piece.Index] = piece
	}
}
