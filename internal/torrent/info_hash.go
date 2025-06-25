package torrent

import (
	"crypto/sha1"
	"encoding/hex"
)

type InfoHash [20]byte

func (ih InfoHash) String() string {
	return hex.EncodeToString(ih[:])
}

func (t *Torrent) GenerateInfoHash(rawInfoDict []byte) InfoHash {
	hash := sha1.Sum(rawInfoDict)
	return InfoHash(hash)
}
