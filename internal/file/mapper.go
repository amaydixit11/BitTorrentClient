package file

import (
	"fmt"
	"path/filepath"
)

// FileRange represents a range of bytes within a file
type FileRange struct {
	FileIndex int    // Index in the torrent's file list
	FilePath  string // Full path to the file
	Offset    int64  // Offset within the file
	Length    int64  // Number of bytes
}

// PieceFileMap represents the mapping of a piece to files
type PieceFileMap struct {
	PieceIndex int         // The piece index
	FileRanges []FileRange // Files and ranges this piece affects
}

// Mapper handles piece-to-file mapping calculations
type Mapper struct {
	files       []FileInfo     // File information from torrent
	pieceLength int64          // Length of each piece
	totalLength int64          // Total torrent length
	pieceMaps   []PieceFileMap // Pre-calculated mappings
}

// FileInfo represents information about a file in the torrent
type FileInfo struct {
	Path   string // Relative path from torrent root
	Length int64  // File length in bytes
	Offset int64  // Cumulative offset in torrent data
}

// NewMapper creates a new file mapper
func NewMapper(files []FileInfo, pieceLength int64, totalLength int64) *Mapper {
	mapper := &Mapper{
		files:       files,
		pieceLength: pieceLength,
		totalLength: totalLength,
	}

	mapper.buildFileMappings()
	return mapper
}

// buildFileMappings pre-calculates piece-to-file mappings
func (m *Mapper) buildFileMappings() {
	totalPieces := int((m.totalLength + m.pieceLength - 1) / m.pieceLength)
	m.pieceMaps = make([]PieceFileMap, totalPieces)

	for pieceIndex := 0; pieceIndex < totalPieces; pieceIndex++ {
		m.pieceMaps[pieceIndex] = m.calculatePieceMapping(pieceIndex)
	}
}

// calculatePieceMapping calculates which files a piece affects
func (m *Mapper) calculatePieceMapping(pieceIndex int) PieceFileMap {
	pieceStart := int64(pieceIndex) * m.pieceLength
	pieceEnd := pieceStart + m.pieceLength

	// Handle last piece which might be smaller
	if pieceEnd > m.totalLength {
		pieceEnd = m.totalLength
	}

	var fileRanges []FileRange

	// Find all files that this piece overlaps
	for fileIndex, file := range m.files {
		fileStart := file.Offset
		fileEnd := file.Offset + file.Length

		// Check if piece overlaps with this file
		if pieceStart < fileEnd && pieceEnd > fileStart {
			// Calculate overlap
			overlapStart := max(pieceStart, fileStart)
			overlapEnd := min(pieceEnd, fileEnd)

			fileRange := FileRange{
				FileIndex: fileIndex,
				FilePath:  file.Path,
				Offset:    overlapStart - fileStart, // Offset within the file
				Length:    overlapEnd - overlapStart,
			}

			fileRanges = append(fileRanges, fileRange)
		}
	}

	return PieceFileMap{
		PieceIndex: pieceIndex,
		FileRanges: fileRanges,
	}
}

// GetPieceMapping returns the file mapping for a specific piece
func (m *Mapper) GetPieceMapping(pieceIndex int) (PieceFileMap, error) {
	if pieceIndex < 0 || pieceIndex >= len(m.pieceMaps) {
		return PieceFileMap{}, fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	return m.pieceMaps[pieceIndex], nil
}

// GetAllFiles returns all files in the torrent
func (m *Mapper) GetAllFiles() []FileInfo {
	return m.files
}

// GetTotalFiles returns the number of files
func (m *Mapper) GetTotalFiles() int {
	return len(m.files)
}

// ValidatePieceData validates that piece data can be properly mapped
func (m *Mapper) ValidatePieceData(pieceIndex int, data []byte) error {
	mapping, err := m.GetPieceMapping(pieceIndex)
	if err != nil {
		return err
	}

	expectedLength := int64(0)
	for _, fileRange := range mapping.FileRanges {
		expectedLength += fileRange.Length
	}

	if int64(len(data)) != expectedLength {
		return fmt.Errorf("piece data length mismatch: expected %d, got %d",
			expectedLength, len(data))
	}

	return nil
}

// Helper functions
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// CreateFileInfoFromTorrent creates FileInfo slice from torrent data
func CreateFileInfoFromTorrent(torrentFiles []TorrentFile, isSingleFile bool, torrentName string) []FileInfo {
	var files []FileInfo
	currentOffset := int64(0)

	if isSingleFile {
		// Single file torrent
		files = append(files, FileInfo{
			Path:   torrentName,
			Length: torrentFiles[0].Length,
			Offset: 0,
		})
	} else {
		// Multi-file torrent
		for _, tFile := range torrentFiles {
			// Build full path from torrent name and file path
			fullPath := filepath.Join(torrentName, filepath.Join(tFile.Path...))

			files = append(files, FileInfo{
				Path:   fullPath,
				Length: tFile.Length,
				Offset: currentOffset,
			})

			currentOffset += tFile.Length
		}
	}

	return files
}

// TorrentFile represents a file in the torrent metadata
type TorrentFile struct {
	Path   []string // Path components
	Length int64    // File length
}
