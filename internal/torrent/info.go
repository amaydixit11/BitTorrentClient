package torrent

import (
	"errors"
	"fmt"
)

// Info represents the info dictionary of a torrent
type Info struct {
	Name        string     `bencode:"name"`
	PieceLength int64      `bencode:"piece length"`
	Pieces      [][20]byte `bencode:"pieces"`

	Length *int64  `bencode:"length,omitempty"`
	MD5Sum *string `bencode:"md5sum,omitempty"`

	Files []File `bencode:"files,omitempty"`
}

// IsSingleFile returns true if this is a single-file torrent
func (i *Info) IsSingleFile() bool {
	return i.Length != nil
}

// IsMultiFile returns true if this is a multi-file torrent
func (i *Info) IsMultiFile() bool {
	return len(i.Files) > 0
}

// TotalLength returns the total length of all files
func (i *Info) TotalLength() int64 {
	if i.IsSingleFile() {
		return *i.Length
	}

	var total int64
	for _, file := range i.Files {
		total += file.Length
	}
	return total
}

// NumPieces returns the total number of pieces
func (i *Info) NumPieces() int {
	return len(i.Pieces)
}

// PieceHash returns the hash for a specific piece
func (i *Info) PieceHash(index int) ([20]byte, error) {
	if index < 0 || index >= len(i.Pieces) {
		return [20]byte{}, fmt.Errorf("piece index %d out of range", index)
	}
	return i.Pieces[index], nil
}

// LastPieceLength returns the length of the last piece (often smaller)
func (i *Info) LastPieceLength() int64 {
	if len(i.Pieces) == 0 {
		return 0
	}

	totalLength := i.TotalLength()
	remainder := totalLength % i.PieceLength

	if remainder == 0 {
		return i.PieceLength
	}
	return remainder
}

// In info.go
func (i *Info) Validate() error {
	if i.Name == "" {
		return errors.New("torrent name cannot be empty")
	}

	if i.PieceLength <= 0 {
		return errors.New("piece length must be positive")
	}

	if len(i.Pieces) == 0 {
		return errors.New("no piece hashes provided")
	}

	// Validate single vs multi-file consistency
	if i.IsSingleFile() && i.IsMultiFile() {
		return errors.New("torrent cannot be both single-file and multi-file")
	}

	if !i.IsSingleFile() && !i.IsMultiFile() {
		return errors.New("torrent must specify either length or files")
	}

	return nil
}

func (i *Info) GetTotalLength() int64 {
	if i.IsSingleFile() {
		return *i.Length
	}

	var total int64
	for _, file := range i.Files {
		total += file.Length
	}
	return total
}
