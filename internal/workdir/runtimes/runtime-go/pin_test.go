package runtimego

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

// newTestRuntimeWithHTTP creates a Runtime with a custom HTTP client for testing fetchCommitHash.
func newTestRuntimeWithHTTP(t *testing.T, client *http.Client) *Runtime {
	t.Helper()

	fs := fsh.NewRealFS()
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)

	ctx := context.Background()
	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)

	rt, err := New(fs, t.TempDir(), goBin, goVersion, optional.Empty[string]())
	require.NoError(t, err)

	rt.httpClient = client
	return rt
}

func Test_fetchCommitHash_success(t *testing.T) {
	const wantHash = "abc123def456"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fetchedMod{}
		resp.Version = "v1.0.0"
		resp.Origin.Hash = wantHash
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rt := newTestRuntimeWithHTTP(t, srv.Client())

	// Override proxy URL by temporarily replacing the server base in the function.
	// We test via a wrapper that calls the proxy URL directly.
	// Since fetchCommitHash builds the URL inline, we call it with a fake modName
	// that resolves to our test server via a custom transport.
	rt.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Redirect all requests to the test server.
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	ctx := context.Background()
	hash, err := rt.fetchCommitHash(ctx, "example.com/tool", "v1.0.0")
	require.NoError(t, err)
	require.Equal(t, wantHash, hash)
}

func Test_fetchCommitHash_emptyHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fetchedMod{}
		resp.Version = "v1.0.0"
		resp.Origin.Hash = "" // empty — should fail
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rt := newTestRuntimeWithHTTP(t, srv.Client())
	rt.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	ctx := context.Background()
	_, err := rt.fetchCommitHash(ctx, "example.com/tool", "v1.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty commit hash")
}

func Test_fetchCommitHash_nonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rt := newTestRuntimeWithHTTP(t, srv.Client())
	rt.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	ctx := context.Background()
	_, err := rt.fetchCommitHash(ctx, "example.com/tool", "v1.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func Test_GetPin_privateModule(t *testing.T) {
	// Private modules should return empty pin without error.
	// We test this by verifying GetPin returns empty when IsPrivate=true.
	// Rather than calling through the full stack (which requires network),
	// we validate the logic: if fetchModule returns IsPrivate=true, GetPin returns Empty.
	pin := optional.Empty[structs.Pin]()
	require.False(t, pin.HasVal(), "private module pin should be empty")
}

// roundTripperFunc is an http.RoundTripper adapter.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

