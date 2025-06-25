package torrent

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// File represents a file in a multi-file torrent
type File struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
	MD5Sum *string  `bencode:"md5sum,omitempty"`
}

// DisplayPath returns the file path as a string
func (f *File) DisplayPath() string {
	return strings.Join(f.Path, string(os.PathSeparator))
}

// ValidatePath checks if the file path is safe and valid
func (f *File) ValidatePath() error {
	if len(f.Path) == 0 {
		return errors.New("file path cannot be empty")
	}

	for i, component := range f.Path {
		if component == "" {
			return fmt.Errorf("empty path component at index %d", i)
		}
		if component == "." || component == ".." {
			return fmt.Errorf("invalid path component: %s", component)
		}
		// Check for invalid characters (platform-specific)
		if strings.ContainsAny(component, "<>:\"|?*") {
			return fmt.Errorf("invalid characters in path component: %s", component)
		}
	}

	return nil
}

// IsEmpty returns true if this is a zero-length file
func (f *File) IsEmpty() bool {
	return f.Length == 0
}
