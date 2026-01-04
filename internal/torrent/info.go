package torrent

import (
	"errors"
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
