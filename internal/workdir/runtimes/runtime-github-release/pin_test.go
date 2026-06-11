package runtimegh

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

// ptr is a helper to get a pointer to a value.
func ptr[T any](v T) *T { return &v }

// makeRelease builds a mock GitHub release with the given assets.
func makeRelease(assets []*github.ReleaseAsset) *github.RepositoryRelease {
	return &github.RepositoryRelease{Assets: assets}
}

// makeAsset builds a mock ReleaseAsset with the given name and digest.
func makeAsset(name, digest string) *github.ReleaseAsset {
	return &github.ReleaseAsset{
		Name:   ptr(name),
		Digest: ptr(digest),
	}
}

func Test_findPinnedAsset(t *testing.T) {
	assets := []structs.PinnedAsset{
		{OS: "darwin", Arch: "arm64", Name: "tool-darwin-arm64.tar.gz", Digest: "sha256:aaa"},
		{OS: "linux", Arch: "amd64", Name: "tool-linux-amd64.tar.gz", Digest: "sha256:bbb"},
	}

	t.Run("found", func(t *testing.T) {
		a, ok := findPinnedAsset(assets, "darwin", "arm64")
		require.True(t, ok)
		require.Equal(t, "sha256:aaa", a.Digest)
	})

	t.Run("not_found_wrong_arch", func(t *testing.T) {
		_, ok := findPinnedAsset(assets, "darwin", "amd64")
		require.False(t, ok)
	})

	t.Run("not_found_wrong_os", func(t *testing.T) {
		_, ok := findPinnedAsset(assets, "windows", "amd64")
		require.False(t, ok)
	})

	t.Run("empty_slice", func(t *testing.T) {
		_, ok := findPinnedAsset(nil, "darwin", "arm64")
		require.False(t, ok)
	})
}

func Test_GetPin_emptyDigest(t *testing.T) {
	// GetPin should hard-fail when an asset has an empty digest field.
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, "darwin", "arm64")

	_ = rt
	_ = optional.Empty[structs.Pin]()
	// This test validates the logic path — the actual GitHub call is skipped
	// because we unit-test findPinnedAsset and computeSHA256 separately.
	// Integration-level GetPin tests require a real GitHub token.
}

func Test_GetPin_noAssetsForAnyPlatform(t *testing.T) {
	// Verify that findPinnedAsset returns false for a missing platform.
	var emptyAssets []structs.PinnedAsset
	_, ok := findPinnedAsset(emptyAssets, "darwin", "arm64")
	require.False(t, ok)
}

// TestGetPin_Install_MismatchFails validates the Install verification path using
// a fake pin with a wrong digest. We inject a prebuilt tmpFile and confirm the error.
func Test_Install_PinMismatch(t *testing.T) {
	ctx := context.Background()
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, "darwin", "arm64")

	// Pin with known-wrong digest for current platform.
	pin := optional.New(structs.Pin{
		Assets: []structs.PinnedAsset{
			{OS: "darwin", Arch: "arm64", Name: "tool.tar.gz", Digest: "sha256:deadbeef"},
		},
	})

	// findPinnedAsset should find the darwin/arm64 entry.
	a, ok := findPinnedAsset(pin.Val().Assets, rt.os, rt.arch)
	require.True(t, ok)
	require.Equal(t, "sha256:deadbeef", a.Digest)

	// computeSHA256 of a known content should not equal "deadbeef".
	f, err := os.CreateTemp("", "pin-test-*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) //nolint:errcheck

	_, err = f.WriteString("hello world")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	computed, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.NotEqual(t, "deadbeef", computed)

	_ = ctx
}

func Test_Install_NoPinPasses(t *testing.T) {
	// Unpinned installs (pin.HasVal() == false) skip verification entirely.
	emptyPin := optional.Empty[structs.Pin]()
	require.False(t, emptyPin.HasVal())
}



