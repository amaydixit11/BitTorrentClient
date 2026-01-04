# BitTorrent Client

A fully functional BitTorrent client written from scratch in Go.

## Features

- **Torrent Parsing** - Parses `.torrent` files with bencode decoder
- **Tracker Communication** - HTTP tracker announce requests
- **Peer Protocol** - Complete BitTorrent wire protocol implementation
- **Piece Management** - SHA1 hash verification for data integrity
- **Multi-file Support** - Handles both single-file and multi-file torrents
- **Parallel Connections** - Connects to multiple peers simultaneously
- **Rarest First** - Intelligent piece selection strategy

## Quick Start

```bash
# Build
go build .

# Run with a torrent file
go run main.go <torrent-file> [output-directory]

# Examples
go run main.go debian.torrent ./downloads
go run main.go ubuntu.torrent ./my-downloads
```

## Project Structure

```
├── main.go                    # Entry point
├── internal/
│   ├── bencode/              # Bencode encoder/decoder
│   │   ├── bencode_decode.go
│   │   └── bencode_encode.go
│   ├── torrent/              # Torrent file handling
│   │   ├── torrent.go        # Main struct
│   │   ├── parser.go         # .torrent file parser
│   │   ├── info.go           # Info dictionary
│   │   ├── info_hash.go      # SHA1 hash generation
│   │   ├── file.go           # File struct
│   │   └── download.go       # Download orchestration
│   ├── tracker/              # Tracker communication
│   │   ├── types.go          # Request/Response types
│   │   ├── client.go         # HTTP client
│   │   ├── announce.go       # Announce requests
│   │   └── peers.go          # Peer parsing
│   ├── peer/                 # Peer-to-peer protocol
│   │   ├── peer.go           # Peer struct & bitfield
│   │   ├── connection.go     # Connection management
│   │   ├── handshake.go      # BitTorrent handshake
│   │   └── message.go        # Wire protocol messages
│   ├── pieces/               # Piece management
│   │   ├── piece.go          # Piece & Block structs
│   │   ├── manager.go        # Piece state tracking
│   │   ├── request.go        # Request management
│   │   └── selector.go       # Piece selection
│   └── file/                 # File I/O
│       ├── allocator.go      # File preallocation
│       ├── mapper.go         # Piece-to-file mapping
│       ├── progress.go       # Progress tracking
│       └── writer.go         # Disk writes
```

## How It Works

1. **Parse** - Reads and decodes the `.torrent` file
2. **Connect to Tracker** - Gets list of peers from the tracker
3. **Handshake** - Establishes connections with peers using BitTorrent protocol
4. **Download** - Requests pieces, verifies SHA1 hashes, writes to disk
5. **Complete** - File assembled from verified pieces

## Requirements

- Go 1.21+
- Network access to trackers and peers

## Test Torrents

Popular test torrents with active seeders:

| Torrent | Size | Description |
|---------|------|-------------|
| [Big Buck Bunny](https://webtorrent.io/torrents/big-buck-bunny.torrent) | 140 MB | Animated short film |
| [Sintel](https://webtorrent.io/torrents/sintel.torrent) | 130 MB | Animated short film |
| [Debian ISO](https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/) | 700+ MB | Linux distribution |

## Not Yet Supported

The following features are not implemented:

- **Seeding/Uploading** - Download only, no upload to other peers
- **DHT (Distributed Hash Table)** - Requires tracker; no trackerless mode
- **Magnet Links** - Only `.torrent` files are supported
- **UDP Trackers** - HTTP trackers only
- **Peer Exchange (PEX)** - No peer sharing between connections
- **Encryption (MSE/PE)** - Unencrypted connections only
- **Resume Downloads** - Fresh start on each run (resume logic exists but not wired up)
- **Rate Limiting** - No bandwidth throttling
- **IPv6** - IPv4 only
- **Web Seeds** - No HTTP/FTP fallback sources