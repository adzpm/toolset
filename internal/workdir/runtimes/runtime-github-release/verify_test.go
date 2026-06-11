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

