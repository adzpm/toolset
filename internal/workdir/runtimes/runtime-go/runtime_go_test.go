package runtimego

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

// newTestRuntime creates a Runtime with a custom HTTP transport that routes all requests
// to the given test server. This allows mocking both fetchModule and fetchCommitHash.
func newTestRuntimeWithTransport(t *testing.T, srv *httptest.Server) *Runtime {
	t.Helper()

	ctx := context.Background()
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)

	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)

	rt, err := New(fsh.NewMemFS(nil), t.TempDir(), goBin, goVersion, optional.Empty[string]())
	require.NoError(t, err)

	rt.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	return rt
}

// Test_Install_SHA256Verification_doesNotRejectMatchingHash verifies that Install() does
// not produce a "commit hash mismatch" error when the proxy returns the pinned hash.
// After PIN verification passes, `go install` runs and may fail (module doesn't exist),
// but that is unrelated to supply-chain verification.
func Test_Install_SHA256Verification_doesNotRejectMatchingHash(t *testing.T) {
	const pinnedHash = "abc123def456abc123def456abc123def456abc1"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fetchedMod{Version: "v1.0.0"}
		resp.Origin.Hash = pinnedHash
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rt := newTestRuntimeWithTransport(t, srv)
	pin := optional.New(structs.Pin{CommitHash: pinnedHash})

	err := rt.Install(context.Background(), "github.com/example/tool@v1.0.0", pin)

	// PIN verification passed; error (if any) must come from `go install`, not the PIN check.
	if err != nil {
		require.NotContains(t, err.Error(), "commit hash mismatch",
			"expected PIN to be accepted but got hash mismatch")
	}
}

// Test_Install_pinMismatch_rejected verifies that Install() rejects installation when
// the proxy returns a hash different from the pinned one (tamper-detection).
func Test_Install_pinMismatch_rejected(t *testing.T) {
	const (
		pinnedHash  = "pinned_hash_111111111111111111111111111111"
		currentHash = "current_hash_99999999999999999999999999999999" // different
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fetchedMod{Version: "v1.0.0"}
		resp.Origin.Hash = currentHash
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rt := newTestRuntimeWithTransport(t, srv)
	pin := optional.New(structs.Pin{CommitHash: pinnedHash}) // pinned != current

	err := rt.Install(context.Background(), "github.com/example/tool@v1.0.0", pin)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "commit hash mismatch"),
		"expected 'commit hash mismatch' error, got: %v", err)
}

func Test_Version_returnsGo(t *testing.T) {
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)
	ctx := context.Background()
	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)

	rt, err := New(fsh.NewMemFS(nil), "/tmp", goBin, goVersion, optional.Empty[string]())
	require.NoError(t, err)

	version := rt.Version()
	require.NotEmpty(t, version)
	require.Contains(t, version, "go")
}

func Test_Parse_validModule(t *testing.T) {
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)
	ctx := context.Background()
	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)

	rt, err := New(fsh.NewMemFS(nil), "/tmp", goBin, goVersion, optional.Empty[string]())
	require.NoError(t, err)

	result, err := rt.Parse(context.Background(), "github.com/golang/tools@v0.1.12")
	require.NoError(t, err)
	require.NotEmpty(t, result)
}

func Test_Parse_invalidFormat(t *testing.T) {
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)
	ctx := context.Background()
	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)

	rt, err := New(fsh.NewMemFS(nil), "/tmp", goBin, goVersion, optional.Empty[string]())
	require.NoError(t, err)

	_, err = rt.Parse(context.Background(), "not-a-valid-module-string")
	require.Error(t, err)
}
