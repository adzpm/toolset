package runtimego

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"golang.org/x/mod/module"
)

// moduleResolver abstracts fetchModule so tests can inject a fake without running
// the real go toolchain. *Runtime is the production implementation.
type moduleResolver interface {
	fetchModule(ctx context.Context, link string) (*moduleInfo, error)
}

var _ moduleResolver = (*Runtime)(nil)

// GetPin returns the git commit hash recorded by proxy.golang.org for the given program.
// Private modules cannot be queried via the public proxy — they return optional.Empty with
// a warning printed to stdout.
func (r *Runtime) GetPin(ctx context.Context, program string) (optional.Val[structs.Pin], error) {
	mod, err := r.resolver.fetchModule(ctx, program)
	if err != nil {
		return optional.Empty[structs.Pin](), fmt.Errorf("fetch module: %w", err)
	}

	if mod.IsPrivate {
		fmt.Printf("warning: %s is a private module — pinning not supported, skipping\n", program)
		return optional.Empty[structs.Pin](), nil
	}

	hash, err := r.fetchCommitHash(ctx, mod.ResolvedModPath, mod.Mod.Version())
	if err != nil {
		return optional.Empty[structs.Pin](), fmt.Errorf("fetch commit hash: %w", err)
	}

	return optional.New(structs.Pin{CommitHash: hash}), nil
}

// fetchCommitHash retrieves the git commit hash for modName@version from proxy.golang.org.
// modName must be the module root path (e.g. "github.com/golangci/golangci-lint/v2"),
// not a cmd/ sub-path.
func (r *Runtime) fetchCommitHash(ctx context.Context, modName, version string) (string, error) {
	escapedPath, err := module.EscapePath(modName)
	if err != nil {
		return "", fmt.Errorf("escape module path (%s): %w", modName, err)
	}

	url := fmt.Sprintf("https://proxy.golang.org/%s/@v/%s.info", escapedPath, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	client := r.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get module info from proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %s for %s@%s", resp.Status, modName, version)
	}

	var fMod fetchedMod
	if err := json.NewDecoder(resp.Body).Decode(&fMod); err != nil {
		return "", fmt.Errorf("decode proxy response: %w", err)
	}

	if fMod.Origin.Hash == "" {
		return "", fmt.Errorf("proxy returned empty commit hash for %s@%s", modName, version)
	}

	return fMod.Origin.Hash, nil
}
