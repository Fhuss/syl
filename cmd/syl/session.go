package main

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// fileSessionLabel keeps explicitly selected transcripts separate even when
// they share a basename. Resolving an existing symlink makes equivalent paths
// to the same transcript use the same snapshot.
func fileSessionLabel(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return fmt.Sprintf("file-%x", sum[:8]), nil
}
