package runtimegh

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// computeSHA256 returns the lowercase hex-encoded SHA256 digest of the file at filePath.
func computeSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
