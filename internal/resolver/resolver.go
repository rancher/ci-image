// Package resolver resolves release-checksums tool versions and checksums.
//
// There are three operations:
//   - ApplyLock: used by generate — reads version and checksums exclusively
//     from deps.lock and applies them to cfg. No network calls are made.
//   - Update: used by the update command — queries upstream for the latest
//     release, fetches checksums, and writes new entries into the lock.
//     If a version has changed since the last lock write, a warning is logged.
//   - ValidateLock: used by validate — re-fetches checksums for the versions
//     recorded in deps.lock and verifies they match. Detects tampering or
//     manual edits that introduced incorrect checksums.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	ghrelease "github.com/mallardduck/ghreleases"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/resolver/depslock"
)

// ApplyLock reads version and checksum data for all release-checksums tools
// exclusively from lk, and mutates cfg in-place so that downstream generation
// sees fully-resolved data.
//
// Returns an error if any release-checksums tool is absent from the lock or
// has no checksums recorded — run the update command to populate the lock.
func ApplyLock(cfg *config.Config, lk *depslock.Lock) error {
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.EffectiveMode() != "release-checksums" {
			continue
		}

		cached, ok := lk.Tools[t.Name]
		if !ok || cached.ResolvedVersion == "" || len(cached.Checksums) == 0 {
			return fmt.Errorf("tool %q: not found in deps.lock; run 'update' to resolve release-checksum tools", t.Name)
		}

		t.Version = cached.ResolvedVersion
		t.Checksums = cached.Checksums
	}
	return nil
}

// Update resolves the latest version and checksums for all release-checksums
// tools by querying upstream, then updates lk in place. If a tool's version
// has changed from what the lock records, a warning is logged but the update
// proceeds normally.
func Update(cfg *config.Config, lk *depslock.Lock) error {
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.EffectiveMode() != "release-checksums" {
			continue
		}

		version, err := resolveVersion(t)
		if err != nil {
			return fmt.Errorf("tool %q: %w", t.Name, err)
		}

		if cached, ok := lk.Tools[t.Name]; ok && cached.ResolvedVersion != "" {
			if cached.ResolvedVersion != version {
				log.Printf("WARNING: update release-checksum based dep %q: %s → %s", t.Name, cached.ResolvedVersion, version)
			} else if len(cached.Checksums) > 0 {
				log.Printf("tool %q: already at %s", t.Name, version)
				continue
			}
		}

		platforms := toolPlatforms(cfg, t)
		if len(platforms) == 0 {
			return fmt.Errorf("tool %q: no images include this tool — cannot determine required platforms", t.Name)
		}

		checksums, err := fetchChecksums(t, version, platforms)
		if err != nil {
			return fmt.Errorf("tool %q: %w", t.Name, err)
		}

		t.Version = version
		t.Checksums = checksums
		lk.Tools[t.Name] = depslock.Entry{
			ResolvedVersion: version,
			ResolvedAt:      time.Now().UTC(),
			Checksums:       checksums,
		}
		log.Printf("tool %q: resolved %s", t.Name, version)
	}
	return nil
}

// ValidateLock verifies that the checksums recorded in lk match the actual
// checksums for the stated versions. This detects tampering or manual edits
// where the version was changed but checksums weren't updated correctly.
//
// Returns an error if any tool's checksums don't match what's upstream for
// the recorded version. Allows stale (old but correct) lock entries.
func ValidateLock(cfg *config.Config, lk *depslock.Lock) error {
	var problems []string

	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.EffectiveMode() != "release-checksums" {
			continue
		}

		cached, ok := lk.Tools[t.Name]
		if !ok || cached.ResolvedVersion == "" || len(cached.Checksums) == 0 {
			problems = append(problems, fmt.Sprintf("tool %q: not found in deps.lock or missing checksums", t.Name))
			continue
		}

		// Use the version from the lock, not "latest"
		version := cached.ResolvedVersion

		platforms := toolPlatforms(cfg, t)
		if len(platforms) == 0 {
			problems = append(problems, fmt.Sprintf("tool %q: no images include this tool — cannot validate", t.Name))
			continue
		}

		// Re-fetch checksums for the stated version
		expectedChecksums, err := fetchChecksums(t, version, platforms)
		if err != nil {
			errMsg := fmt.Sprintf("tool %q: failed to fetch checksums for %s: %v", t.Name, version, err)
			var statusErr *ghrelease.HTTPStatusError
			if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
				errMsg += "\n    (This may indicate manual editing of deps.lock with a non-existent version, or the release was deleted upstream)"
			}
			problems = append(problems, errMsg)
			continue
		}

		// Compare fetched checksums against what's in the lock
		for platform, expectedSum := range expectedChecksums {
			actualSum, ok := cached.Checksums[platform]
			if !ok {
				problems = append(problems, fmt.Sprintf("tool %q (%s): missing checksum for platform %s", t.Name, version, platform))
				continue
			}
			if actualSum != expectedSum {
				problems = append(problems, fmt.Sprintf("tool %q (%s): checksum mismatch for %s:\n  locked: %s\n  actual: %s", t.Name, version, platform, actualSum, expectedSum))
			}
		}

		// Check for extra platforms in lock that aren't needed
		for platform := range cached.Checksums {
			if _, ok := expectedChecksums[platform]; !ok {
				log.Printf("WARNING: tool %q: lock has checksum for platform %s but it's not required by any image", t.Name, platform)
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("deps.lock validation failed:\n  %s", strings.Join(problems, "\n  "))
	}

	return nil
}

