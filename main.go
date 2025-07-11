package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
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
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <torrent-file> [output-directory]")
		os.Exit(1)
	}

	torrentFile := os.Args[1]
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
	maxPeers := 3 // Very limited for debugging

	// Create a context with timeout for peer connections
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

		// Small delay between connections for debugging
		time.Sleep(1 * time.Second)
	}

	if connectedPeers == 0 {
		log.Fatalf("❌ Could not connect to any peers")
	}

	fmt.Printf("✅ Connected to %d peers successfully\n", connectedPeers)

	fmt.Println("\n🔍 STEP 7: Starting download monitoring...")
	fmt.Println("   📊 Progress will be shown every 5 seconds")
	fmt.Println("   🛑 Press Ctrl+C to stop\n")

	// Give peers a moment to exchange initial messages
	time.Sleep(3 * time.Second)

	// Simple progress monitoring loop with crash protection
	iteration := 0
	for {
		// Use defer to catch any panics in the monitoring loop
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("❌ Panic in monitoring loop: %v\n", r)
				}
			}()

			time.Sleep(5 * time.Second)
			iteration++

			// Safely get progress
			var progress float64
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("❌ Panic getting progress: %v\n", r)
						progress = -1
					}
				}()
				progress = downloader.GetProgress()
			}()

			if progress >= 0 {
				fmt.Printf("📊 Progress: %.1f%% (iteration %d)\n", progress, iteration)
			} else {
				fmt.Printf("📊 Progress: ERROR (iteration %d)\n", iteration)
			}

			// Add some diagnostics every few iterations
			if iteration%3 == 0 {
				fmt.Println("🔍 DIAGNOSTIC INFO:")
				fmt.Printf("   - Total pieces needed: %d\n", len(t.Info.Pieces)/20)
				fmt.Printf("   - Connected peers: %d\n", connectedPeers)

				// Safely check if complete
				var isComplete bool
				func() {
					defer func() {
						if r := recover(); r != nil {
							fmt.Printf("❌ Panic checking completion: %v\n", r)
							isComplete = false
						}
					}()
					isComplete = downloader.IsComplete()
				}()

				fmt.Printf("   - Downloader complete: %v\n", isComplete)

				if isComplete {
					fmt.Printf("🎉 Download completed! Files saved to: %s\n", outputDir)
					downloader.Stop()
					return
				}
			}

			// Safety exit after reasonable time for debugging
			if iteration > 20 {
				fmt.Println("⏰ Stopping after 20 iterations for debugging")
				downloader.Stop()
				return
			}
		}()
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
