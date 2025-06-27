// // internal/download/manager.go
package download

// import (
// 	"fmt"
// 	"os"
// 	"sync"
// 	"time"

// 	"bittorrentclient/internal/peer"
// 	piece "bittorrentclient/internal/pieces"
// 	"bittorrentclient/internal/torrent"
// 	"bittorrentclient/internal/tracker"
// )

// // DownloadManager manages the download process
// type DownloadManager struct {
// 	torrent      *torrent.Torrent
// 	pieceManager *piece.PieceManager
// 	peers        map[string]*peer.Peer
// 	peersMutex   sync.RWMutex
// 	done         chan bool
// 	cancel       chan bool
// }

// // NewDownloadManager creates a new download manager
// func NewDownloadManager(t *torrent.Torrent) *DownloadManager {
// 	// Extract piece hashes from torrent
// 	hashes := t.Info.Pieces

// 	dm := &DownloadManager{
// 		torrent: t,
// 		pieceManager: piece.NewPieceManager(
// 			len(hashes)/20,
// 			uint32(t.Info.PieceLength),
// 			uint64(*t.Info.Length),
// 			hashes,
// 		),
// 		peers:  make(map[string]*peer.Peer),
// 		done:   make(chan bool),
// 		cancel: make(chan bool),
// 	}

// 	return dm
// }

// // Start starts the download process
// func (dm *DownloadManager) Start(peerAddresses []string) error {
// 	fmt.Printf("Starting download for torrent: %s\n", dm.torrent.Info.Name)

// 	// Connect to peers
// 	for _, addr := range peerAddresses {
// 		go dm.connectToPeer(addr)
// 	}

// 	// Start main download loop
// 	go dm.downloadLoop()

// 	// Start timeout handler
// 	go dm.timeoutHandler()

// 	return nil
// }

// // Stop stops the download process
// func (dm *DownloadManager) Stop() {
// 	close(dm.cancel)

// 	// Close all peer connections
// 	dm.peersMutex.Lock()
// 	for _, p := range dm.peers {
// 		p.Close()
// 	}
// 	dm.peersMutex.Unlock()
// }

// // connectToPeer connects to a single peer
// func (dm *DownloadManager) connectToPeer(address string) {
// 	// Generate peer ID (you should have this from your tracker client)
// 	peerID := [20]byte{} // Use your actual peer ID

// 	p, err := peer.ConnectToPeer(address, dm.torrent.InfoHash, peerID)
// 	if err != nil {
// 		fmt.Printf("Failed to connect to peer %s: %v\n", address, err)
// 		return
// 	}

// 	fmt.Printf("Connected to peer: %s\n", address)

// 	dm.peersMutex.Lock()
// 	dm.peers[address] = p
// 	dm.peersMutex.Unlock()

// 	// Handle this peer
// 	go dm.handlePeer(p, address)
// }

// // handlePeer handles communication with a single peer
// func (dm *DownloadManager) handlePeer(p *peer.Peer, address string) {
// 	defer func() {
// 		dm.peersMutex.Lock()
// 		delete(dm.peers, address)
// 		dm.peersMutex.Unlock()
// 		p.Close()
// 	}()

// 	// Send interested message
// 	err := p.SendInterested()
// 	if err != nil {
// 		fmt.Printf("Failed to send interested to %s: %v\n", address, err)
// 		return
// 	}

// 	// Message handling loop
// 	for {
// 		select {
// 		case <-dm.cancel:
// 			return
// 		default:
// 			// Set read timeout
// 			p.SetDeadline(time.Now().Add(30 * time.Second))

// 			msg, err := p.ReadMessage()
// 			if err != nil {
// 				fmt.Printf("Error reading message from %s: %v\n", address, err)
// 				return
// 			}

// 			err = dm.handleMessage(p, msg, address)
// 			if err != nil {
// 				fmt.Printf("Error handling message from %s: %v\n", address, err)
// 				return
// 			}
// 		}
// 	}
// }

// // handleMessage handles a message from a peer
// func (dm *DownloadManager) handleMessage(p *peer.Peer, msg *peer.Message, address string) error {
// 	// Update peer state
// 	err := p.HandleMessage(msg)
// 	if err != nil {
// 		return err
// 	}

// 	if msg == nil {
// 		// Keep-alive message
// 		return nil
// 	}

// 	switch msg.ID {
// 	case peer.MsgUnchoke:
// 		fmt.Printf("Peer %s unchoked us\n", address)
// 		// Start requesting pieces
// 		go dm.requestPieces(p)

// 	case peer.MsgBitfield:
// 		fmt.Printf("Received bitfield from %s\n", address)
// 		// Peer's bitfield is already stored in p.Bitfield by HandleMessage

// 	case peer.MsgHave:
// 		// Peer announced they have a new piece
// 		// The piece is already marked in p.Bitfield by HandleMessage

// 	case peer.MsgPiece:
// 		// Handle received piece data
// 		index, offset, data, err := peer.ParsePieceMessage(msg.Payload)
// 		if err != nil {
// 			return fmt.Errorf("failed to parse piece message: %w", err)
// 		}

