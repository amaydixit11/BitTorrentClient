package file

import (
	"fmt"
	"sync"
	"time"
)

// FileProgress tracks progress for a single file
type FileProgress struct {
	FileIndex    int       // Index in the torrent's file list
	FilePath     string    // Path to the file
	TotalBytes   int64     // Total file size
	WrittenBytes int64     // Bytes written so far
	IsComplete   bool      // Whether file is complete
	LastUpdate   time.Time // Last time this file was updated
}

// Progress tracks overall download progress
type Progress struct {
	mu           sync.RWMutex
	files        []FileProgress // Progress for each file
	totalBytes   int64          // Total torrent size
	writtenBytes int64          // Total bytes written
	startTime    time.Time      // When download started
}

// NewProgress creates a new progress tracker
func NewProgress(files []FileInfo) *Progress {
	fileProgress := make([]FileProgress, len(files))
	totalBytes := int64(0)

	for i, file := range files {
		fileProgress[i] = FileProgress{
			FileIndex:    i,
			FilePath:     file.Path,
			TotalBytes:   file.Length,
			WrittenBytes: 0,
			IsComplete:   false,
			LastUpdate:   time.Now(),
		}
		totalBytes += file.Length
	}

	return &Progress{
		files:        fileProgress,
		totalBytes:   totalBytes,
		writtenBytes: 0,
		startTime:    time.Now(),
	}
}

// AddWrittenBytes adds bytes written to a specific file
func (p *Progress) AddWrittenBytes(fileIndex int, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if fileIndex < 0 || fileIndex >= len(p.files) {
		return
	}

	// Update file progress
	p.files[fileIndex].WrittenBytes += bytes
	p.files[fileIndex].LastUpdate = time.Now()

	// Check if file is complete
	if p.files[fileIndex].WrittenBytes >= p.files[fileIndex].TotalBytes {
		p.files[fileIndex].IsComplete = true
	}

	// Update total progress
	p.writtenBytes += bytes
}

// SetFileComplete marks a file as complete
func (p *Progress) SetFileComplete(fileIndex int, complete bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if fileIndex < 0 || fileIndex >= len(p.files) {
		return
	}

	p.files[fileIndex].IsComplete = complete
	p.files[fileIndex].LastUpdate = time.Now()

	if complete {
		// Ensure written bytes matches total bytes
		if p.files[fileIndex].WrittenBytes < p.files[fileIndex].TotalBytes {
			diff := p.files[fileIndex].TotalBytes - p.files[fileIndex].WrittenBytes
			p.files[fileIndex].WrittenBytes = p.files[fileIndex].TotalBytes
			p.writtenBytes += diff
		}
	}
}

// GetFileProgress returns progress for a specific file
func (p *Progress) GetFileProgress(fileIndex int) (FileProgress, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if fileIndex < 0 || fileIndex >= len(p.files) {
		return FileProgress{}, false
	}

	return p.files[fileIndex], true
}

// GetAllFileProgress returns progress for all files
func (p *Progress) GetAllFileProgress() []FileProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a copy to avoid race conditions
	progress := make([]FileProgress, len(p.files))
	copy(progress, p.files)
	return progress
}

// IsFileComplete checks if a specific file is complete
func (p *Progress) IsFileComplete(fileIndex int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if fileIndex < 0 || fileIndex >= len(p.files) {
		return false
	}

	return p.files[fileIndex].IsComplete
}

// IsComplete returns true if all files are complete
func (p *Progress) IsComplete() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, file := range p.files {
		if !file.IsComplete {
			return false
		}
	}

	return true
}

// GetOverallProgress returns overall download progress (0.0 to 1.0)
func (p *Progress) GetOverallProgress() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.totalBytes == 0 {
		return 0.0
	}

	return float64(p.writtenBytes) / float64(p.totalBytes)
}

// GetOverallProgressPercent returns overall progress as percentage (0-100)
func (p *Progress) GetOverallProgressPercent() float64 {
	return p.GetOverallProgress() * 100.0
}

// GetFileProgressPercent returns progress percentage for a specific file
func (p *Progress) GetFileProgressPercent(fileIndex int) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if fileIndex < 0 || fileIndex >= len(p.files) {
		return 0.0
	}

	file := p.files[fileIndex]
	if file.TotalBytes == 0 {
		return 100.0
	}

	return (float64(file.WrittenBytes) / float64(file.TotalBytes)) * 100.0
}

// GetTotalBytes returns total torrent size
func (p *Progress) GetTotalBytes() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.totalBytes
}

// GetWrittenBytes returns total bytes written
func (p *Progress) GetWrittenBytes() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.writtenBytes
}

// GetRemainingBytes returns bytes remaining to download
func (p *Progress) GetRemainingBytes() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.totalBytes - p.writtenBytes
}

// GetDownloadSpeed returns current download speed in bytes/second
func (p *Progress) GetDownloadSpeed() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	elapsed := time.Since(p.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(p.writtenBytes) / elapsed
}

// GetETA returns estimated time to completion
func (p *Progress) GetETA() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	remaining := p.totalBytes - p.writtenBytes
	if remaining <= 0 {
		return 0
	}

	speed := p.GetDownloadSpeed()
	if speed <= 0 {
		return time.Duration(0) // Cannot estimate
	}

	return time.Duration(float64(remaining)/speed) * time.Second
}

// GetCompletedFiles returns number of completed files
func (p *Progress) GetCompletedFiles() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	completed := 0
	for _, file := range p.files {
		if file.IsComplete {
			completed++
		}
	}

	return completed
}

// GetTotalFiles returns total number of files
func (p *Progress) GetTotalFiles() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.files)
}

// GetProgressSummary returns a formatted progress summary
func (p *Progress) GetProgressSummary() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	completedFiles := p.GetCompletedFiles()
	totalFiles := len(p.files)
	percent := p.GetOverallProgressPercent()
	speed := p.GetDownloadSpeed()
	eta := p.GetETA()

	return fmt.Sprintf("Progress: %.1f%% (%d/%d files) | Speed: %.2f KB/s | ETA: %v",
		percent, completedFiles, totalFiles, speed/1024, eta.Truncate(time.Second))
}

// Reset resets all progress tracking
func (p *Progress) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.writtenBytes = 0
	p.startTime = time.Now()

	for i := range p.files {
		p.files[i].WrittenBytes = 0
		p.files[i].IsComplete = false
		p.files[i].LastUpdate = time.Now()
	}
}

// GetRecentlyUpdatedFiles returns files updated within the last duration
func (p *Progress) GetRecentlyUpdatedFiles(within time.Duration) []FileProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var recent []FileProgress
	cutoff := time.Now().Add(-within)

	for _, file := range p.files {
		if file.LastUpdate.After(cutoff) {
			recent = append(recent, file)
		}
	}

	return recent
}

// GetSlowFiles returns files that haven't been updated recently
func (p *Progress) GetSlowFiles(threshold time.Duration) []FileProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var slow []FileProgress
	cutoff := time.Now().Add(-threshold)

	for _, file := range p.files {
		if !file.IsComplete && file.LastUpdate.Before(cutoff) {
			slow = append(slow, file)
		}
	}

	return slow
}
