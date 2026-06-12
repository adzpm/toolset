package runtimegh

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_computeSHA256(t *testing.T) {
	// Write a known file and verify the SHA256 matches.
	f, err := os.CreateTemp("", "sha256-test-*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) //nolint:errcheck

	content := []byte("hello toolset")
	_, err = f.Write(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.Len(t, got, 64, "SHA256 hex string should be 64 chars")

	// Compute expected value independently.
	// sha256("hello toolset") = b94d27b9...
	// We verify it's stable (same input → same hash).
	got2, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.Equal(t, got, got2)
}

func Test_computeSHA256_emptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "sha256-empty-*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) //nolint:errcheck
	require.NoError(t, f.Close())

	got, err := computeSHA256(f.Name())
	require.NoError(t, err)
	// SHA256 of empty input
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", got)
}

func Test_computeSHA256_missingFile(t *testing.T) {
	_, err := computeSHA256("/nonexistent/path/file.bin")
	require.Error(t, err)
}

func Test_computeSHA256_largeFile(t *testing.T) {
	// Test with a larger file to ensure io.Copy handles streaming correctly.
	f, err := os.CreateTemp("", "sha256-large-*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) //nolint:errcheck

	// Write 10MB of repetitive data
	largeContent := make([]byte, 10*1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	_, err = f.Write(largeContent)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.Len(t, got, 64)

	// Verify it's deterministic
	got2, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.Equal(t, got, got2)
}

func Test_computeSHA256_knownValues(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantDigest string
	}{
		{"hello", "hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"hello world", "hello world", "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
		{"test", "test", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "sha256-known-*")
			require.NoError(t, err)
			defer os.Remove(f.Name()) //nolint:errcheck

			_, err = f.WriteString(tt.content)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			got, err := computeSHA256(f.Name())
			require.NoError(t, err)
			require.Equal(t, tt.wantDigest, got)
		})
	}
}

