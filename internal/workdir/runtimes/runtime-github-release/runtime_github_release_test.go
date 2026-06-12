package runtimegh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

// fakeContent is a known byte slice used to compute predictable SHA256 digests in tests.
const fakeContent = "fake release archive content for test"

// hexSHA256 returns the lowercase hex SHA256 of s.
func hexSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// testGithubClient implements githubClient for tests.
var _ githubClient = (*testGithubClient)(nil)

type testGithubClient struct {
	assetName string
	content   string
	downloadErr error
}

func (f *testGithubClient) GetAsset(_ context.Context, _, _, _ string) (*github.ReleaseAsset, error) {
	id := int64(1)
	return &github.ReleaseAsset{ID: &id, Name: &f.assetName}, nil
}

func (f *testGithubClient) DownloadAsset(_ context.Context, _, _ string, _ int64, targetFile string) error {
	if f.downloadErr != nil {
		return f.downloadErr
	}
	return os.WriteFile(targetFile, []byte(f.content), 0o644)
}

func (f *testGithubClient) GetLatestRelease(_ context.Context, _, _ string) (*github.RepositoryRelease, error) {
	return nil, errors.New("not implemented")
}

func (f *testGithubClient) GetReleaseByTag(_ context.Context, _, _, _ string) (*github.RepositoryRelease, error) {
	return nil, errors.New("not implemented")
}

// newTestRuntime creates a Runtime with a fake GitHub client injected.
// The fake returns a single asset named assetName and writes content on download.
func newTestRuntime(t *testing.T, assetName, content string) *Runtime {
	t.Helper()
	rt := New(fsh.NewRealFS(), t.TempDir(), nil, goosDarwin, goarchARM64)
	rt.gh = &testGithubClient{assetName: assetName, content: content}
	return rt
}

// Test_Install_SHA256Verification_doesNotRejectMatchingDigest verifies that Install()
// does not produce a SHA256 mismatch error when the digest matches the downloaded file.
// The install may still fail at the archive extraction step (fakeContent is not a real
// archive), but that is unrelated to supply-chain verification.
func Test_Install_SHA256Verification_doesNotRejectMatchingDigest(t *testing.T) {
	digest := "sha256:" + hexSHA256(fakeContent)

	rt := newTestRuntime(t, "tool.tar.gz", fakeContent)
	pin := optional.New(structs.Pin{
		Assets: []structs.PinnedAsset{
			{OS: goosDarwin, Arch: goarchARM64, Name: "tool.tar.gz", Digest: digest},
		},
	})

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", pin)

	// SHA256 verification passed; any remaining error must not be from the PIN check.
	if err != nil {
		require.NotContains(t, err.Error(), "SHA256 mismatch",
			"expected PIN to be accepted but got SHA256 mismatch")
	}
}

// Test_Install_pinMismatch_rejected verifies that Install() rejects installation when the
// downloaded archive's SHA256 digest differs from the pinned value (tamper-detection).
func Test_Install_pinMismatch_rejected(t *testing.T) {
	wrongDigest := "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	rt := newTestRuntime(t, "tool.tar.gz", fakeContent)
	pin := optional.New(structs.Pin{
		Assets: []structs.PinnedAsset{
			{OS: goosDarwin, Arch: goarchARM64, Name: "tool.tar.gz", Digest: wrongDigest},
		},
	})

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", pin)

	require.Error(t, err)
	require.Contains(t, err.Error(), "SHA256 mismatch",
		"expected SHA256 mismatch rejection but got: %v", err)
}

// Test_Install_pinMissingPlatform verifies Install() fails when the PIN has no entry
// for the current platform.
func Test_Install_pinMissingPlatform(t *testing.T) {
	rt := newTestRuntime(t, "tool.tar.gz", fakeContent)
	pin := optional.New(structs.Pin{
		Assets: []structs.PinnedAsset{
			{OS: goosLinux, Arch: goarchAMD64, Name: "tool-linux.tar.gz", Digest: "sha256:aaa"},
		},
	})

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", pin)

	require.Error(t, err)
	require.Contains(t, err.Error(), "pin has no entry for platform darwin/arm64")
}

// Test_Install_noPin verifies that Install() without a PIN skips verification entirely.
func Test_Install_noPin(t *testing.T) {
	rt := newTestRuntime(t, "tool.tar.gz", fakeContent)
	pin := optional.Empty[structs.Pin]()

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", pin)

	// May fail at extraction step (not a real archive), but NOT at SHA256 verification.
	if err != nil {
		require.NotContains(t, err.Error(), "SHA256 mismatch")
		require.NotContains(t, err.Error(), "pin has no entry")
	}
}

