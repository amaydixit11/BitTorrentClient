// internal/piece/piece.go
package piece

import (
	"crypto/sha1"
	"fmt"
	"time"
)

const (
	BlockSize          = 16384 // 16KB blocks
	MaxRequestsPerPeer = 5
	RequestTimeout     = 30 * time.Second
)

// Piece represents a single piece of the torrent
type Piece struct {
	Index      int
	Hash       [20]byte
	Length     int64
	Blocks     []Block
	Downloaded []bool // Track which blocks are downloaded
	Complete   bool
	Data       []byte
}

// Block represents a block within a piece
type Block struct {
	Begin  int64
	Length int64
	Data   []byte
}

// Request represents a pending block request
type Request struct {
	PieceIndex int64
	Begin      int64
	Length     int64
	Requested  time.Time
	PeerID     [20]byte
}

// NewPiece creates a new piece
func NewPiece(index int, hash [20]byte, length int64) *Piece {
	numBlocks := (length + BlockSize - 1) / BlockSize
	blocks := make([]Block, numBlocks)
	downloaded := make([]bool, numBlocks)

	// Initialize blocks
	var i int64 = 0
	for ; i < numBlocks; i++ {
		var begin int64 = int64(i) * BlockSize
		var blockLength int64 = BlockSize

		// Last block might be smaller
		if (begin + blockLength) > length {
			blockLength = length - begin
		}

		blocks[i] = Block{
			Begin:  begin,
			Length: blockLength,
		}
	}

	return &Piece{
		Index:      index,
		Hash:       hash,
		Length:     length,
		Blocks:     blocks,
		Downloaded: downloaded,
		Complete:   false,
		Data:       make([]byte, length),
	}
}

// SetBlock sets data for a specific block
func (p *Piece) SetBlock(begin int64, data []byte) error {
	blockIndex := begin / BlockSize
	if int(blockIndex) >= len(p.Blocks) {
		return fmt.Errorf("block index out of range: %d", blockIndex)
	}

	if int(p.Blocks[blockIndex].Begin) != int(begin) {
		return fmt.Errorf("block begin mismatch: expected %d, got %d",
			p.Blocks[blockIndex].Begin, begin)
	}

	if len(data) != int(p.Blocks[blockIndex].Length) {
		return fmt.Errorf("block length mismatch: expected %d, got %d",
			p.Blocks[blockIndex].Length, len(data))
	}

	// Copy data to piece buffer
	copy(p.Data[begin:int(begin)+len(data)], data)
	p.Downloaded[blockIndex] = true
	p.Blocks[blockIndex].Data = data

	// Check if piece is complete
	p.checkComplete()
	return nil
}

// checkComplete checks if all blocks are downloaded
func (p *Piece) checkComplete() {
	for _, downloaded := range p.Downloaded {
		if !downloaded {
			return
		}
	}
	p.Complete = true
}

// Validate validates the piece against its hash
func (p *Piece) Validate() bool {
	if !p.Complete {
		return false
	}

	hash := sha1.Sum(p.Data)
	return hash == p.Hash
}

// GetMissingBlocks returns list of blocks that haven't been downloaded
func (p *Piece) GetMissingBlocks() []Block {
	var missing []Block
	for i, downloaded := range p.Downloaded {
		if !downloaded {
			missing = append(missing, p.Blocks[i])
		}
	}
	return missing
}

// GetNextBlock returns the next block that needs to be requested
func (p *Piece) GetNextBlock() *Block {
	for i, downloaded := range p.Downloaded {
		if !downloaded {
			return &p.Blocks[i]
		}
	}
	return nil
}

// Reset resets the piece to empty state
func (p *Piece) Reset() {
	for i := range p.Downloaded {
		p.Downloaded[i] = false
		p.Blocks[i].Data = nil
	}
	p.Complete = false
	p.Data = make([]byte, p.Length)
}
