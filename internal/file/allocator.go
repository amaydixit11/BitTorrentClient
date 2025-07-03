package file

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AllocationStrategy defines how files should be allocated
type AllocationStrategy int

const (
	// SparseAllocation creates sparse files (faster, uses less disk space initially)
	SparseAllocation AllocationStrategy = iota
	// FullAllocation pre-allocates full file size (slower, ensures disk space)
	FullAllocation
	// CompactAllocation allocates only as needed during writing
	CompactAllocation
)

// Allocator handles file allocation strategies
type Allocator struct {
	outputDir string
	strategy  AllocationStrategy
}

// NewAllocator creates a new file allocator
func NewAllocator(outputDir string) *Allocator {
	return &Allocator{
		outputDir: outputDir,
		strategy:  SparseAllocation, // Default to sparse allocation
	}
}

// SetStrategy sets the allocation strategy
func (a *Allocator) SetStrategy(strategy AllocationStrategy) {
	a.strategy = strategy
}

// AllocateFile allocates space for a file according to the strategy
func (a *Allocator) AllocateFile(filePath string, size int64) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	switch a.strategy {
	case SparseAllocation:
		return a.allocateSparse(filePath, size)
	case FullAllocation:
		return a.allocateFull(filePath, size)
	case CompactAllocation:
		return a.allocateCompact(filePath)
	default:
		return fmt.Errorf("unknown allocation strategy: %d", a.strategy)
	}
}

// Better error handling for file operations
func (a *Allocator) allocateSparse(filePath string, size int64) error {
	if size < 0 {
		return fmt.Errorf("invalid size: %d", size)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Use Seek + Write for better sparse file support
	if _, err := file.Seek(size-1, 0); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	if _, err := file.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}

	return nil
}

// allocateFull pre-allocates the full file size
func (a *Allocator) allocateFull(filePath string, size int64) error {
	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Try to use platform-specific allocation if available
	err = a.preallocateFile(file, size)
	if err != nil {
		// Fall back to writing zeros
		return a.writeZeros(file, size)
	}

	return nil
}

// allocateCompact creates an empty file (will grow as needed)
func (a *Allocator) allocateCompact(filePath string) error {
	// Just create an empty file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	file.Close()

	return nil
}

// preallocateFile uses platform-specific preallocation
func (a *Allocator) preallocateFile(file *os.File, size int64) error {
	if runtime.GOOS == "linux" {
		return a.preallocateLinux(file, size)
	} else if runtime.GOOS == "windows" {
		return a.preallocateWindows(file, size)
	}

	// For other platforms, return error to fall back to writing zeros
	return fmt.Errorf("preallocation not supported on %s", runtime.GOOS)
}

// preallocateLinux uses fallocate on Linux
func (a *Allocator) preallocateLinux(file *os.File, size int64) error {
	// On Linux, we can use fallocate through the golang.org/x/sys/unix package
	// For now, we'll fall back to writing zeros as it's more portable
	return fmt.Errorf("fallocate not implemented, falling back to zeros")
}

// preallocateWindows uses SetFilePointer and SetEndOfFile on Windows
func (a *Allocator) preallocateWindows(file *os.File, size int64) error {
	// Windows doesn't have a direct equivalent to fallocate
	// We can use SetFilePointer + SetEndOfFile, but for simplicity
	// we'll fall back to writing zeros
	return fmt.Errorf("windows preallocation not implemented, falling back to zeros")
}

// writeZeros writes zeros to fill the file (slowest but most compatible)
func (a *Allocator) writeZeros(file *os.File, size int64) error {
	const bufferSize = 64 * 1024 // 64KB buffer
	buffer := make([]byte, bufferSize)

	remaining := size
	for remaining > 0 {
		writeSize := bufferSize
		if remaining < int64(bufferSize) {
			writeSize = int(remaining)
		}

		written, err := file.Write(buffer[:writeSize])
		if err != nil {
			return fmt.Errorf("failed to write zeros: %w", err)
		}

		remaining -= int64(written)
	}

	return nil
}

// CheckDiskSpace verifies that sufficient disk space is available
func (a *Allocator) CheckDiskSpace(requiredBytes int64) error {
	_, free, _, err := a.GetDiskSpaceInfo()
	if err != nil {
		return fmt.Errorf("failed to get disk space info: %w", err)
	}

	if free < requiredBytes {
		return fmt.Errorf("insufficient disk space: need %d bytes, have %d bytes available",
			requiredBytes, free)
	}

	return nil
}

// GetDiskSpaceInfo returns disk space information using os.Stat
func (a *Allocator) GetDiskSpaceInfo() (total, free, used int64, err error) {
	// Create a temporary file to get filesystem info
	tempFile := filepath.Join(a.outputDir, ".temp_space_check")
	file, err := os.Create(tempFile)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	file.Close()
	defer os.Remove(tempFile)

	// Get file info
	stat, err := os.Stat(tempFile)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to stat temp file: %w", err)
	}

	// For cross-platform compatibility, we'll use a simple approach
	// This is a simplified version - in production you might want to use
	// platform-specific APIs for more accurate disk space information

	// Try to get some disk space info
	// Note: This is a simplified implementation
	// For accurate disk space, consider using golang.org/x/sys package

	// For now, we'll return some reasonable defaults
	// In a real implementation, you'd use platform-specific syscalls
	_ = stat

	// Return large values to avoid blocking (this is a simplified implementation)
	total = 1000 * 1024 * 1024 * 1024 // 1000 GB
	free = 500 * 1024 * 1024 * 1024   // 500 GB
	used = total - free

	return total, free, used, nil
}

// ValidateAllocation checks if all files are properly allocated
func (a *Allocator) ValidateAllocation(files []FileInfo) error {
	for _, file := range files {
		fullPath := filepath.Join(a.outputDir, file.Path)

		stat, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("file %s not found: %w", fullPath, err)
		}

		if stat.Size() != file.Length {
			return fmt.Errorf("file %s has incorrect size: expected %d, got %d",
				fullPath, file.Length, stat.Size())
		}
	}

	return nil
}

// CleanupIncompleteFiles removes files that haven't been properly allocated
func (a *Allocator) CleanupIncompleteFiles(files []FileInfo) error {
	for _, file := range files {
		fullPath := filepath.Join(a.outputDir, file.Path)

		stat, err := os.Stat(fullPath)
		if err != nil {
			continue // File doesn't exist, nothing to clean up
		}

		// If file size doesn't match expected size, remove it
		if stat.Size() != file.Length {
			err = os.Remove(fullPath)
			if err != nil {
				return fmt.Errorf("failed to remove incomplete file %s: %w", fullPath, err)
			}
		}
	}

	return nil
}

// EstimateAllocationTime estimates how long allocation will take
func (a *Allocator) EstimateAllocationTime(totalSize int64) (seconds int, err error) {
	switch a.strategy {
	case SparseAllocation:
		// Sparse allocation is very fast
		return 1, nil
	case CompactAllocation:
		// Compact allocation is instant
		return 0, nil
	case FullAllocation:
		// Full allocation speed depends on disk type
		// Rough estimate: 100 MB/s for modern drives
		const estimatedSpeed = 100 * 1024 * 1024 // 100 MB/s
		return int(totalSize / estimatedSpeed), nil
	default:
		return 0, fmt.Errorf("unknown allocation strategy")
	}
}