// FetchToolChecksums fetches upstream checksums for t at version using the
// tool's own release configuration, for every platform of every image that
// includes t. It is the same lookup Update and ValidateLock use, so a tool's
// release: block (download_template, checksum_template) is the single source
// of truth for where checksums come from.
func FetchToolChecksums(cfg *config.Config, t *config.Tool, version string) (map[string]string, error) {
	platforms := toolPlatforms(cfg, t)
	if len(platforms) == 0 {
		return nil, fmt.Errorf("no images include this tool — cannot determine required platforms")
	}
	return fetchChecksums(t, version, platforms)
}

// HashDownloads computes checksums by downloading each platform's artifact
// (per the tool's download_template) and hashing it locally. It is the
// fallback for upstreams that publish no checksum file. The result only proves
// the bytes match what was downloaded, not that upstream vouches for them, so
// callers must make that visible to a human before it is used. It also returns
// the URLs that were hashed.
func HashDownloads(cfg *config.Config, t *config.Tool, version string) (sums map[string]string, urls []string, err error) {
	platforms := toolPlatforms(cfg, t)
	if len(platforms) == 0 {
		return nil, nil, fmt.Errorf("no images include this tool — cannot determine required platforms")
	}
	sums = make(map[string]string, len(platforms))
	for _, p := range platforms {
		u, _, err := platformDownloadFilename(t, version, p)
		if err != nil {
			return nil, nil, err
		}
		res, err := ghClient().Download(u, io.Discard, ghrelease.DownloadOptions{})
		if err != nil {
			return nil, nil, err
		}
		sums[p] = res.Hash
		urls = append(urls, u)
	}
	return sums, urls, nil
}

// ChecksumLocation describes where FetchToolChecksums looks for t at version:
// the rendered checksum_template URL (plus the column-order file for multi-hash
// files), or the per-platform "<download>.sha256sum" URLs. Used to show the
// reviewer what a checksum is derived from.
func ChecksumLocation(cfg *config.Config, t *config.Tool, version string) ([]string, error) {
	rel := t.EffectiveRelease()
	if rel == nil {
		return nil, fmt.Errorf("no release config")
	}
	baseVars := config.TemplateVars(t.Name, t.Source, version, "", "", "")
	if rel.ChecksumTemplate != "" {
		u, err := config.RenderTemplate(config.ExpandGitHubTemplate(rel.ChecksumTemplate, t.Source), baseVars)
		if err != nil {
			return nil, err
		}
		urls := []string{u}
		if rel.ChecksumHashesOrderTemplate != "" {
			o, err := config.RenderTemplate(config.ExpandGitHubTemplate(rel.ChecksumHashesOrderTemplate, t.Source), baseVars)
			if err != nil {
				return nil, err
			}
			urls = append(urls, o+" (column order, SHA-256 selected by name)")
		}
		return urls, nil
	}
	var urls []string
	for _, p := range toolPlatforms(cfg, t) {
		u, _, err := platformDownloadFilename(t, version, p)
		if err != nil {
			return nil, err
		}
		urls = append(urls, u+".sha256sum")
	}
	return urls, nil
}

