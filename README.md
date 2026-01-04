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

# BitTorrent Client Architecture

Comprehensive documentation of the codebase architecture and implementation details.

## Table of Contents

1. [System Overview](#system-overview)
2. [Data Flow](#data-flow)
3. [Module Architecture](#module-architecture)
4. [File Reference](#file-reference)

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         main.go                                  │
│                    (Entry Point & Orchestration)                 │
└─────────────────────────────────────────────────────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   torrent/    │    │    tracker/     │    │     peer/       │
│ Parse .torrent│    │ Get peer list   │    │ Wire protocol   │
└───────────────┘    └─────────────────┘    └─────────────────┘
        │                      │                      │
        └──────────────────────┼──────────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │      pieces/        │
                    │ Manage downloads    │
                    └─────────────────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │       file/         │
                    │   Write to disk     │
                    └─────────────────────┘
```

---

## Data Flow

### 1. Startup Flow
```
.torrent file
     │
     ▼ [bencode/Decode]
Torrent struct
     │
     ▼ [torrent/GenerateInfoHash]
20-byte SHA1 InfoHash
     │
     ▼ [tracker/Announce]
List of Peer IPs
     │
     ▼ [peer/ConnectToPeer]
Active Peer Connections
```

### 2. Download Flow
```
Peer Connection
     │
     ▼ [peer/PerformHandshake]
Handshake verified
     │
     ▼ [peer/ReadMessage]
Bitfield, Have, Unchoke messages
     │
     ▼ [pieces/GetPieceToRequest]
Select piece (rarest first)
     │
     ▼ [peer/SendMessage - Request]
Request 16KB blocks
     │
     ▼ [peer/ReadMessage - Piece]
Receive block data
     │
     ▼ [pieces/HandlePieceMessage]
Assemble blocks into piece
     │
     ▼ [piece/Validate]
SHA1 hash verification
     │
     ▼ [file/WritePiece]
Write to disk
```

---

## Module Architecture

### bencode/ - Bencode Serialization

The Bencode format is BitTorrent's data serialization format.

| File | Purpose |
|------|---------|
| `bencode_decode.go` | Decodes bencode → Go types (strings, ints, lists, dicts) |
| `bencode_encode.go` | Encodes Go types → bencode (used for info hash calculation) |

**Key Types:**
- Strings: `<length>:<content>` (e.g., `5:hello`)
- Integers: `i<number>e` (e.g., `i42e`)
- Lists: `l<items>e` (e.g., `li1ei2ee`)
- Dictionaries: `d<key><value>...e` (sorted keys)

---

### torrent/ - Torrent File Handling

Parses `.torrent` files and manages download coordination.

| File | Purpose |
|------|---------|
| `torrent.go` | Main `Torrent` struct definition |
| `parser.go` | Parses raw `.torrent` bytes into structs |
| `info.go` | `Info` struct (name, piece length, files) |
| `info_hash.go` | SHA1 hash of info dictionary (torrent identifier) |
| `file.go` | `File` struct for multi-file torrents |
| `download.go` | `Downloader` - orchestrates the entire download process |

**Key Structs:**

```go
type Torrent struct {
    Announce     string      // Tracker URL
    AnnounceList [][]string  // Backup trackers
    Info         *Info       // File metadata
    InfoHash     InfoHash    // 20-byte identifier
}

type Info struct {
    Name        string      // Torrent name
    PieceLength int64       // Bytes per piece (usually 256KB)
    Pieces      [][20]byte  // SHA1 hashes for each piece
    Length      *int64      // Single-file size
    Files       []File      // Multi-file list
}
```

---

### tracker/ - Tracker Communication

Communicates with BitTorrent trackers to discover peers.

| File | Purpose |
|------|---------|
| `types.go` | `TrackerClient`, `TrackerRequest`, `TrackerResponse` |
| `client.go` | Creates client, parses tracker responses |
| `announce.go` | Builds announce URL, makes HTTP request |
| `peers.go` | Parses compact/dictionary peer formats |

**Tracker Request Parameters:**
- `info_hash` - 20-byte torrent identifier
- `peer_id` - 20-byte client identifier
- `port` - Listening port
- `uploaded/downloaded/left` - Transfer stats
- `compact=1` - Request compact peer format

**Response:**
- `interval` - Re-announce interval
- `peers` - List of (IP, port) tuples

---

### peer/ - Peer Wire Protocol

Implements the BitTorrent peer-to-peer wire protocol (BEP 3).

| File | Purpose |
|------|---------|
| `peer.go` | `Peer` struct, bitfield operations |
| `connection.go` | TCP connection management, message loops |
| `handshake.go` | Protocol handshake (pstr + reserved + info_hash + peer_id) |
| `message.go` | Message types: Choke, Unchoke, Interested, Have, Bitfield, Request, Piece, Cancel |

**Handshake Format (68 bytes):**
```
┌────────┬───────────────────────┬──────────┬────────────┬──────────┐
│ 1 byte │      19 bytes         │ 8 bytes  │  20 bytes  │ 20 bytes │
│ pstrlen│ "BitTorrent protocol" │ reserved │ info_hash  │ peer_id  │
└────────┴───────────────────────┴──────────┴────────────┴──────────┘
```

**Message Format:**
```
┌──────────┬─────────┬─────────────┐
│ 4 bytes  │ 1 byte  │ variable    │
│ length   │ msg_id  │ payload     │
└──────────┴─────────┴─────────────┘
```

**Message Types:**
| ID | Name | Payload |
|----|------|---------|
| 0 | Choke | - |
| 1 | Unchoke | - |
| 2 | Interested | - |
| 3 | Not Interested | - |
| 4 | Have | piece index (4 bytes) |
| 5 | Bitfield | bitfield |
| 6 | Request | index, begin, length (12 bytes) |
| 7 | Piece | index, begin, data |
| 8 | Cancel | index, begin, length (12 bytes) |

---

### pieces/ - Piece Management

Manages piece state, block assembly, and validation.

| File | Purpose |
|------|---------|
| `piece.go` | `Piece` and `Block` structs, SHA1 validation |
| `manager.go` | Tracks piece state, handles incoming data, writes to files |
| `request.go` | `RequestManager` - tracks outstanding block requests |
| `selector.go` | `PieceSelector` - rarest-first piece selection |

**Piece Structure:**
```
┌─────────────────────────────────────────────────┐
│                    Piece                         │
│  (typically 256KB, verified by SHA1 hash)       │
├────────┬────────┬────────┬────────┬─────────────┤
│ Block 0│ Block 1│ Block 2│ Block 3│ ... Block N │
│ 16KB   │ 16KB   │ 16KB   │ 16KB   │ (last < 16KB)│
└────────┴────────┴────────┴────────┴─────────────┘
```

**Piece States:**
- `Pending` - Not yet started
- `Downloading` - Blocks being received
- `Complete` - All blocks received, hash verified
- `Failed` - Hash mismatch, needs re-download

---

### file/ - File System Operations

Handles mapping pieces to files and writing to disk.

| File | Purpose |
|------|---------|
| `allocator.go` | Preallocates disk space (sparse/full allocation) |
| `mapper.go` | Maps piece indices to file byte ranges |
| `progress.go` | Tracks download progress per file |
| `writer.go` | Writes piece data to correct file positions |

**Multi-File Mapping:**
```
Piece 0     Piece 1     Piece 2     Piece 3
┌───────────┬───────────┬───────────┬───────────┐
│████████████████████████│██████████████████████│
└───────────┴───────────┴───────────┴───────────┘
     │              │              │
     ▼              ▼              ▼
┌─────────────┐ ┌──────────────┐ ┌────────────┐
│  File A     │ │   File B     │ │  File C    │
│  (spans 2   │ │  (spans 2    │ │ (partial   │
│   pieces)   │ │   pieces)    │ │  piece)    │
└─────────────┘ └──────────────┘ └────────────┘
```

---

## File Reference

### main.go
**Entry point** - Orchestrates the entire download:
1. Parses command line args
2. Opens and parses torrent file
3. Contacts tracker for peers
4. Creates parallel peer connections
5. Monitors download progress
6. Handles graceful shutdown (Ctrl+C)

### internal/bencode/bencode_decode.go
**Bencode Decoder** with:
- `Decoder` struct with position tracking
- `Decode()` - Main entry point
- `DecodeString()`, `DecodeInt()`, `DecodeList()`, `DecodeDict()`
- Exports `Pos` and `Data` for raw info dict extraction

### internal/bencode/bencode_encode.go
**Bencode Encoder** with:
- `Encode()` - Converts Go types to bencode bytes
- Used for re-encoding info dict for hash calculation
- Handles nested structures via reflection

### internal/torrent/parser.go
**Torrent File Parser**:
- `Open()` - Reads file from disk
- `ParseTorrent()` - Decodes and validates
- `extractRawInfoDict()` - Gets exact bytes for hashing
- `parseInfoFromMap()` - Converts decoded map to `Info` struct

### internal/torrent/download.go
**Download Coordinator**:
- `Downloader` struct - holds torrent, peers, piece manager
- `Start()` - Initializes file system, starts download loop
- `handlePeer()` - Goroutine per peer for message handling
- `AddPeer()` / `RemovePeer()` - Dynamic peer management

### internal/tracker/client.go
**Tracker HTTP Client**:
- `NewTrackerClient()` - Creates client with peer ID and port
- `Announce()` - Makes HTTP request, parses response
- Handles both compact (6-byte) and dictionary peer formats

### internal/peer/connection.go
**Peer Connection Manager**:
- `ConnectToPeer()` - Dial + handshake in one call
- `Connection` struct - Wraps net.Conn with state
- `readLoop()` - Receives messages in goroutine
- `messageLoop()` - Processes message queue

### internal/peer/handshake.go
**BitTorrent Handshake**:
- `Handshake` struct - pstr, info_hash, peer_id
- `Serialize()` / `DeserializeHandshake()` - Wire format
- `PerformHandshake()` - Send + receive + verify

### internal/pieces/manager.go
**Piece State Manager**:
- `Manager` struct - tracks all pieces, pending downloads
- `GetPieceToRequest()` - Selects next piece (rarest first)
- `HandlePieceMessage()` - Processes incoming blocks
- Integrates with `file.Writer` for disk writes

### internal/pieces/piece.go
**Individual Piece**:
- `Piece` struct - hash, blocks, data buffer
- `SetBlock()` - Stores received block data
- `Validate()` - SHA1 hash verification
- `Reset()` - Clears for re-download on failure

### internal/file/writer.go
**Disk Writer**:
- `Writer` struct - holds file handles
- `Initialize()` - Creates/opens all files
- `WritePiece()` - Maps piece → file ranges → disk writes
- Handles pieces spanning multiple files

### internal/file/mapper.go
**Piece-to-File Mapper**:
- `Mapper` struct - precomputed piece→file mappings
- `PieceFileMap` - Which files a piece touches
- `GetPieceMapping()` - Returns file ranges for a piece

---

## Threading Model

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Goroutine                          │
│  - Parse torrent                                            │
│  - Contact tracker                                          │
│  - Spawn peer handlers                                      │
│  - Monitor progress                                         │
└─────────────────────────────────────────────────────────────┘
           │
           ├──────────── Peer Handler 1 ──────────────────────┐
           │               └── readLoop (goroutine)           │
           │                                                   │
           ├──────────── Peer Handler 2 ──────────────────────┤
           │               └── readLoop (goroutine)           │
           │                                                   │
           └──────────── Peer Handler N ──────────────────────┘
                          └── readLoop (goroutine)

All peer handlers share:
  - PieceManager (mutex-protected)
  - FileWriter (mutex-protected)
```

---

## Error Handling

| Error Type | Handling |
|------------|----------|
| Tracker timeout | Log error, exit (could retry) |
| Peer connection fail | Skip peer, try next |
| Handshake fail | Close connection, try next peer |
| Piece hash mismatch | Reset piece, re-download |
| Disk write fail | Log error, reset piece |
