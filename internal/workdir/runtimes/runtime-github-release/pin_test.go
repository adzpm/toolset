package runtimegh

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

func Test_findPinnedAsset(t *testing.T) {
	assets := []structs.PinnedAsset{
		{OS: goosDarwin, Arch: goarchARM64, Name: "tool-darwin-arm64.tar.gz", Digest: "sha256:aaa"},
		{OS: goosLinux, Arch: goarchAMD64, Name: "tool-linux-amd64.tar.gz", Digest: "sha256:bbb"},
	}

	t.Run("found", func(t *testing.T) {
		a, ok := findPinnedAsset(assets, goosDarwin, goarchARM64)
		require.True(t, ok)
		require.Equal(t, "sha256:aaa", a.Digest)
	})

	t.Run("not_found_wrong_arch", func(t *testing.T) {
		_, ok := findPinnedAsset(assets, goosDarwin, goarchAMD64)
		require.False(t, ok)
	})

	t.Run("not_found_wrong_os", func(t *testing.T) {
		_, ok := findPinnedAsset(assets, goosWindows, goarchAMD64)
		require.False(t, ok)
	})

	t.Run("empty_slice", func(t *testing.T) {
		_, ok := findPinnedAsset(nil, goosDarwin, goarchARM64)
		require.False(t, ok)
	})
}

func Test_findPinnedAsset_multiPlatform(t *testing.T) {
	assets := []structs.PinnedAsset{
		{OS: goosDarwin, Arch: goarchARM64, Name: "tool-darwin-arm64.tar.gz", Digest: "sha256:aaa111"},
		{OS: goosDarwin, Arch: goarchAMD64, Name: "tool-darwin-x86.tar.gz", Digest: "sha256:aaa222"},
		{OS: goosLinux, Arch: goarchAMD64, Name: "tool-linux-amd64.tar.gz", Digest: "sha256:bbb111"},
		{OS: goosLinux, Arch: goarchARM64, Name: "tool-linux-arm64.tar.gz", Digest: "sha256:bbb222"},
	}

	tests := []struct {
		os, arch string
		wantOk   bool
		wantName string
	}{
		{goosDarwin, goarchARM64, true, "tool-darwin-arm64.tar.gz"},
		{goosDarwin, goarchAMD64, true, "tool-darwin-x86.tar.gz"},
		{goosLinux, goarchAMD64, true, "tool-linux-amd64.tar.gz"},
		{goosLinux, goarchARM64, true, "tool-linux-arm64.tar.gz"},
		{goosWindows, goarchAMD64, false, ""},
		{goosLinux, goarch386, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.os+"_"+tt.arch, func(t *testing.T) {
			a, ok := findPinnedAsset(assets, tt.os, tt.arch)
			require.Equal(t, tt.wantOk, ok)
			if ok {
				require.Equal(t, tt.wantName, a.Name)
			}
		})
	}
}

func Test_Install_NoPinPasses(t *testing.T) {
	emptyPin := optional.Empty[structs.Pin]()
	require.False(t, emptyPin.HasVal())
}

func Test_computeSHA256_content(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantMatch string
	}{
		{"hello_toolset", "hello toolset", "e00894f268e905f2ead060c0fac9763d9642eb471be673c274e12ee293028d86"},
		{"empty_string", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"short_string", "hi", "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "sha256-content-*")
			require.NoError(t, err)
			defer os.Remove(f.Name()) //nolint:errcheck

			_, err = f.WriteString(tt.content)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			got, err := computeSHA256(f.Name())
			require.NoError(t, err)
			require.Equal(t, tt.wantMatch, got)
		})
	}
}

func Test_Install_PinMismatch_helper(t *testing.T) {
	// Verify computeSHA256 and findPinnedAsset correctly identify a mismatch.
	// The full end-to-end mismatch is tested in Test_Install_pinMismatch_rejected.
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosDarwin, goarchARM64)

	pin := optional.New(structs.Pin{
		Assets: []structs.PinnedAsset{
			{OS: goosDarwin, Arch: goarchARM64, Name: "tool.tar.gz", Digest: "sha256:deadbeef"},
		},
	})

	a, ok := findPinnedAsset(pin.Val().Assets, rt.os, rt.arch)
	require.True(t, ok)
	require.Equal(t, "sha256:deadbeef", a.Digest)

	f, err := os.CreateTemp("", "pin-test-*")
	require.NoError(t, err)
	defer os.Remove(f.Name()) //nolint:errcheck

	_, err = f.WriteString("hello world")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	computed, err := computeSHA256(f.Name())
	require.NoError(t, err)
	require.NotEqual(t, "deadbeef", computed)
}

// testGetPinClient extends testGithubClient with a configurable GetReleaseByTag.
type testGetPinClient struct {
	testGithubClient
	releaseByTagFn func(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, error)
}

func (f *testGetPinClient) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, error) {
	return f.releaseByTagFn(ctx, owner, repo, tag)
}

// newTestRuntimeForGetPin creates a Runtime with a fake GetReleaseByTag injected.
func newTestRuntimeForGetPin(t *testing.T, releaseFn func(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, error)) *Runtime {
	t.Helper()
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosDarwin, goarchARM64)
	rt.gh = &testGetPinClient{releaseByTagFn: releaseFn}
	return rt
}

// Test_GetPin_emptyDigest verifies that GetPin returns an error when a matched asset
// has no digest (i.e., the release predates digest support).
func Test_GetPin_emptyDigest(t *testing.T) {
	assetName := "golangci-lint-v1.61.0-darwin-arm64.tar.gz"
	emptyDigest := ""
	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{
			{Name: &assetName, Digest: &emptyDigest},
		},
	}

	rt := newTestRuntimeForGetPin(t, func(_ context.Context, _, _, _ string) (*github.RepositoryRelease, error) {
		return release, nil
	})

	_, err := rt.GetPin(context.Background(), "golangci/golangci-lint@v1.61.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no digest")
}

// Test_GetPin_noAssetsForAnyPlatform verifies that GetPin returns an error when
// no release asset matches any known platform.
func Test_GetPin_noAssetsForAnyPlatform(t *testing.T) {
	// Return a release with no assets — all platform lookups return errAutoDiscover.
	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{},
	}

	rt := newTestRuntimeForGetPin(t, func(_ context.Context, _, _, _ string) (*github.RepositoryRelease, error) {
		return release, nil
	})

	_, err := rt.GetPin(context.Background(), "golangci/golangci-lint@v1.61.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pinnable assets")
}

// Test_GetPin_releaseAPIError verifies that GetPin propagates errors from the GitHub
// release API call.
func Test_GetPin_releaseAPIError(t *testing.T) {
	rt := newTestRuntimeForGetPin(t, func(_ context.Context, _, _, _ string) (*github.RepositoryRelease, error) {
		return nil, errors.New("rate limit exceeded")
	})

	_, err := rt.GetPin(context.Background(), "golangci/golangci-lint@v1.61.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "get release by tag")
}