// resolveVersion returns the version to use for t. If t.Version is "latest",
// the GitHub API is queried for the current release tag.
func resolveVersion(t *config.Tool) (string, error) {
	if t.Version != "latest" {
		return t.Version, nil
	}
	owner, repo, err := ghrelease.ParseSource(t.Source)
	if err != nil {
		return "", fmt.Errorf("cannot resolve latest version: %w", err)
	}
	ctx := ghrelease.WithTagPrefix(context.Background(), t.TagPrefix())
	tag, err := ghClient().LatestRelease(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	if tag == "" {
		return "", fmt.Errorf("GitHub API returned empty tag_name for %s/%s", owner, repo)
	}
	return tag, nil
}

// toolPlatforms returns the sorted union of platforms across all images that
// include t (either as a universal tool or via image.tools).
func toolPlatforms(cfg *config.Config, t *config.Tool) []string {
	seen := make(map[string]bool)
	for _, img := range cfg.Images {
		if !config.ImageIncludesTool(img, t) {
			continue
		}
		for _, p := range img.Platforms {
			seen[p] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// fetchChecksums downloads the upstream checksum file for t at version and
// returns a platform → sha256 map for the requested platforms.
//
// If release.checksum_template is set, a single aggregated checksum file is
// fetched and each platform is looked up by its download filename. Otherwise,
// a per-platform file at "{download_url}.sha256sum" is fetched.
func fetchChecksums(t *config.Tool, version string, platforms []string) (map[string]string, error) {
	baseVars := config.TemplateVars(t.Name, t.Source, version, "", "", "")

	rel := t.EffectiveRelease()
	if rel == nil {
		return nil, fmt.Errorf("no release config (set release: block or use a GitHub source)")
	}

	result := make(map[string]string, len(platforms))

	if rel.ChecksumTemplate != "" {
		return fetchFromAggregatedFile(t, version, platforms, baseVars, rel)
	}
	return fetchPerPlatform(t, version, platforms, baseVars, result)
}

func fetchFromAggregatedFile(t *config.Tool, version string, platforms []string, baseVars ghrelease.TemplateVars, rel *config.ReleaseConfig) (map[string]string, error) {
	checksumTmpl := config.ExpandGitHubTemplate(rel.ChecksumTemplate, t.Source)
	checksumURL, err := config.RenderTemplate(checksumTmpl, baseVars)
	if err != nil {
		return nil, fmt.Errorf("checksum_template: %w", err)
	}
	var fileMap map[string]string
	if rel.ChecksumHashesOrderTemplate != "" {
		// Multi-hash file: the SHA-256 column is chosen by name from the order file.
		orderTmpl := config.ExpandGitHubTemplate(rel.ChecksumHashesOrderTemplate, t.Source)
		orderURL, rerr := config.RenderTemplate(orderTmpl, baseVars)
		if rerr != nil {
			return nil, fmt.Errorf("checksum_hashes_order_template: %w", rerr)
		}
		fileMap, err = ghClient().FetchMultiHashChecksums(context.Background(), checksumURL, orderURL)
	} else {
		fileMap, err = ghClient().FetchChecksumsLenient(context.Background(), checksumURL)
	}
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(platforms))
	for _, platform := range platforms {
		_, filename, err := platformDownloadFilename(t, version, platform)
		if err != nil {
			return nil, err
		}
		checksum, ok := fileMap[filename]
		if !ok {
			return nil, fmt.Errorf("checksum file %s has no entry for %s (filename: %s)", checksumURL, platform, filename)
		}
		result[platform] = checksum
	}
	return result, nil
}

func fetchPerPlatform(t *config.Tool, version string, platforms []string, baseVars ghrelease.TemplateVars, result map[string]string) (map[string]string, error) {
	for _, platform := range platforms {
		dlURL, filename, err := platformDownloadFilename(t, version, platform)
		if err != nil {
			return nil, err
		}
		fileMap, err := ghClient().FetchChecksumsLenient(context.Background(), dlURL+".sha256sum")
		if err != nil {
			return nil, err
		}
		checksum, ok := fileMap[filename]
		if !ok {
			if len(fileMap) == 1 {
				for _, v := range fileMap {
					checksum = v
				}
			} else {
				return nil, fmt.Errorf("checksum file %s.sha256sum has no entry for %s", dlURL, filename)
			}
		}
		result[platform] = checksum
	}
	return result, nil
}

// platformDownloadFilename renders the download URL for a platform and returns
// both the full URL and the basename (used as the key in checksum files).
func platformDownloadFilename(t *config.Tool, version, platform string) (dlURL, filename string, err error) {
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid platform %q", platform)
	}
	vars := config.TemplateVars(t.Name, t.Source, version, "", parts[0], parts[1])
	rel := t.EffectiveRelease()
	if rel == nil {
		return "", "", fmt.Errorf("no release config for tool %q", t.Name)
	}
	dlTmpl := config.ExpandGitHubTemplate(rel.DownloadTemplate, t.Source)
	dlURL, err = config.RenderTemplate(dlTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("download_template for %s: %w", platform, err)
	}
	return dlURL, path.Base(dlURL), nil
}
