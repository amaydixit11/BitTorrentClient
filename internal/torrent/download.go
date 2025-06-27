package torrent

import (
	"fmt"
	"sync"
	"time"

	"bittorrentclient/internal/peer"
	piece "bittorrentclient/internal/pieces"
)

// Downloader manages the download process for a torrent
type Downloader struct {
	torrent      *Torrent
	pieceManager *piece.Manager
	requestMgr   *piece.RequestManager
	selector     *piece.PieceSelector
	connections  map[string]*peer.Connection
	mu           sync.RWMutex
	done         chan struct{}
	downloadDone chan struct{}
}

func GetPieceManager(t *Torrent) *piece.Manager {
	pieceHashes := make([][20]byte, len(t.Info.Pieces)/20)
	for i := 0; i < len(pieceHashes); i++ {
		pieceHashes[i] = t.Info.Pieces[i]
	}
	return piece.NewManager(pieceHashes, t.Info.PieceLength, t.Info.GetTotalLength())
}

// NewDownloader creates a new downloader
func NewDownloader(t *Torrent) *Downloader {
	pieceHashes := make([][20]byte, len(t.Info.Pieces)/20)
	for i := 0; i < len(pieceHashes); i++ {
		pieceHashes[i] = t.Info.Pieces[i]
	}

	return &Downloader{
		torrent:      t,
		pieceManager: piece.NewManager(pieceHashes, t.Info.PieceLength, t.Info.GetTotalLength()),
		requestMgr:   piece.NewRequestManager(piece.MaxRequestsPerPeer),
		selector:     piece.NewPieceSelector(),
		connections:  make(map[string]*peer.Connection),
		done:         make(chan struct{}),
		downloadDone: make(chan struct{}),
	}
}

// AddPeer adds a peer connection to the downloader
func (d *Downloader) AddPeer(conn *peer.Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()

	peerKey := fmt.Sprintf("%x", conn.ID[:8])
	d.connections[peerKey] = conn

	// Start handling this peer
	go d.handlePeer(conn)
}

// RemovePeer removes a peer connection
func (d *Downloader) RemovePeer(peerID [20]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	peerKey := fmt.Sprintf("%x", peerID[:8])
	if conn, exists := d.connections[peerKey]; exists {
		conn.Stop()
		delete(d.connections, peerKey)
		d.requestMgr.ClearPeerRequests(peerID)
	}
}

// Start starts the download process
func (d *Downloader) Start() {
	go d.downloadLoop()
}

// Stop stops the download process
func (d *Downloader) Stop() {
	close(d.done)

	// Stop all connections
	d.mu.Lock()
	for _, conn := range d.connections {
		conn.Stop()
	}
	d.mu.Unlock()
}

// IsComplete returns true if download is complete
func (d *Downloader) IsComplete() bool {
	return d.pieceManager.IsComplete()
}

// GetProgress returns download progress
func (d *Downloader) GetProgress() float64 {
	return d.pieceManager.GetProgress()
}

// WaitForCompletion waits until download is complete
func (d *Downloader) WaitForCompletion() {
	<-d.downloadDone
}

// downloadLoop main download coordination loop
func (d *Downloader) downloadLoop() {
	defer close(d.downloadDone)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.done:
			return

		case <-ticker.C:
			if d.pieceManager.IsComplete() {
				fmt.Printf("Download complete! 🎉\n")
				return
			}

			// Handle timeout requests
			d.handleTimeouts()

			// Try to make new requests
			d.makeRequests()

			// Print progress
			fmt.Printf("Progress: %.1f%% - Speed: %.2f KB/s\n",
				d.pieceManager.GetProgress(),
				d.pieceManager.GetDownloadSpeed()/1024)
		}
	}
}

// handlePeer handles a single peer connection
func (d *Downloader) handlePeer(conn *peer.Connection) {
	defer d.RemovePeer(conn.ID)

	// Send interested message if peer has pieces we need
	if conn.IsUseful(d.pieceManager.GetCompletedPieces(), d.pieceManager.GetTotalPieces()) {
		conn.SendInterested()
	}

	// Handle incoming piece data
	for {
		select {
		case <-d.done:
			return

		case pieceData := <-conn.GetPieceData():
			// Remove the request
			d.requestMgr.RemoveRequest(conn.ID, pieceData.PieceIndex, pieceData.Begin)

			// Handle the piece data
			err := d.pieceManager.HandlePieceMessage(
				int(pieceData.PieceIndex),
				pieceData.Begin,
				pieceData.Data,
			)
			if err != nil {
				fmt.Printf("Error handling piece data: %v\n", err)
				continue
			}

			// Check if we need to request more blocks from this piece
			d.requestMoreBlocks(conn, int(pieceData.PieceIndex))
		}
	}
}

// makeRequests tries to make new piece requests
func (d *Downloader) makeRequests() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, conn := range d.connections {
		if conn.Choked || !d.requestMgr.CanRequestFromPeer(conn.ID) {
			continue
		}

		// Find a piece to request
		piece := d.selector.SelectPiece(
			d.pieceManager,
			conn.Bitfield,
			d.pieceManager.GetDownloaded() == 0,
		)

		if piece != nil {
			d.requestBlocksFromPiece(conn, piece)
		}
	}
}

// requestBlocksFromPiece requests blocks from a specific piece
func (d *Downloader) requestBlocksFromPiece(conn *peer.Connection, piece *piece.Piece) {
	missingBlocks := piece.GetMissingBlocks()

	for _, block := range missingBlocks {
		if !d.requestMgr.CanRequestFromPeer(conn.ID) {
			break
		}

		// Add request to manager
		err := d.requestMgr.AddRequest(conn.ID, int64(piece.Index), block.Begin, block.Length)
		if err != nil {
			continue
		}

		// Send request to peer
		err = conn.RequestPiece(int64(piece.Index), block.Begin, block.Length)
		if err != nil {
			d.requestMgr.RemoveRequest(conn.ID, int64(piece.Index), block.Begin)
			fmt.Printf("Failed to request block: %v\n", err)
		}
	}
}

// requestMoreBlocks requests more blocks from a piece that's being downloaded
func (d *Downloader) requestMoreBlocks(conn *peer.Connection, pieceIndex int) {
	if pieceIndex >= len(d.pieceManager.GetPieces()) {
		return
	}

	piece := d.pieceManager.GetPieces()[pieceIndex]
	if piece.Complete {
		return
	}

	// Request more blocks if we have capacity
	missingBlocks := piece.GetMissingBlocks()
	for _, block := range missingBlocks {
		if !d.requestMgr.CanRequestFromPeer(conn.ID) {
			break
		}

		err := d.requestMgr.AddRequest(conn.ID, int64(piece.Index), block.Begin, block.Length)
		if err != nil {
			continue
		}

		err = conn.RequestPiece(int64(piece.Index), block.Begin, block.Length)
		if err != nil {
			d.requestMgr.RemoveRequest(conn.ID, int64(piece.Index), block.Begin)
		}
	}
}

// handleTimeouts handles request timeouts
func (d *Downloader) handleTimeouts() {
	timeouts := d.requestMgr.GetTimeoutRequests(piece.RequestTimeout)

	for _, req := range timeouts {
		fmt.Printf("Request timeout: piece %d, begin %d\n", req.PieceIndex, req.Begin)
		d.requestMgr.RemoveRequest(req.PeerID, req.PieceIndex, req.Begin)

		// TODO: Could re-request from different peer
	}
}
