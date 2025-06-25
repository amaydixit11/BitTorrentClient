package torrent

import (
	"bittorrentclient/internal/bencode"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

type InfoHash [20]byte

func (ih InfoHash) String() string {
	return hex.EncodeToString(ih[:])
}

func (t *Torrent) GenerateInfoHash(rawInfoDict []byte) InfoHash {
	hash := sha1.Sum(rawInfoDict)
	return InfoHash(hash)
}

// CalculateInfoHash computes the SHA1 hash of the bencoded info dictionary
func (t *Torrent) CalculateInfoHash(rawTorrentData []byte) ([]byte, error) {
	// Parse the raw torrent to find the info dictionary boundaries
	decoded, err := bencode.Decode(rawTorrentData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode torrent: %v", err)
	}

	torrentDict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent is not a dictionary")
	}

	// Find the raw info dictionary in the original data
	// This is a bit tricky - we need to find where "info" starts in the raw data
	// For now, let's use a simpler approach by re-encoding the info dict

	infoDict := torrentDict["info"]
	if infoDict == nil {
		return nil, fmt.Errorf("no info dictionary found")
	}

	// Re-encode the info dictionary
	infoBytes, err := bencode.Encode(infoDict)
	if err != nil {
		return nil, fmt.Errorf("failed to encode info dict: %v", err)
	}

	// Calculate SHA1 hash
	hash := sha1.Sum(infoBytes)
	return hash[:], nil
}
