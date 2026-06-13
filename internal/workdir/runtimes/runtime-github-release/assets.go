package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/toolset/internal/fsh"
)

var errAutoDiscover = errors.New("auto-discover")

var _ githubClient = (*productionGithubClient)(nil)

// productionGithubClient wraps *github.Client and implements githubClient.
type productionGithubClient struct {
	client *github.Client
	fs     fsh.FS
	goos   string
	goarch string
}

func (c *productionGithubClient) GetAsset(ctx context.Context, owner, repo, tag string) (*github.ReleaseAsset, error) {
	release, _, err := c.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("get release by tag: %w", err)
	}

	targetAsset, err := autoDiscoverAsset(release.Assets, repo, tag, c.goos, c.goarch)
	if err != nil {
		if errors.Is(err, errAutoDiscover) {
			var assetNames []string
			for _, asset := range release.Assets {
				assetNames = append(assetNames, asset.GetName())
			}
			return nil, fmt.Errorf("could not auto-discover compatible asset for %s/%s (platform: %s/%s). Available assets: %v",
				owner, repo, c.goos, c.goarch, assetNames)
		}
		return nil, fmt.Errorf("auto-discover asset: %w", err)
	}

	return targetAsset, nil
}

func (c *productionGithubClient) GetLatestRelease(ctx context.Context, owner, repo string) (*github.RepositoryRelease, error) {
	release, _, err := c.client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func (c *productionGithubClient) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, error) {
	release, _, err := c.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func autoDiscoverAsset(assets []*github.ReleaseAsset, toolName, version, goos, goarch string) (*github.ReleaseAsset, error) {
	osNames := knownOSNames[goos]
	archNames := knownArchNames[goarch]

	if goos == goosDarwin {
		// xxx_darwin_all for multi-platform binaries
		archNames = append([]string{"all"}, archNames...)
	}

	if len(osNames) == 0 || len(archNames) == 0 {
		return nil, fmt.Errorf("unsupported local platform (%s/%s)", goos, goarch)
	}

	// Build regex patterns to try (in order of preference)
	patterns := []string{
		// Pattern 1: toolname-v1.0.0-darwin-arm64.tar.gz
		fmt.Sprintf(`(?i)^%s[-_](v)?%s[-_](%s)[-_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.xz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.TrimPrefix(version, "v"),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 2: toolname-darwin-arm64.tar.gz (no version)
		fmt.Sprintf(`(?i)^%s[-_](%s)[-_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.xz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 3: toolname_v1.0.0_darwin_arm64.tar.gz (underscores)
		fmt.Sprintf(`(?i)^%s[_](v)?%s[_](%s)[_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.TrimPrefix(version, "v"),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 4: toolname_darwin_arm64.tar.gz (underscores, no version)
		fmt.Sprintf(`(?i)^%s[_](%s)[_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 5: toolname_darwin_arm64
		fmt.Sprintf(`(?i)^%s[-_](%s)[-_](%s)$`,
			regexp.QuoteMeta(toolName),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
	}

	// Compile all patterns once, then scan assets for each.
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("regexp compile: %w", err)
		}
		compiled = append(compiled, re)
	}

	for _, re := range compiled {
		for _, asset := range assets {
			if re.MatchString(asset.GetName()) {
				return asset, nil
			}
		}
	}

	return nil, errAutoDiscover
}

func (c *productionGithubClient) DownloadAsset(ctx context.Context, owner, repo string, assetID int64, targetFile string) error {
	body, _, err := c.client.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, http.DefaultClient)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer body.Close() //nolint:errcheck

	target, err := c.fs.OpenFile(targetFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer target.Close() //nolint:errcheck

	if _, err := io.Copy(target, body); err != nil {
		return fmt.Errorf("copy body to file: %w", err)
	}

	return nil
}
