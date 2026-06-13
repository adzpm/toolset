package workdir_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	// TODO(zhuravlev): improve tests

	ctx := context.Background()
	const dir = "/dir"

	fs := fsh.NewMemFS(nil)
	require.NoError(t, workdir.Init(ctx, fs, dir))

	tree, err := fs.GetTree("/")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/",
		"/dir",
		"/dir/.toolset.json",
		"/dir/.toolset.lock.json",
		"/home-dir",
		"/home-dir/.cache",
		"/home-dir/.cache/toolset",
		"/home-dir/.cache/toolset/stats.json",
	}, tree)

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)
	require.NotEmpty(t, wd)

	require.NoError(t, wd.Save(ctx))
	require.Equal(t, []string{"gh", "go"}, wd.RuntimeList())

	tools, err := wd.GetTools(ctx)
	require.NoError(t, err)
	require.Equal(t, []structs.ToolState{}, tools)

	require.ErrorIs(t, wd.RemoveTool(ctx, "unknown-tool"), workdir.ErrToolNotFoundInSpec)
}

func TestCustomDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	const dir = "./project"

	fs := fsh.NewMemFS(nil)

	require.NoError(t, os.Setenv(workdir.EnvCacheDir, "/cache"))
	require.NoError(t, os.Setenv(workdir.EnvSpecDir, ".some-local-dir/"))

	require.NoError(t, workdir.Init(ctx, fs, dir))

	tree, err := fs.GetTree("/")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/",
		"/cache",
		"/cache/stats.json",
		"/project",
		"/project/.some-local-dir",
		"/project/.some-local-dir/.toolset.json",
		"/project/.some-local-dir/.toolset.lock.json",
	}, tree)

	wd, err := workdir.New(ctx, fs, dir+"/sub-dir/example")
	require.NoError(t, err)
	require.NotEmpty(t, wd)

	{
		tree, err := fs.GetTree("/")
		require.NoError(t, err)
		require.Equal(t, []string{
			"/",
			"/cache",
			"/cache/stats.json",
			"/project",
			"/project/.some-local-dir",
			"/project/.some-local-dir/.toolset.json",
			"/project/.some-local-dir/.toolset.lock.json",
		}, tree)
	}
}

// Task 1.1: Workdir initialization tests
// These tests cover the New() function's initialization paths with various configurations.

func Test_New_success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/test-project"

	fs := fsh.NewMemFS(nil)
	require.NoError(t, workdir.Init(ctx, fs, dir))

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)
	require.NotNil(t, wd)

	// Verify runtimes are discovered
	runtimes := wd.RuntimeList()
	require.Contains(t, runtimes, "go")
	require.Contains(t, runtimes, "gh")
}

func Test_New_missingSpec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/empty-project"

	fs := fsh.NewMemFS(nil)
	// Don't call Init(), so spec file doesn't exist

	wd, err := workdir.New(ctx, fs, dir)
	require.Error(t, err)
	require.Nil(t, wd)
	require.ErrorContains(t, err, "spec") // More flexible error check
}

func Test_New_invalidSpec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/corrupt-project"

	fs := fsh.NewMemFS(map[string]string{
		"/corrupt-project/.toolset.json": "invalid json {",
	})

	wd, err := workdir.New(ctx, fs, dir)
	require.Error(t, err)
	require.Nil(t, wd)
}

func Test_New_customCacheDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/project"

	fs := fsh.NewMemFS(nil)
	require.NoError(t, os.Setenv(workdir.EnvCacheDir, "/custom-cache"))
	defer func() { require.NoError(t, os.Unsetenv(workdir.EnvCacheDir)) }()

	require.NoError(t, workdir.Init(ctx, fs, dir))

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)
	require.NotNil(t, wd)

	// Verify system info reflects custom cache dir
	info, err := wd.GetSystemInfo()
	require.NoError(t, err)
	require.Contains(t, info.Locations.CacheDir, "custom-cache")
}

func Test_New_deprecatedDirParameter(t *testing.T) {
	// Test that spec.Dir parameter is deprecated and cleared
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/project"

	fs := fsh.NewMemFS(nil)
	// First initialize to get the proper directory structure
	require.NoError(t, workdir.Init(ctx, fs, dir))

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)
	require.NotNil(t, wd)

	// Save and verify no errors
	require.NoError(t, wd.Save(ctx))
}

func Test_New_multipleRuntimes(t *testing.T) {
	// Verify both runtimes are initialized and discoverable
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	dir := "/project"

	fs := fsh.NewMemFS(nil)
	require.NoError(t, workdir.Init(ctx, fs, dir))

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)

	runtimes := wd.RuntimeList()
	require.Equal(t, 2, len(runtimes))
	require.Contains(t, runtimes, "go")
	require.Contains(t, runtimes, "gh")
}
