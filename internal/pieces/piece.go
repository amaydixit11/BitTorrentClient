package piece

import (
	"bytes"
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

	block := p.Blocks[blockIndex]

	if block.Begin != begin {
		return fmt.Errorf("block begin mismatch: expected %d, got %d",
			block.Begin, begin)
	}

	if int64(len(data)) != block.Length {
		return fmt.Errorf("block length mismatch: expected %d, got %d",
			block.Length, len(data))
	}

	// 🚫 Skip if already downloaded
	if p.Downloaded[blockIndex] {
		// Optional: log or silently skip
		fmt.Printf("⚠️  Duplicate block: piece %d, block %d, skipping\n", p.Index, blockIndex)
		return nil
	}

	// ✅ Copy data
	copy(p.Data[begin:begin+int64(len(data))], data)
	p.Downloaded[blockIndex] = true
	p.Blocks[blockIndex].Data = data
	if begin+int64(len(data)) > int64(len(p.Data)) {
		return fmt.Errorf("write out of bounds: offset %d + %d > %d", begin, len(data), len(p.Data))
	}

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
	if len(p.Data) != int(p.Length) {
		fmt.Printf("Piece %d length mismatch: expected %d, got %d\n", p.Index, p.Length, len(p.Data))
		return false
	}

	hash := sha1.Sum(p.Data[:p.Length])

	fmt.Printf("Piece %d validation - Expected: %x, Got: %x\n", p.Index, p.Hash, hash)

	isValid := bytes.Equal(hash[:], p.Hash[:])
	if isValid {
		fmt.Printf("✅ Piece %d validated successfully!\n", p.Index)
	} else {
		fmt.Printf("❌ Piece %d validation failed!\n", p.Index)
	}
	return isValid
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
