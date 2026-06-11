package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
)

// knownPlatforms is the set of OS/arch combinations checked when computing a pin.
var knownPlatforms = [][2]string{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"linux", "386"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// GetPin fetches the GitHub release for program and records the SHA256 digest for every
// known platform that has a matching asset. Platforms that have no matching asset are
// skipped with a log message (DN-2). Returns an error if any matched asset has an empty
// digest (release predates digest support) or no assets were found for any platform.
func (r *Runtime) GetPin(ctx context.Context, program string) (optional.Val[structs.Pin], error) {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return optional.Empty[structs.Pin](), fmt.Errorf("get module: %w", err)
	}

	owner, repo, ok := strings.Cut(mod.Mod.Name(), "/")
	if !ok {
		return optional.Empty[structs.Pin](), fmt.Errorf("unexpected module name (%s)", mod.Mod.Name())
	}

	release, _, err := r.github.Repositories.GetReleaseByTag(ctx, owner, repo, mod.Mod.Version())
	if err != nil {
		return optional.Empty[structs.Pin](), fmt.Errorf("get release by tag: %w", err)
	}

	var assets []structs.PinnedAsset
	for _, platform := range knownPlatforms {
		goos, goarch := platform[0], platform[1]

		asset, err := autoDiscoverAsset(release.Assets, repo, mod.Mod.Version(), goos, goarch)
		if err != nil {
			if errors.Is(err, errAutoDiscover) {
				fmt.Printf("no asset for %s/%s — skipping\n", goos, goarch)
				continue
			}
			return optional.Empty[structs.Pin](), fmt.Errorf("auto-discover %s/%s: %w", goos, goarch, err)
		}

		digest := asset.GetDigest()
		if digest == "" {
			return optional.Empty[structs.Pin](), fmt.Errorf(
				"asset %s has no digest (release predates digest support)", asset.GetName())
		}

		assets = append(assets, structs.PinnedAsset{
			OS:     goos,
			Arch:   goarch,
			Name:   asset.GetName(),
			Digest: digest,
		})
	}

	if len(assets) == 0 {
		return optional.Empty[structs.Pin](), fmt.Errorf(
			"no pinnable assets found for any known platform in %s@%s",
			mod.Mod.Name(), mod.Mod.Version())
	}

	return optional.New(structs.Pin{Assets: assets}), nil
}

// findPinnedAsset returns the PinnedAsset matching goos/goarch, or false if not found.
func findPinnedAsset(assets []structs.PinnedAsset, goos, goarch string) (structs.PinnedAsset, bool) {
	for _, a := range assets {
		if a.OS == goos && a.Arch == goarch {
			return a, true
		}
	}
	return structs.PinnedAsset{}, false
}