// 		err = dm.pieceManager.ReceiveBlock(index, offset, data)
// 		if err != nil {
// 			return fmt.Errorf("failed to receive block: %w", err)
// 		}

// 		fmt.Printf("Received block: piece %d, offset %d, length %d\n",
// 			index, offset, len(data))

// 		// Request more pieces from this peer
// 		go dm.requestPieces(p)

// 	case peer.MsgChoke:
// 		fmt.Printf("Peer %s choked us\n", address)
// 	}

// 	return nil
// }

// // requestPieces requests pieces from a peer
// func (dm *DownloadManager) requestPieces(p *peer.Peer) {
// 	if p.Choked {
// 		return
// 	}

// 	// Request up to 5 blocks at a time to keep the pipeline full
// 	for i := 0; i < 5; i++ {
// 		block, err := dm.pieceManager.GetNextBlockRequest(p.Bitfield, p.ID)
// 		if err != nil {
// 			// No more blocks to request
// 			break
// 		}

// 		err = p.SendRequest(block.PieceIndex, block.Offset, block.Length)
// 		if err != nil {
// 			fmt.Printf("Failed to send request: %v\n", err)
// 			dm.pieceManager.ResetTimedOutBlock(block)
// 			break
// 		}

// 		fmt.Printf("Requested block: piece %d, offset %d, length %d\n",
// 			block.PieceIndex, block.Offset, block.Length)
// 	}
// }

// // downloadLoop main download coordination loop
// func (dm *DownloadManager) downloadLoop() {
// 	ticker := time.NewTicker(1 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-dm.cancel:
// 			return
// 		case <-ticker.C:
// 			// Check progress
// 			completed, total, percentage := dm.pieceManager.GetProgress()
// 			fmt.Printf("Progress: %d/%d pieces (%.2f%%)\n",
// 				completed, total, percentage)

// 			if dm.pieceManager.IsComplete() {
// 				fmt.Println("Download completed!")
// 				dm.done <- true
// 				return
// 			}
// 		}
// 	}
// }

// // timeoutHandler handles timed out requests
// func (dm *DownloadManager) timeoutHandler() {
// 	ticker := time.NewTicker(10 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-dm.cancel:
// 			return
// 		case <-ticker.C:
// 			// Check for timed out requests
// 			timedOut := dm.pieceManager.GetTimedOutRequests()
// 			for _, block := range timedOut {
// 				fmt.Printf("Block timed out: piece %d, offset %d\n",
// 					block.PieceIndex, block.Offset)
// 				dm.pieceManager.ResetTimedOutBlock(block)
// 			}
// 		}
// 	}
// }

// // WaitForCompletion waits for download to complete
// func (dm *DownloadManager) WaitForCompletion() {
// 	<-dm.done
// }

// // GetProgress returns current download progress
// func (dm *DownloadManager) GetProgress() (completed, total int, percentage float64) {
// 	return dm.pieceManager.GetProgress()
// }

// // Extensions to internal/peer/message.go (add these functions)

// // ParsePieceMessage parses a piece message payload
// func ParsePieceMessage(payload []byte) (index, offset uint32, data []byte, err error) {
// 	if len(payload) < 8 {
// 		return 0, 0, nil, fmt.Errorf("piece message too short")
// 	}

// 	index = uint32(payload[0])<<24 | uint32(payload[1])<<16 |
// 		uint32(payload[2])<<8 | uint32(payload[3])
// 	offset = uint32(payload[4])<<24 | uint32(payload[5])<<16 |
// 		uint32(payload[6])<<8 | uint32(payload[7])

// 	data = payload[8:]
// 	return index, offset, data, nil
// }

// // Extensions to main.go (add download command)

// func main() {
// 	if len(os.Args) < 3 {
// 		fmt.Println("Usage: go run main.go <command> <torrent-file>")
// 		fmt.Println("Commands: parse, announce, download")
// 		return
// 	}

// 	command := os.Args[1]
// 	torrentFile := os.Args[2]

// 	switch command {
// 	case "download":
// 		downloadTorrent(torrentFile)
// 		// ... other existing commands
// 	}
// }

// func downloadTorrent(filename string) {
// 	// Parse torrent file
// 	t, err := torrent.Open(filename)
// 	if err != nil {
// 		fmt.Printf("Error parsing torrent: %v\n", err)
// 		return
// 	}

// 	// Get peers from tracker
// 	tc := tracker.NewTrackerClient(6881)
// 	defer tc.Close()

// 	peers, err := tc.GetPeers(t.Announce, t.InfoHash, 0)
// 	if err != nil {
// 		fmt.Printf("Error getting peers: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("Found %d peers\n", len(peers))

// 	// Start download
// 	dm := download.NewDownloadManager(t)

// 	var peerAddresses []string
// 	for _, peer := range peers {
// 		peerAddresses = append(peerAddresses, fmt.Sprintf("%s:%d", peer.IP, peer.Port))
// 	}

// 	err = dm.Start(peerAddresses[:5]) // Connect to first 5 peers
// 	if err != nil {
// 		fmt.Printf("Error starting download: %v\n", err)
// 		return
// 	}

// 	// Wait for completion
// 	dm.WaitForCompletion()
// 	fmt.Println("Download finished!")
// }
