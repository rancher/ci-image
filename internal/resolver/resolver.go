// Package resolver resolves release-checksums tool versions and checksums.
//
// There are two operations:
//   - ApplyLock: used by generate — reads version and checksums exclusively
//     from deps.lock and applies them to cfg. No network calls are made.
//   - Update: used by the update command — queries upstream for the latest
//     release, fetches checksums, and writes new entries into the lock.
//     If a version has changed since the last lock write, a warning is logged.
package resolver

import (
	"fmt"
	"log"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/config/renderer"
	gh "github.com/rancher/ci-image/internal/github"
	"github.com/rancher/ci-image/internal/lock"
)

// ApplyLock reads version and checksum data for all release-checksums tools
// exclusively from lk, and mutates cfg in-place so that downstream generation
// sees fully-resolved data.
//
// Returns an error if any release-checksums tool is absent from the lock or
// has no checksums recorded — run the update command to populate the lock.
func ApplyLock(cfg *config.Config, lk *lock.Lock) error {
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
func Update(cfg *config.Config, lk *lock.Lock) error {
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
		lk.Tools[t.Name] = lock.Entry{
			ResolvedVersion: version,
			ResolvedAt:      time.Now().UTC(),
			Checksums:       checksums,
		}
		log.Printf("tool %q: resolved %s", t.Name, version)
	}
	return nil
}

// resolveVersion returns the version to use for t. If t.Version is "latest",
// the GitHub API is queried for the current release tag.
func resolveVersion(t *config.Tool) (string, error) {
	if t.Version != "latest" {
		return t.Version, nil
	}
	owner, repo, err := gh.ParseSourceRepo(t.Source)
	if err != nil {
		return "", fmt.Errorf("cannot resolve latest version: %w", err)
	}
	return gh.LatestReleaseTag(owner, repo)
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
	baseVars := renderer.Vars{
		Name:    t.Name,
		Source:  t.Source,
		Version: version,
	}

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

func fetchFromAggregatedFile(t *config.Tool, version string, platforms []string, baseVars renderer.Vars, rel *config.ReleaseConfig) (map[string]string, error) {
	checksumTmpl := gh.ExpandGitHubTemplate(rel.ChecksumTemplate, t.Source)
	checksumURL, err := renderer.Render(checksumTmpl, baseVars)
	if err != nil {
		return nil, fmt.Errorf("checksum_template: %w", err)
	}
	fileMap, err := gh.FetchChecksumFile(checksumURL)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(platforms))
	for _, platform := range platforms {
		_, filename, err := platformDownloadFilename(t, version, platform, baseVars)
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

func fetchPerPlatform(t *config.Tool, version string, platforms []string, baseVars renderer.Vars, result map[string]string) (map[string]string, error) {
	for _, platform := range platforms {
		dlURL, filename, err := platformDownloadFilename(t, version, platform, baseVars)
		if err != nil {
			return nil, err
		}
		fileMap, err := gh.FetchChecksumFile(dlURL + ".sha256sum")
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
func platformDownloadFilename(t *config.Tool, version, platform string, baseVars renderer.Vars) (dlURL, filename string, err error) {
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid platform %q", platform)
	}
	vars := baseVars
	vars.OS = parts[0]
	vars.Arch = parts[1]
	vars.Version = version
	rel := t.EffectiveRelease()
	if rel == nil {
		return "", "", fmt.Errorf("no release config for tool %q", t.Name)
	}
	dlTmpl := gh.ExpandGitHubTemplate(rel.DownloadTemplate, t.Source)
	dlURL, err = renderer.Render(dlTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("download_template for %s: %w", platform, err)
	}
	return dlURL, path.Base(dlURL), nil
}
