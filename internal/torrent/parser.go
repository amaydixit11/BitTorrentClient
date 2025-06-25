package torrent

import (
	"bittorrentclient/internal/bencode"
	"fmt"
	"os"
)

// In parser.go
func Open(filename string) (*Torrent, error) {
	Data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read torrent file: %w", err)
	}

	return ParseTorrent(Data)
}

func ParseTorrent(Data []byte) (*Torrent, error) {
	// First, decode the entire torrent file
	decoded, err := bencode.Decode(Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bencode: %w", err)
	}

	// Convert to map
	torrentMap, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent file is not a dictionary")
	}

	// Extract raw info dictionary for InfoHash calculation
	rawInfoDict, err := extractRawInfoDict(Data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract info dictionary: %w", err)
	}

	// Parse the torrent structure
	torrent, err := parseTorrentFromMap(torrentMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse torrent structure: %w", err)
	}

	// Calculate InfoHash from raw info dictionary
	torrent.InfoHash = torrent.GenerateInfoHash(rawInfoDict)

	// Validate the parsed torrent
	if err := torrent.Validate(); err != nil {
		return nil, fmt.Errorf("torrent validation failed: %w", err)
	}

	return torrent, nil
}

// extractRawInfoDict extracts the raw bencode bytes for the "info" dictionary
// This is crucial for InfoHash calculation as it must be the exact bytes
func extractRawInfoDict(Data []byte) ([]byte, error) {
	decoder := bencode.NewDecoder(Data)

	// We need to manually parse to find the "info" key and extract its raw bytes
	if decoder.Pos >= len(decoder.Data) || decoder.Data[decoder.Pos] != 'd' {
		return nil, fmt.Errorf("torrent file must start with dictionary")
	}

	decoder.Pos++ // skip 'd'

	for decoder.Pos < len(decoder.Data) && decoder.Data[decoder.Pos] != 'e' {
		// Decode key
		key, err := decoder.DecodeString()
		if err != nil {
			return nil, fmt.Errorf("error decoding key: %w", err)
		}

		if key == "info" {
			// Found info key, now extract the raw value bytes
			valueStart := decoder.Pos
			_, err := decoder.Decode() // This advances the position past the value
			if err != nil {
				return nil, fmt.Errorf("error decoding info value: %w", err)
			}
			valueEnd := decoder.Pos

			return decoder.Data[valueStart:valueEnd], nil
		} else {
			// Skip this key-value pair
			_, err := decoder.Decode()
			if err != nil {
				return nil, fmt.Errorf("error skipping value: %w", err)
			}
		}
	}

	return nil, fmt.Errorf("info dictionary not found")
}

// parseTorrentFromMap converts the decoded map to a Torrent struct
func parseTorrentFromMap(torrentMap map[string]interface{}) (*Torrent, error) {
	torrent := &Torrent{}

	// Parse announce
	if announce, ok := torrentMap["announce"].(string); ok {
		torrent.Announce = announce
	}

	// Parse announce-list (optional)
	if announceList, ok := torrentMap["announce-list"].([]interface{}); ok {
		for _, tierInterface := range announceList {
			if tier, ok := tierInterface.([]interface{}); ok {
				var tierStrings []string
				for _, urlInterface := range tier {
					if url, ok := urlInterface.(string); ok {
						tierStrings = append(tierStrings, url)
					}
				}
				if len(tierStrings) > 0 {
					torrent.AnnounceList = append(torrent.AnnounceList, tierStrings)
				}
			}
		}
	}

	// Parse optional fields
	if comment, ok := torrentMap["comment"].(string); ok {
		torrent.Comment = &comment
	}

	if createdBy, ok := torrentMap["created by"].(string); ok {
		torrent.CreatedBy = &createdBy
	}

	if creationDate, ok := torrentMap["creation date"].(int64); ok {
		torrent.CreationDate = &creationDate
	}

	// Parse info dictionary
	infoInterface, ok := torrentMap["info"]
	if !ok {
		return nil, fmt.Errorf("missing info dictionary")
	}

	infoMap, ok := infoInterface.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("info is not a dictionary")
	}

	info, err := parseInfoFromMap(infoMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse info dictionary: %w", err)
	}

	torrent.Info = info
	return torrent, nil
}

// parseInfoFromMap converts the info map to an Info struct
func parseInfoFromMap(infoMap map[string]interface{}) (*Info, error) {
	info := &Info{}

	// Parse required fields
	name, ok := infoMap["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid name field")
	}
	info.Name = name

	pieceLength, ok := infoMap["piece length"].(int64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid piece length field")
	}
	info.PieceLength = pieceLength

	piecesStr, ok := infoMap["pieces"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid pieces field")
	}

	// Convert pieces string to [][20]byte
	if len(piecesStr)%20 != 0 {
		return nil, fmt.Errorf("invalid pieces length: must be multiple of 20")
	}

	numPieces := len(piecesStr) / 20
	info.Pieces = make([][20]byte, numPieces)

	for i := 0; i < numPieces; i++ {
		copy(info.Pieces[i][:], piecesStr[i*20:(i+1)*20])
	}

	// Parse single-file vs multi-file
	if length, ok := infoMap["length"].(int64); ok {
		// Single-file torrent
		info.Length = &length

		if md5sum, ok := infoMap["md5sum"].(string); ok {
			info.MD5Sum = &md5sum
		}
	} else if filesInterface, ok := infoMap["files"].([]interface{}); ok {
		// Multi-file torrent
		for _, fileInterface := range filesInterface {
			fileMap, ok := fileInterface.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid file entry")
			}

			file, err := parseFileFromMap(fileMap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse file: %w", err)
			}

			info.Files = append(info.Files, *file)
		}
	} else {
		return nil, fmt.Errorf("torrent must have either 'length' or 'files' field")
	}

	return info, nil
}

// parseFileFromMap converts a file map to a File struct
func parseFileFromMap(fileMap map[string]interface{}) (*File, error) {
	file := &File{}

	length, ok := fileMap["length"].(int64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid file length")
	}
	file.Length = length

	pathInterface, ok := fileMap["path"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid file path")
	}

	for _, pathComponent := range pathInterface {
		if component, ok := pathComponent.(string); ok {
			file.Path = append(file.Path, component)
		} else {
			return nil, fmt.Errorf("invalid path component")
		}
	}

	if md5sum, ok := fileMap["md5sum"].(string); ok {
		file.MD5Sum = &md5sum
	}

	// Validate the file path
	if err := file.ValidatePath(); err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	return file, nil
}
