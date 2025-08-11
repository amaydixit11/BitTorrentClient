package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bittorrentclient/internal/peer"
	"bittorrentclient/internal/torrent"
	"bittorrentclient/internal/tracker"
)

func generatePeerID() [20]byte {
	var peerID [20]byte
	copy(peerID[:8], []byte("-BC0100-"))
	rand.Read(peerID[8:])
	return peerID
}

func main() {
	//if len(os.Args) < 2 {
	//	fmt.Println("Usage: go run main.go <torrent-file> [output-directory]")
	//	os.Exit(1)
	//}

	torrentFile := "small.torrent"
	outputDir := "./downloads"
	if len(os.Args) >= 3 {
		outputDir = os.Args[2]
	}

	fmt.Println("🔍 STEP 1: Parsing torrent file...")
	t, err := torrent.Open(torrentFile)
	if err != nil {
		log.Fatalf("❌ Failed to parse torrent: %v", err)
	}
	fmt.Printf("✅ Torrent parsed successfully\n")
	fmt.Printf("   📁 Name: %s\n", t.Info.Name)
	fmt.Printf("   💾 Size: %s\n", formatBytes(t.Info.GetTotalLength()))
	fmt.Printf("   🧩 Pieces: %d\n", len(t.Info.Pieces)/20)
	fmt.Printf("   🔗 Announce URL: %s\n", t.Announce)

	fmt.Println("\n🔍 STEP 2: Creating output directory...")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("❌ Failed to create output directory: %v", err)
	}
	fmt.Printf("✅ Output directory ready: %s\n", outputDir)

	fmt.Println("\n🔍 STEP 3: Generating peer ID...")
	peerID := generatePeerID()
	fmt.Printf("✅ Peer ID generated: %x\n", peerID[:8])

	fmt.Println("\n🔍 STEP 4: Contacting tracker...")
	client := tracker.NewTrackerClient(6881)

	req := &tracker.TrackerRequest{
		InfoHash:   t.InfoHash[:],
		PeerID:     peerID[:],
		Port:       6881,
		Uploaded:   0,
		Downloaded: 0,
		Left:       t.Info.GetTotalLength(),
		Compact:    true,
		Event:      "started",
		NumWant:    10, // Reduced for debugging
	}

	resp, err := client.Announce(t.Announce, req)
	if err != nil {
		log.Fatalf("❌ Failed to get peers from tracker: %v", err)
	}

	if len(resp.Peers) == 0 {
		log.Fatalf("❌ No peers available from tracker")
	}

	fmt.Printf("✅ Got %d peers from tracker\n", len(resp.Peers))
	for i, p := range resp.Peers {
		fmt.Printf("   Peer %d: %s:%d\n", i+1, p.IP, p.Port)
	}

	fmt.Println("\n🔍 STEP 5: Creating downloader...")
	downloader := torrent.NewDownloader(t, outputDir)
	downloader.Start()
	fmt.Printf("✅ Downloader created and started\n")

	fmt.Println("\n🔍 STEP 6: Connecting to peers (ONE AT A TIME)...")
	connectedPeers := 0
	maxPeers := 10 // Increased from 3

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Increased from 10s
	defer cancel()

	for i, p := range resp.Peers {
		if connectedPeers >= maxPeers {
			break
		}

		peerAddr := fmt.Sprintf("%s:%d", p.IP, p.Port)
		fmt.Printf("   Attempting to connect to peer %d: %s\n", i+1, peerAddr)

		conn, err := peer.ConnectToPeer(ctx, peerAddr, t.InfoHash, peerID)
		if err != nil {
			fmt.Printf("   ❌ Failed to connect to %s: %v\n", peerAddr, err)
			continue
		}

		fmt.Printf("   ✅ Connected to %s\n", peerAddr)

		peerConn := peer.NewConnection(conn.Conn, t.InfoHash)
		peerConn.ID = conn.ID
		peerConn.Start()

		downloader.AddPeer(peerConn)
		connectedPeers++

		fmt.Printf("   📊 Added peer to downloader (total: %d)\n", connectedPeers)

		time.Sleep(1 * time.Second)
	}

	if connectedPeers == 0 {
		log.Fatalf("❌ Could not connect to any peers")
	}

	fmt.Printf("✅ Connected to %d peers successfully\n", connectedPeers)

	fmt.Println("\n🔍 STEP 7: Starting download monitoring...")
	fmt.Println("   📊 Progress will be shown every 5 seconds")
	fmt.Println("   🛑 Press Ctrl+C to stop\n")
	// Replace the section from "STEP 7" to the end of the main function.
	fmt.Printf("✅ Connected to %d peers successfully\n", connectedPeers)

	fmt.Println("\n🔍 STEP 7: Starting download monitoring...")
	fmt.Println("   📊 Progress will be shown every 5 seconds")
	fmt.Println("   🛑 Press Ctrl+C to stop\n")

	// Create a channel to listen for OS signals (like Ctrl+C)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	// Main monitoring loop
	for {
		select {
		case <-progressTicker.C:
			progress := downloader.GetProgress()
			isComplete := downloader.IsComplete()

			// Get some diagnostic info from the piece manager
			var speed float64
			var downloadedPieces, totalPieces int
			if downloader.GetPieceMgr() != nil {
				speed = downloader.GetPieceMgr().GetDownloadSpeed()
				downloadedPieces = downloader.GetPieceMgr().GetDownloaded()
				totalPieces = downloader.GetPieceMgr().GetTotalPieces()
			}

			fmt.Printf("📊 Progress: %.2f%% (%d/%d pieces) | Speed: %.2f KB/s\n",
				progress, downloadedPieces, totalPieces, speed/1024)

			if isComplete {
				fmt.Printf("\n🎉 Download completed! Files saved to: %s\n", outputDir)
				downloader.Stop()
				return // Exit main
			}

		case <-signals:
			// Signal received, start graceful shutdown.
			fmt.Println("\n🛑 Shutdown signal received. Stopping downloader...")
			downloader.Stop()
			// You might want to wait for the downloader to finish stopping here.
			// For now, we'll just exit.
			fmt.Println("Downloader stopped. Exiting.")
			return // Exit main
		}
	}
}

