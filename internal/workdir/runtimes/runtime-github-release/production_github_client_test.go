package runtimegh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// newTestProductionClient creates a productionGithubClient backed by a local httptest server.
func newTestProductionClient(t *testing.T, mux *http.ServeMux) *productionGithubClient {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ghClient := github.NewClient(nil)
	u, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	ghClient.BaseURL = u

	return &productionGithubClient{
		client: ghClient,
		fs:     fsh.NewMemFS(nil),
		goos:   goosDarwin,
		goarch: goarchARM64,
	}
}

func TestProductionGithubClient_GetLatestRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(&github.RepositoryRelease{TagName: github.Ptr("v1.2.3")})
	})

	c := newTestProductionClient(t, mux)
	rel, err := c.GetLatestRelease(context.Background(), "owner", "repo")
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", rel.GetTagName())
}

func TestProductionGithubClient_GetReleaseByTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/tags/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(&github.RepositoryRelease{TagName: github.Ptr("v1.0.0")})
	})

	c := newTestProductionClient(t, mux)
	rel, err := c.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0.0")
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", rel.GetTagName())
}

func TestProductionGithubClient_GetAsset(t *testing.T) {
	// Asset name must match autoDiscoverAsset for darwin/arm64.
	assetName := "repo-v1.0.0-darwin-arm64.tar.gz"
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/tags/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(&github.RepositoryRelease{
			TagName: github.Ptr("v1.0.0"),
			Assets:  []*github.ReleaseAsset{{Name: github.Ptr(assetName), ID: github.Ptr(int64(42))}},
		})
	})

	c := newTestProductionClient(t, mux)
	asset, err := c.GetAsset(context.Background(), "owner", "repo", "v1.0.0")
	require.NoError(t, err)
	require.Equal(t, assetName, asset.GetName())
}

func TestProductionGithubClient_DownloadAsset(t *testing.T) {
	content := []byte("binary content")
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/assets/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(content)
	})

	c := newTestProductionClient(t, mux)
	targetFile := "/out/tool"
	err := c.DownloadAsset(context.Background(), "owner", "repo", 42, targetFile)
	require.NoError(t, err)

	got, err := afero.ReadFile(c.fs, targetFile)
	require.NoError(t, err)
	require.Equal(t, content, got)
}
