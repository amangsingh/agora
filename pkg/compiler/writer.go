package compiler

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// SafeWriteFile writes content to a file at path, ensuring the directory exists
// and the code is formatted if it's a Go file.
// It uses filepath.Clean to prevent path traversal.
func SafeWriteFile(baseDir, relPath string, content []byte) error {
	// 1. Sanitize path
	cleanPath := filepath.Clean(relPath)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path contains traversal characters: %s", relPath)
	}

	fullPath := filepath.Join(baseDir, cleanPath)

	// 2. Format Go code
	if strings.HasSuffix(fullPath, ".go") {
		formatted, err := format.Source(content)
		if err == nil {
			content = formatted
		} else {
			// If formatting fails, we still write the file but log a warning (or error).
			// For a compiler, maybe we want to fail strict?
			// Let's print a warning but write raw content so user can debug.
			fmt.Printf("Warning: failed to format %s: %v\n", relPath, err)
		}
	}

	// 3. Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// 4. Write file
	return os.WriteFile(fullPath, content, 0644)
}