// You will also need to add these imports to main.go:
// import "os/signal"
// import "syscall"
// // Give peers a moment to exchange initial messages
// time.Sleep(3 * time.Second)

// // Simple progress monitoring loop with crash protection
// iteration := 0
// for {
// 	func() {
// 		defer func() {
// 			if r := recover(); r != nil {
// 				fmt.Printf("❌ Panic in monitoring loop: %v\n", r)
// 			}
// 		}()

// 		time.Sleep(5 * time.Second)
// 		iteration++

// 		// Get both piece and file progress
// 		var pieceProgress float64
// 		var fileProgress *file.Progress
// 		func() {
// 			defer func() {
// 				if r := recover(); r != nil {
// 					fmt.Printf("❌ Panic getting progress: %v\n", r)
// 					pieceProgress = -1
// 				}
// 			}()
// 			pieceProgress = downloader.GetProgress()
// 			fileProgress = downloader.GetPieceMgr().GetFileProgress()
// 		}()

// 		// Calculate file-based progress
// 		var fileProgressPercent float64
// 		var fileSummary string
// 		if fileProgress != nil {
// 			fileProgressPercent = fileProgress.GetOverallProgressPercent()
// 			fileSummary = fileProgress.GetProgressSummary()
// 		} else {
// 			fileProgressPercent = 0.0
// 			fileSummary = "N/A"
// 		}

// 		if pieceProgress >= 0 {
// 			fmt.Printf("📊 Piece Progress: %.1f%% | File Progress: %.1f%% | Files: %s (iteration %d)\n",
// 				pieceProgress, fileProgressPercent, fileSummary, iteration)
// 		} else {
// 			fmt.Printf("📊 Progress: ERROR | Files: %s (iteration %d)\n", fileSummary, iteration)
// 		}

// 		if iteration%3 == 0 {
// 			fmt.Println("🔍 DIAGNOSTIC INFO:")
// 			fmt.Printf("   - Total pieces needed: %d\n", len(t.Info.Pieces)/20)
// 			fmt.Printf("   - Completed pieces: %d\n", downloader.GetPieceMgr().GetDownloaded())
// 			fmt.Printf("   - Connected peers: %d\n", connectedPeers)

// 			var isComplete bool
// 			func() {
// 				defer func() {
// 					if r := recover(); r != nil {
// 						fmt.Printf("❌ Panic checking completion: %v\n", r)
// 						isComplete = false
// 					}
// 				}()
// 				isComplete = downloader.IsComplete()
// 			}()

// 			fmt.Printf("   - Downloader complete: %v\n", isComplete)

// 			if isComplete {
// 				fmt.Printf("🎉 Download completed! Files saved to: %s\n", outputDir)
// 				downloader.Stop()
// 				return
// 			}
// 		}

// 		if iteration > 100 {
// 			fmt.Println("⏰ Stopping after 20 iterations for debugging")
// 			downloader.Stop()
// 			return
// 		}
// 	}()
// }
// }

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
