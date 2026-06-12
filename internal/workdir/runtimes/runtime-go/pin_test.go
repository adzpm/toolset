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
	"github.com/kazhuravlev/toolset/internal/prog"
	"github.com/stretchr/testify/require"
)

// testModuleResolver is a fake moduleResolver for tests.
type testModuleResolver struct {
	result *moduleInfo
	err    error
}

var _ moduleResolver = (*testModuleResolver)(nil)

func (f *testModuleResolver) fetchModule(_ context.Context, _ string) (*moduleInfo, error) {
	return f.result, f.err
}


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

func Test_fetchCommitHash_jsonDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{ invalid json"))
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
	require.Contains(t, err.Error(), "decode proxy response")
}

func Test_fetchCommitHash_contextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Listen for context cancellation instead of blocking forever
		<-r.Context().Done()
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := rt.fetchCommitHash(ctx, "example.com/tool", "v1.0.0")
	require.Error(t, err)
}

func Test_fetchCommitHash_successViaMockedProxy(t *testing.T) {
	// Test the full GetPin() success path with a mocked public module.
	// Note: GetPin calls fetchModule (which needs go list) then fetchCommitHash.
	// We test fetchCommitHash here since that is the core of the pin hash retrieval.
	const (
		wantHash    = "abc123def456abc123def456abc123def456abc1"
		testModule  = "github.com/example/tool/cmd/mytool"
		testVersion = "v1.0.0"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fetchedMod{
			Version: testVersion,
		}
		resp.Origin.Hash = wantHash
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
	hash, err := rt.fetchCommitHash(ctx, testModule, testVersion)
	require.NoError(t, err)
	require.Equal(t, wantHash, hash)
}

func Test_fetchCommitHash_invalidModulePath(t *testing.T) {
	// Test error when module path can't be escaped for the proxy URL.
	rt := newTestRuntimeWithHTTP(t, http.DefaultClient)

	ctx := context.Background()
	// Use an invalid module path that will fail escaping
	_, err := rt.fetchCommitHash(ctx, "../../etc/passwd", "v1.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "escape module path")
}

func Test_GetPin_privateModule_returnsEmpty(t *testing.T) {
	// Verify that GetPin returns optional.Empty (no error) for private modules.
	rt := newTestRuntimeWithHTTP(t, http.DefaultClient)
	rt.resolver = &testModuleResolver{result: &moduleInfo{
		IsPrivate: true,
		Mod:       prog.NewVer("example.com/private/tool", "v1.0.0"),
		Program:   "tool",
	}}

	pin, err := rt.GetPin(context.Background(), "example.com/private/tool@v1.0.0")
	require.NoError(t, err)
	require.False(t, pin.HasVal(), "expected empty pin for private module")
}

// roundTripperFunc is an http.RoundTripper adapter.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

