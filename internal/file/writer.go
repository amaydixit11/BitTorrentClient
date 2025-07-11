package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Writer handles writing piece data to files
type Writer struct {
	mu           sync.RWMutex
	mapper       *Mapper
	outputDir    string
	fileHandles  map[string]*os.File // Cache of open file handles
	maxOpenFiles int                 // Maximum number of open files
	allocator    *Allocator
	progress     *Progress
}

// NewWriter creates a new file writer
func NewWriter(mapper *Mapper, outputDir string) *Writer {
	return &Writer{
		mapper:       mapper,
		outputDir:    outputDir,
		fileHandles:  make(map[string]*os.File),
		maxOpenFiles: 100, // Reasonable default
		allocator:    NewAllocator(outputDir),
		progress:     NewProgress(mapper.GetAllFiles()),
	}
}

// Initialize prepares the file structure and allocates space
func (w *Writer) Initialize() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Create output directory
	err := os.MkdirAll(w.outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create file structure and allocate space
	files := w.mapper.GetAllFiles()
	for _, file := range files {
		fullPath := filepath.Join(w.outputDir, file.Path)

		// Create directory structure
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Allocate file space
		err = w.allocator.AllocateFile(fullPath, file.Length)
		if err != nil {
			return fmt.Errorf("failed to allocate file %s: %w", fullPath, err)
		}
	}

	fmt.Printf("Initialized file structure in %s\n", w.outputDir)
	return nil
}

// WritePiece writes a completed piece to its corresponding files
func (w *Writer) WritePiece(pieceIndex int, data []byte) error {

	// Validate piece data
	err := w.mapper.ValidatePieceData(pieceIndex, data)
	if err != nil {
		return fmt.Errorf("piece validation failed: %w", err)
	}

	// Get piece mapping
	mapping, err := w.mapper.GetPieceMapping(pieceIndex)
	if err != nil {
		return fmt.Errorf("failed to get piece mapping: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	dataOffset := int64(0)

	// Write to each file that this piece affects
	for _, fileRange := range mapping.FileRanges {
		fullPath := filepath.Join(w.outputDir, fileRange.FilePath)

		// Get file handle
		file, err := w.getFileHandle(fullPath)
		if err != nil {
			return fmt.Errorf("failed to get file handle for %s: %w", fullPath, err)
		}

		// Seek to correct position
		_, err = file.Seek(fileRange.Offset, 0)
		if err != nil {
			return fmt.Errorf("failed to seek in file %s: %w", fullPath, err)
		}

		if dataOffset+fileRange.Length > int64(len(data)) {
			return fmt.Errorf("data slice overflow: offset=%d + length=%d > data=%d",
				dataOffset, fileRange.Length, len(data))
		}

		// Write data
		dataToWrite := data[dataOffset : dataOffset+fileRange.Length]
		written, err := file.Write(dataToWrite)
		if err != nil {
			return fmt.Errorf("failed to write to file %s: %w", fullPath, err)
		}

		if int64(written) != fileRange.Length {
			return fmt.Errorf("incomplete write to file %s: wrote %d, expected %d",
				fullPath, written, fileRange.Length)
		}

		// Update progress
		w.progress.AddWrittenBytes(fileRange.FileIndex, fileRange.Length)

		dataOffset += fileRange.Length
		fmt.Printf("Piece %d, Writing to %s, fileOffset=%d, dataOffset=%d, len=%d\n",
			pieceIndex, fileRange.FilePath, fileRange.Offset, dataOffset, fileRange.Length)

	}

	// Sync files to ensure data is written to disk
	for _, fileRange := range mapping.FileRanges {
		fullPath := filepath.Join(w.outputDir, fileRange.FilePath)
		if file, exists := w.fileHandles[fullPath]; exists {
			file.Sync()
		}
	}

	fmt.Printf("Wrote piece %d to %d files\n", pieceIndex, len(mapping.FileRanges))
	return nil
}

// getFileHandle gets or creates a file handle
func (w *Writer) getFileHandle(fullPath string) (*os.File, error) {
	// Check if we already have this file open
	if file, exists := w.fileHandles[fullPath]; exists {
		return file, nil
	}

	// Check if we need to close some files first
	if len(w.fileHandles) >= w.maxOpenFiles {
		w.closeOldestFile()
	}

	// Open the file
	file, err := os.OpenFile(fullPath, os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	w.fileHandles[fullPath] = file
	return file, nil
}

// closeOldestFile closes one file handle to free up resources
func (w *Writer) closeOldestFile() {
	// Simple strategy: close the first file we find
	// In a more sophisticated implementation, you might track access times
	for path, file := range w.fileHandles {
		file.Close()
		delete(w.fileHandles, path)
		break
	}
}

// Close closes all file handles and resources
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for path, file := range w.fileHandles {
		if err := file.Close(); err != nil {
			lastErr = err
		}
		delete(w.fileHandles, path)
	}

	return lastErr
}

// GetProgress returns the current file writing progress
func (w *Writer) GetProgress() *Progress {
	return w.progress
}

// VerifyFiles verifies that all files are complete and correct
func (w *Writer) VerifyFiles() ([]FileInfo, error) {
	files := w.mapper.GetAllFiles()

	for i, file := range files {
		fullPath := filepath.Join(w.outputDir, file.Path)

		// Check if file exists
		stat, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("file %s not found: %w", fullPath, err)
		}

		// Check file size
		if stat.Size() != file.Length {
			return nil, fmt.Errorf("file %s has incorrect size: expected %d, got %d",
				fullPath, file.Length, stat.Size())
		}

		// Update progress
		w.progress.SetFileComplete(i, true)
	}

	return files, nil
}

// GetOutputDirectory returns the output directory path
func (w *Writer) GetOutputDirectory() string {
	return w.outputDir
}

// IsComplete returns true if all files are completely written
func (w *Writer) IsComplete() bool {
	return w.progress.IsComplete()
}

// GetCompletedFiles returns a list of completed file paths
func (w *Writer) GetCompletedFiles() []string {
	var completed []string
	files := w.mapper.GetAllFiles()

	for i, file := range files {
		if w.progress.IsFileComplete(i) {
			completed = append(completed, filepath.Join(w.outputDir, file.Path))
		}
	}

	return completed
}

// FlushAll forces all pending writes to disk
func (w *Writer) FlushAll() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var lastErr error
	for _, file := range w.fileHandles {
		if err := file.Sync(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
