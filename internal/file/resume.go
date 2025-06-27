package file

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ResumeState represents the saved state of a download
type ResumeState struct {
	InfoHash        string            `json:"info_hash"`        // Torrent info hash
	TorrentName     string            `json:"torrent_name"`     // Name of the torrent
	TotalPieces     int               `json:"total_pieces"`     // Total number of pieces
	PieceLength     int64             `json:"piece_length"`     // Length of each piece
	TotalLength     int64             `json:"total_length"`     // Total torrent length
	CompletedPieces []bool            `json:"completed_pieces"` // Which pieces are complete
	FileStates      []FileResumeState `json:"file_states"`      // State of each file
	LastSaved       time.Time         `json:"last_saved"`       // When state was last saved
	OutputDir       string            `json:"output_dir"`       // Output directory
	PieceHashes     []string          `json:"piece_hashes"`     // SHA1 hashes for verification
}

// FileResumeState represents the state of a single file
type FileResumeState struct {
	Path         string    `json:"path"`          // File path
	Length       int64     `json:"length"`        // Expected file length
	WrittenBytes int64     `json:"written_bytes"` // Bytes written so far
	IsComplete   bool      `json:"is_complete"`   // Whether file is complete
	LastModified time.Time `json:"last_modified"` // Last modification time
}

// ResumeManager handles saving and loading download state
type ResumeManager struct {
	stateFile string
	infoHash  string
}

// NewResumeManager creates a new resume manager
func NewResumeManager(outputDir, infoHash string) *ResumeManager {
	stateFileName := fmt.Sprintf(".torrent_%s.resume", infoHash)
	stateFile := filepath.Join(outputDir, stateFileName)

	return &ResumeManager{
		stateFile: stateFile,
		infoHash:  infoHash,
	}
}

// SaveState saves the current download state
func (rm *ResumeManager) SaveState(
	torrentName string,
	totalPieces int,
	pieceLength int64,
	totalLength int64,
	completedPieces []bool,
	files []FileInfo,
	progress *Progress,
	outputDir string,
	pieceHashes [][]byte,
) error {
	// Convert piece hashes to strings
	hashStrings := make([]string, len(pieceHashes))
	for i, hash := range pieceHashes {
		hashStrings[i] = fmt.Sprintf("%x", hash)
	}

	// Get file states from progress
	fileStates := make([]FileResumeState, len(files))
	for i, file := range files {
		fileProgress, _ := progress.GetFileProgress(i)

		// Get file modification time
		fullPath := filepath.Join(outputDir, file.Path)
		var lastModified time.Time
		if stat, err := os.Stat(fullPath); err == nil {
			lastModified = stat.ModTime()
		}

		fileStates[i] = FileResumeState{
			Path:         file.Path,
			Length:       file.Length,
			WrittenBytes: fileProgress.WrittenBytes,
			IsComplete:   fileProgress.IsComplete,
			LastModified: lastModified,
		}
	}

	// Create resume state
	state := ResumeState{
		InfoHash:        rm.infoHash,
		TorrentName:     torrentName,
		TotalPieces:     totalPieces,
		PieceLength:     pieceLength,
		TotalLength:     totalLength,
		CompletedPieces: completedPieces,
		FileStates:      fileStates,
		LastSaved:       time.Now(),
		OutputDir:       outputDir,
		PieceHashes:     hashStrings,
	}

	// Write to temporary file first
	tempFile := rm.stateFile + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	defer file.Close()

	// Encode state as JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(state)
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	// Sync to disk
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync state file: %w", err)
	}

	file.Close()

	// Atomically replace the old state file
	err = os.Rename(tempFile, rm.stateFile)
	if err != nil {
		return fmt.Errorf("failed to replace state file: %w", err)
	}

	return nil
}

// LoadState loads the download state from disk
func (rm *ResumeManager) LoadState() (*ResumeState, error) {
	// Check if state file exists
	if _, err := os.Stat(rm.stateFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("no resume state found")
	}

	// Open state file
	file, err := os.Open(rm.stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}
	defer file.Close()

	// Decode state
	var state ResumeState
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&state)
	if err != nil {
		return nil, fmt.Errorf("failed to decode state: %w", err)
	}

	// Verify info hash matches
	if state.InfoHash != rm.infoHash {
		return nil, fmt.Errorf("info hash mismatch: expected %s, got %s",
			rm.infoHash, state.InfoHash)
	}

	return &state, nil
}

// VerifyFiles verifies that files on disk match the expected state
func (rm *ResumeManager) VerifyFiles(state *ResumeState) error {
	for _, fileState := range state.FileStates {
		fullPath := filepath.Join(state.OutputDir, fileState.Path)

		// Check if file exists
		stat, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist, that's okay if it's not marked complete
				if fileState.IsComplete {
					return fmt.Errorf("file %s marked complete but doesn't exist", fileState.Path)
				}
				continue
			}
			return fmt.Errorf("failed to stat file %s: %w", fileState.Path, err)
		}

		// Check file size
		if stat.Size() > fileState.Length {
			return fmt.Errorf("file %s is larger than expected: %d > %d",
				fileState.Path, stat.Size(), fileState.Length)
		}

		// If file is marked complete, verify size matches
		if fileState.IsComplete && stat.Size() != fileState.Length {
			return fmt.Errorf("file %s marked complete but has wrong size: %d != %d",
				fileState.Path, stat.Size(), fileState.Length)
		}
	}

	return nil
}

