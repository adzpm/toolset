package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
)

// GetPin fetches the GitHub release for program and records the SHA256 digest for every
// known platform that has a matching asset. Platforms that have no matching asset are
// skipped with a log message (DN-2). Returns an error if any matched asset has an empty
// digest (release predates digest support) or no assets were found for any platform.
func (r *Runtime) GetPin(ctx context.Context, program string) (optional.Val[structs.Pin], error) {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return optional.Empty[structs.Pin](), fmt.Errorf("get module: %w", err)
	}

	// owner/repo is guaranteed by parse() — GetModule enforces the format.
	owner, repo, _ := strings.Cut(mod.Mod.Name(), "/")

	release, err := r.gh.GetReleaseByTag(ctx, owner, repo, mod.Mod.Version())
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