func Test_Install_platformCheck(t *testing.T) {
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosLinux, goarchAMD64)
	require.Equal(t, goosLinux, rt.os)
	require.Equal(t, goarchAMD64, rt.arch)
}

func Test_GetModule_pathConstruction(t *testing.T) {
	const binToolDir = "/tools"
	rt := New(fsh.NewMemFS(nil), binToolDir, nil, goosDarwin, goarchARM64)

	mod, err := rt.GetModule(context.Background(), "golangci/golangci-lint@v1.61.0")
	require.NoError(t, err)
	require.Contains(t, mod.BinDir, "golangci")
	require.Equal(t, "golangci-lint", mod.Name)
}

func Test_findBinary_directRoot(t *testing.T) {
	ctx := context.Background()
	memFS := fsh.NewMemFS(map[string]string{
		"/extracted/mytool": "binary content",
	})
	rt := New(memFS, "/tmp/tools", nil, goosDarwin, goarchARM64)

	binFile, err := rt.findBinary("/extracted", "mytool")
	require.NoError(t, err)
	require.Equal(t, filepath.FromSlash("/extracted/mytool"), binFile)
	_ = ctx
}

func Test_findBinary_inSubdirectory(t *testing.T) {
	memFS := fsh.NewMemFS(map[string]string{
		"/extracted/mytool-v1.0/mytool": "binary content",
	})
	rt := New(memFS, "/tmp/tools", nil, goosDarwin, goarchARM64)

	binFile, err := rt.findBinary("/extracted", "mytool")
	require.NoError(t, err)
	require.Equal(t, filepath.FromSlash("/extracted/mytool-v1.0/mytool"), binFile)
}

func Test_findBinary_inBinDir(t *testing.T) {
	memFS := fsh.NewMemFS(map[string]string{
		"/extracted/mytool-v1.0/bin/mytool": "binary content",
	})
	rt := New(memFS, "/tmp/tools", nil, goosDarwin, goarchARM64)

	binFile, err := rt.findBinary("/extracted", "mytool")
	require.NoError(t, err)
	require.Equal(t, filepath.FromSlash("/extracted/mytool-v1.0/bin/mytool"), binFile)
}

func Test_findBinary_notFound(t *testing.T) {
	memFS := fsh.NewMemFS(map[string]string{
		"/extracted/some_other_file": "content",
	})
	rt := New(memFS, "/tmp/tools", nil, goosDarwin, goarchARM64)

	_, err := rt.findBinary("/extracted", "mytool")
	require.Error(t, err)
}

func Test_Version_returnsGh(t *testing.T) {
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosDarwin, goarchARM64)
	require.Equal(t, "gh", rt.Version())
}

func Test_Parse_validModuleString(t *testing.T) {
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosDarwin, goarchARM64)

	result, err := rt.Parse(context.Background(), "golangci/golangci-lint@v1.61.0")
	require.NoError(t, err)
	require.Equal(t, "golangci/golangci-lint@v1.61.0", result)
}

func Test_Parse_invalidModuleString(t *testing.T) {
	rt := New(fsh.NewMemFS(nil), "/tmp/tools", nil, goosDarwin, goarchARM64)

	result, err := rt.Parse(context.Background(), "invalid-module")
	require.Error(t, err)
	require.Empty(t, result)
}

// Test_Install_downloadError verifies that Install propagates errors from the download step.
func Test_Install_downloadError(t *testing.T) {
	rt := New(fsh.NewRealFS(), t.TempDir(), nil, goosDarwin, goarchARM64)
	rt.gh = &testGithubClient{
		assetName:   "tool.tar.gz",
		downloadErr: errors.New("network error: connection refused"),
	}

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", optional.Empty[structs.Pin]())

	require.Error(t, err)
	require.Contains(t, err.Error(), "download asset")
}

// Test_Install_corruptArchive verifies that Install returns an extraction error when the
// downloaded file is not a valid archive.
func Test_Install_corruptArchive(t *testing.T) {
	rt := newTestRuntime(t, "tool.tar.gz", "this is not a valid archive")
	pin := optional.Empty[structs.Pin]()

	err := rt.Install(context.Background(), "golangci/golangci-lint@v1.61.0", pin)

	require.Error(t, err)
	require.Contains(t, err.Error(), "extract release file")
}
