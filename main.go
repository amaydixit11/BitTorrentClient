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

	torrentFile := "debian.torrent"
	outputDir := "./downloads/debian_1"
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
	fmt.Printf("   🧩 Pieces: %d\n", len(t.Info.Pieces))
	fmt.Printf("    Announce URL: %s\n", t.Announce)

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

	fmt.Println("\n🔍 STEP 6: Connecting to peers (PARALLEL)...")

	// Connection result channel
	type connResult struct {
		conn *peer.Connection
		addr string
		err  error
	}

	resultChan := make(chan connResult, len(resp.Peers))
	maxPeers := 5   // Max peers we want to connect to
	batchSize := 15 // Try 15 peers at once
	timeout := 10 * time.Second

	// Try peers in batches
	peersToTry := resp.Peers
	if len(peersToTry) > 50 {
		peersToTry = peersToTry[:50]
	}

	fmt.Printf("   🚀 Attempting %d peers in parallel (timeout: %v)...\n", min(batchSize, len(peersToTry)), timeout)

	// Launch parallel connection attempts
	for i := 0; i < len(peersToTry) && i < batchSize; i++ {
		p := peersToTry[i]
		peerAddr := fmt.Sprintf("%s:%d", p.IP, p.Port)

		go func(addr string) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			conn, err := peer.ConnectToPeer(ctx, addr, t.InfoHash, peerID)
			if err != nil {
				resultChan <- connResult{nil, addr, err}
				return
			}

			peerConn := peer.NewConnection(conn.Conn, t.InfoHash)
			peerConn.ID = conn.ID
			peerConn.Start()
			resultChan <- connResult{peerConn, addr, nil}
		}(peerAddr)
	}

	// Collect results with overall timeout
	connectedPeers := 0
	overallTimeout := time.After(timeout + 5*time.Second)
	attemptsReceived := 0
	totalAttempts := min(batchSize, len(peersToTry))

	for connectedPeers < maxPeers && attemptsReceived < totalAttempts {
		select {
		case result := <-resultChan:
			attemptsReceived++
			if result.err != nil {
				fmt.Printf("   ❌ %s: %v\n", result.addr, result.err)
			} else {
				fmt.Printf("   ✅ Connected to %s\n", result.addr)
				downloader.AddPeer(result.conn)
				connectedPeers++
			}
		case <-overallTimeout:
			fmt.Println("   ⏰ Connection timeout reached")
			goto doneConnecting
		}
	}

doneConnecting:
	if connectedPeers == 0 {
		log.Fatalf("❌ Could not connect to any peers. Try a different network or VPN.")
	}

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