// VerifyPieces verifies completed pieces by checking their hashes
func (rm *ResumeManager) VerifyPieces(
	state *ResumeState,
	mapper *Mapper,
) ([]bool, error) {
	verifiedPieces := make([]bool, len(state.CompletedPieces))

	for pieceIndex, isComplete := range state.CompletedPieces {
		if !isComplete {
			continue
		}

		// Get piece mapping
		mapping, err := mapper.GetPieceMapping(pieceIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to get piece mapping for piece %d: %w", pieceIndex, err)
		}

		// Read piece data from files
		pieceData, err := rm.readPieceFromFiles(pieceIndex, mapping, state)
		if err != nil {
			fmt.Printf("Warning: Failed to read piece %d: %v\n", pieceIndex, err)
			continue
		}

		// Verify hash
		if rm.verifyPieceHash(pieceData, state.PieceHashes[pieceIndex]) {
			verifiedPieces[pieceIndex] = true
		} else {
			fmt.Printf("Warning: Piece %d failed hash verification\n", pieceIndex)
		}
	}

	return verifiedPieces, nil
}

// readPieceFromFiles reads piece data from the actual files
func (rm *ResumeManager) readPieceFromFiles(
	pieceIndex int,
	mapping PieceFileMap,
	state *ResumeState,
) ([]byte, error) {
	var pieceData []byte

	for _, fileRange := range mapping.FileRanges {
		fullPath := filepath.Join(state.OutputDir, fileRange.FilePath)

		// Open file
		file, err := os.Open(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
		}
		defer file.Close()

		// Seek to correct position
		_, err = file.Seek(fileRange.Offset, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to seek in file %s: %w", fullPath, err)
		}

		// Read data
		data := make([]byte, fileRange.Length)
		_, err = io.ReadFull(file, data)
		if err != nil {
			return nil, fmt.Errorf("failed to read from file %s: %w", fullPath, err)
		}

		pieceData = append(pieceData, data...)
	}

	return pieceData, nil
}

// verifyPieceHash verifies that piece data matches expected hash
func (rm *ResumeManager) verifyPieceHash(data []byte, expectedHashStr string) bool {
	hash := sha1.Sum(data)
	actualHashStr := fmt.Sprintf("%x", hash[:])
	return actualHashStr == expectedHashStr
}

// DeleteState removes the resume state file
func (rm *ResumeManager) DeleteState() error {
	err := os.Remove(rm.stateFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	return nil
}

// HasResumeState checks if a resume state file exists
func (rm *ResumeManager) HasResumeState() bool {
	_, err := os.Stat(rm.stateFile)
	return err == nil
}

// GetStateFile returns the path to the state file
func (rm *ResumeManager) GetStateFile() string {
	return rm.stateFile
}

// CleanupOldStates removes old resume state files
func (rm *ResumeManager) CleanupOldStates(maxAge time.Duration) error {
	dir := filepath.Dir(rm.stateFile)
	pattern := filepath.Join(dir, ".torrent_*.resume")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find resume files: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	for _, file := range matches {
		stat, err := os.Stat(file)
		if err != nil {
			continue
		}

		if stat.ModTime().Before(cutoff) {
			err = os.Remove(file)
			if err != nil {
				fmt.Printf("Warning: Failed to remove old resume file %s: %v\n", file, err)
			}
		}
	}

	return nil
}

// RestoreProgress restores progress tracking from resume state
func (rm *ResumeManager) RestoreProgress(state *ResumeState) *Progress {
	// Create file info from state
	files := make([]FileInfo, len(state.FileStates))
	for i, fileState := range state.FileStates {
		files[i] = FileInfo{
			Path:   fileState.Path,
			Length: fileState.Length,
			Offset: 0, // Will be recalculated by mapper
		}
	}

	// Create progress tracker
	progress := NewProgress(files)

	// Restore progress from state
	for i, fileState := range state.FileStates {
		if fileState.WrittenBytes > 0 {
			progress.AddWrittenBytes(i, fileState.WrittenBytes)
		}
		if fileState.IsComplete {
			progress.SetFileComplete(i, true)
		}
	}

	return progress
}

// UpdateFileStates updates file states from current progress
func (rm *ResumeManager) UpdateFileStates(
	progress *Progress,
	files []FileInfo,
	outputDir string,
) []FileResumeState {
	fileStates := make([]FileResumeState, len(files))

	for i, file := range files {
		fileProgress, _ := progress.GetFileProgress(i)

		// Get file modification time
		fullPath := filepath.Join(outputDir, file.Path)
		var lastModified time.Time
		if stat, err := os.Stat(fullPath); err == nil {
			lastModified = stat.ModTime()
		}

		fileStates[i] = FileResumeState{
			Path:         file.Path,
			Length:       file.Length,
			WrittenBytes: fileProgress.WrittenBytes,
			IsComplete:   fileProgress.IsComplete,
			LastModified: lastModified,
		}
	}

	return fileStates
}
