package torrent

import (
	"fmt"
	"sync"
	"time"

	"bittorrentclient/internal/file"
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

// NewDownloader creates a new downloader
func NewDownloader(t *Torrent, outputDir string) *Downloader {
	return &Downloader{
		torrent:      t,
		pieceManager: GetPieceManager(t, outputDir),
		requestMgr:   piece.NewRequestManager(piece.MaxRequestsPerPeer),
		selector:     piece.NewPieceSelector(),
		connections:  make(map[string]*peer.Connection),
		done:         make(chan struct{}),
		downloadDone: make(chan struct{}),
	}
}
func (d *Downloader) GetPieceMgr() *piece.Manager {
	return d.pieceManager
}
func GetPieceManager(t *Torrent, outputDir string) *piece.Manager {
	pieceHashes := make([][20]byte, len(t.Info.Pieces)/20)
	for i := 0; i < len(pieceHashes); i++ {
		pieceHashes[i] = t.Info.Pieces[i]
	}

	// Create file info from torrent
	fileInfos := createFileInfoFromTorrent(t)

	return piece.NewManager(pieceHashes, t.Info.PieceLength, t.Info.GetTotalLength(), fileInfos, outputDir)
}

// Start starts the download process
func (d *Downloader) Start() {
	// Initialize file system before starting download
	err := d.pieceManager.Initialize()
	if err != nil {
		fmt.Printf("Failed to initialize file system: %v\n", err)
		return
	}

	go d.downloadLoop()
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

// Stop stops the download process
func (d *Downloader) Stop() {
	close(d.done)

	// Stop all connections
	d.mu.Lock()
	for _, conn := range d.connections {
		conn.Stop()
	}
	d.mu.Unlock()

	// Close file writer
	if err := d.pieceManager.Close(); err != nil {
		fmt.Printf("Error closing file writer: %v\n", err)
	}
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

			// Print progress - Update this section
			fmt.Printf("Progress: %.1f%% - Speed: %.2f KB/s - Files: %s\n",
				d.pieceManager.GetProgress(),
				d.pieceManager.GetDownloadSpeed()/1024,
				d.getFileProgressSummary())
		}
	}
}

// handlePeer handles a single peer connection
// Replace this function in internal/torrent/download.go
func (d *Downloader) handlePeer(conn *peer.Connection) {
	defer d.RemovePeer(conn.ID)
	fmt.Printf("Handling peer %x\n", conn.ID[:8])

	if conn.IsUseful(d.pieceManager.GetCompletedPieces(), d.pieceManager.GetTotalPieces()) {
		fmt.Printf("Peer %x is useful, sending interested\n", conn.ID[:8])
		conn.SendInterested()
	}

	// Use a for...range loop over the piece data channel.
	// This loop will automatically terminate when conn.GetPieceData() is closed
	// by the connection's Stop() method, preventing a goroutine leak.
	for {
		select {
		case pieceData, ok := <-conn.GetPieceData():
			if !ok {
				// Channel has been closed, exit the goroutine.
				fmt.Printf("Peer %x disconnected. Exiting handler.\n", conn.ID[:8])
				return
			}

			d.requestMgr.RemoveRequest(conn.ID, pieceData.PieceIndex, pieceData.Begin)

			err := d.pieceManager.HandlePieceMessage(
				int(pieceData.PieceIndex),
				pieceData.Begin,
				pieceData.Data,
			)
			if err != nil {
				fmt.Printf("Error handling piece data from peer %x: %v\n", conn.ID[:8], err)
				// Optionally, you could disconnect from a peer that sends bad data.
				continue
			}

			// After handling a piece, try to request more blocks.
			d.requestMoreBlocks(conn, int(pieceData.PieceIndex))

		case <-d.done:
			// The entire downloader is shutting down.
			fmt.Printf("Downloader shutting down. Exiting handler for peer %x.\n", conn.ID[:8])
			return
		}
	}
}

// Replace the existing makeRequests function in internal/torrent/download.go

func (d *Downloader) makeRequests() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, conn := range d.connections {
		// A peer must be connected, not choking us, and have capacity for more requests.
		if !conn.IsConnected() || conn.Choked || !d.requestMgr.CanRequestFromPeer(conn.ID) {
			continue // Skip this peer if it's not ready
		}

		// Select a piece that the peer has, which we need, and is not already pending.
		piece := d.selector.SelectPiece(
			d.pieceManager,
			conn.Bitfield,
			d.pieceManager.GetDownloaded() == 0,
		)

		if piece != nil {
			// *** THIS IS THE CRITICAL FIX ***
			// Mark the piece as pending so we don't select it again for another peer.
			d.pieceManager.MarkPieceAsPending(piece)

			// This log is helpful to see which piece is being worked on
			fmt.Printf("INFO: Requesting piece %d from peer %x\n", piece.Index, conn.ID[:8])
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

// createFileInfoFromTorrent converts torrent info to file.FileInfo
func createFileInfoFromTorrent(t *Torrent) []file.FileInfo {
	if len(t.Info.Files) == 0 {
		// Single file torrent
		return []file.FileInfo{
			{
				Path:   t.Info.Name,
				Length: *t.Info.Length,
				Offset: 0,
			},
		}
	}

	// Multi-file torrent
	var files []file.FileInfo
	var offset int64

	for _, f := range t.Info.Files {
		path := t.Info.Name
		for _, p := range f.Path {
			path += "/" + p
		}

		files = append(files, file.FileInfo{
			Path:   path,
			Length: f.Length,
			Offset: offset,
		})

		offset += f.Length
	}

	return files
}

// getFileProgressSummary returns a summary of file progress
func (d *Downloader) getFileProgressSummary() string {
	progress := d.pieceManager.GetFileProgress()
	if progress == nil {
		return "N/A"
	}

	return progress.GetProgressSummary()
}
