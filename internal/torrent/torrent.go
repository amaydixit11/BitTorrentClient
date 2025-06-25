package torrent

import "errors"

// Torrent represents a parsed torrent file
type Torrent struct {
	Announce     string     `bencode:"announce"`
	AnnounceList [][]string `bencode:"announce-list,omitempty"`
	Info         *Info      `bencode:"info"`
	Comment      *string    `bencode:"comment,omitempty"`
	CreatedBy    *string    `bencode:"created by,omitempty"`
	CreationDate *int64     `bencode:"creation date,omitempty"`

	// Calculated fields (not from bencode)
	InfoHash    InfoHash `bencode:"-"`
	rawInfoDict []byte   `bencode:"-"` // Store for hash calculation
}

// In torrent.go
func (t *Torrent) Validate() error {
	if t.Announce == "" && len(t.AnnounceList) == 0 {
		return errors.New("no announce URLs provided")
	}

	if t.Info == nil {
		return errors.New("missing info dictionary")
	}

	return t.Info.Validate()
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
