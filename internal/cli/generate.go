package cli

import (
	"fmt"
	"log"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/config/renderer"
	"github.com/rancher/ci-image/internal/dockerfile"
	"github.com/rancher/ci-image/internal/fileutil"
	gh "github.com/rancher/ci-image/internal/github"
	"github.com/rancher/ci-image/internal/lock"
	"github.com/rancher/ci-image/internal/readme"
)

const (
	dockerfilesDir = "dockerfiles"
	archiveDir     = "archive"
	defaultConfig  = "deps.yaml"
	readmePath     = "README.md"
)

func runGenerate(args []string) error {
	configPath := defaultConfig
	imageRepo := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--image-repo" && i+1 < len(args):
			i++
			imageRepo = args[i]
		case strings.HasPrefix(args[i], "--image-repo="):
			imageRepo = strings.TrimPrefix(args[i], "--image-repo=")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// Load the lock file (empty if it doesn't exist yet).
	lockPath := filepath.Join(filepath.Dir(configPath), "deps.lock")
	lk, err := lock.Read(lockPath)
	if err != nil {
		return err
	}

	// Resolve release-checksums tools: fetch latest version + checksums.
	anyReleaseChecksums, err := resolveReleaseChecksums(cfg, lk)
	if err != nil {
		return err
	}

	// Generate Dockerfiles (cfg now has resolved versions/checksums).
	files, err := dockerfile.Generate(cfg, defaultSourceURL(imageRepo))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dockerfilesDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", dockerfilesDir, err)
	}

	for imageName, content := range files {
		outputPath := filepath.Join(dockerfilesDir, "Dockerfile."+imageName)
		changed, err := fileutil.WriteIfChanged(outputPath, []byte(content), 0o644)
		if err != nil {
			return fmt.Errorf("writing %s: %w", outputPath, err)
		}
		if changed {
			log.Printf("Updated %s", outputPath)
		}
	}

	// Write the lock file only if its content changed.
	if anyReleaseChecksums {
		changed, err := lock.WriteIfChanged(lockPath, lk)
		if err != nil {
			return fmt.Errorf("writing %s: %w", lockPath, err)
		}
		if changed {
			log.Printf("Updated %s", lockPath)
		}
	}

	// Archive any Dockerfiles for images no longer in config.
	if err := archiveRemovedDockerfiles(files); err != nil {
		return fmt.Errorf("archiving removed dockerfiles: %w", err)
	}

	// Write the compiled images lock.
	imagesLockPath := filepath.Join(filepath.Dir(configPath), "images-lock.yaml")
	if err := writeImagesLock(cfg, imagesLockPath); err != nil {
		return fmt.Errorf("writing %s: %w", imagesLockPath, err)
	}
	log.Printf("Generated %s", imagesLockPath)

	// Update the Available Images table in README.md.
	rows := make([]readme.ImageRow, 0, len(cfg.Images))
	for _, img := range cfg.Images {
		rows = append(rows, readme.ImageRow{
			Name:        img.Name,
			GoVersion:   extractGoVersion(img.Base),
			Description: img.Description,
		})
	}
	readmeFile := filepath.Join(filepath.Dir(configPath), readmePath)
	if err := readme.UpdateTable(readmeFile, rows); err != nil {
		return fmt.Errorf("updating %s: %w", readmeFile, err)
	}

	return nil
}

// archiveRemovedDockerfiles moves any Dockerfile.<name> in dockerfilesDir that
// is not present in the generated set to archiveDir/Dockerfile.<name>.<YYYYMMDD-HHMMSS> (UTC).
// This preserves history in git when images are removed from config.
func archiveRemovedDockerfiles(generated map[string]string) error {
	entries, err := os.ReadDir(dockerfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dateStr := time.Now().UTC().Format("20060102-150405")

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "Dockerfile.") {
			continue
		}
		imageName := strings.TrimPrefix(name, "Dockerfile.")
		if _, active := generated[imageName]; active {
			continue
		}
		// Image no longer in config — move to archive.
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return err
		}
		src := filepath.Join(dockerfilesDir, name)
		dst := filepath.Join(archiveDir, name+"."+dateStr)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		log.Printf("Archived removed image %q → %s", imageName, dst)
	}
	return nil
}

// imagesLock is the structure written to images-lock.yaml.
type imagesLock struct {
	Images   []string                   `yaml:"images"`
	Packages []string                   `yaml:"packages,omitempty"` // universal packages installed in every image
	Tools    map[string]string          `yaml:"tools,omitempty"`    // name → version, all tools across all images
	Configs  map[string]imageLockConfig `yaml:"configs"`
}

type imageLockConfig struct {
	Base        string   `yaml:"base"`
	Platforms   []string `yaml:"platforms"`
	Packages    []string `yaml:"packages,omitempty"` // image-specific packages only (excludes universal)
	Tools       []string `yaml:"tools,omitempty"`    // tool names only; versions in top-level tools map
	GoVersion   string   `yaml:"go_version,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

// extractGoVersion returns the Go version from a SUSE BCI golang base image
// reference (e.g. "registry.suse.com/bci/golang:1.25.9@sha256:…" → "1.25.9").
// Only matches the known BCI registry prefix; returns "" for any other base.
func extractGoVersion(base string) string {
	const prefix = "registry.suse.com/bci/golang:"
	if !strings.HasPrefix(base, prefix) {
		return ""
	}
	tag := base[len(prefix):]
	if at := strings.IndexByte(tag, '@'); at != -1 {
		tag = tag[:at]
	}
	return tag
}

const imagesLockHeader = "# images-lock.yaml — compiled image index generated by 'generate'.\n" +
	"# Records the active image names, universal packages, tool versions, and\n" +
	"# per-image configuration (base, platforms, image-specific packages, tools).\n" +
	"# Do not edit manually.\n"

// writeImagesLock writes images-lock.yaml: the active image names, top-level
// universal packages and tool versions, plus a per-image configs map with the
// resolved base, platforms, image-specific packages, tool memberships, and
// optional metadata such as Go version and description.
func writeImagesLock(cfg *config.Config, path string) error {
	lk := imagesLock{
		Packages: cfg.Packages,
		Tools:    make(map[string]string),
		Configs:  make(map[string]imageLockConfig, len(cfg.Images)),
	}

	// Build a set of universal packages so we can store only image-specific
	// additions in each config entry (mirrors how tools are split: top-level
	// map holds versions, per-image list holds membership).
	universalPkgs := make(map[string]bool, len(cfg.Packages))
	for _, p := range cfg.Packages {
		universalPkgs[p] = true
	}

	for _, img := range cfg.Images {
		lk.Images = append(lk.Images, img.Name)

		var toolNames []string
		for i := range cfg.Tools {
			t := &cfg.Tools[i]
			if config.ImageIncludesTool(img, t) {
				toolNames = append(toolNames, t.Name)
				lk.Tools[t.Name] = t.Version
			}
		}
		slices.Sort(toolNames)

		// img.Packages has universal packages prepended by load.go; strip them
		// so the per-image entry only records image-specific additions.
		var specificPkgs []string
		for _, p := range img.Packages {
			if !universalPkgs[p] {
				specificPkgs = append(specificPkgs, p)
			}
		}

		lk.Configs[img.Name] = imageLockConfig{
			Base:        img.Base,
			Platforms:   img.Platforms,
			Packages:    specificPkgs,
			Tools:       toolNames,
			GoVersion:   extractGoVersion(img.Base),
			Description: img.Description,
		}
	}

	body, err := yaml.Marshal(lk)
	if err != nil {
		return fmt.Errorf("marshalling images lock: %w", err)
	}
	_, err = fileutil.WriteIfChanged(path, append([]byte(imagesLockHeader), body...), 0o644)
	return err
}

// resolveReleaseChecksums iterates cfg.Tools, resolves version and checksums
// for every release-checksums tool, and mutates cfg in-place so that
// dockerfile.Generate sees fully-resolved data. lk is updated with new
// resolved versions and timestamps. Returns true if any tools were resolved.
func resolveReleaseChecksums(cfg *config.Config, lk *lock.Lock) (bool, error) {
	any := false
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.EffectiveMode() != "release-checksums" {
			continue
		}
		any = true

		// Resolve version (may be "latest" → query GitHub).
		version := t.Version
		if version == "latest" {
			owner, repo, err := gh.ParseSourceRepo(t.Source)
			if err != nil {
				return false, fmt.Errorf("tool %q: cannot resolve latest version: %w", t.Name, err)
			}
			resolved, err := gh.LatestReleaseTag(owner, repo)
			if err != nil {
				return false, fmt.Errorf("tool %q: %w", t.Name, err)
			}
			log.Printf("tool %q: resolved latest → %s", t.Name, resolved)
			version = resolved
		}

		// If the lock already has this version with checksums, reuse them —
		// avoids a network round-trip and pins the checksums to what was
		// previously verified.
		if cached, ok := lk.Tools[t.Name]; ok && cached.ResolvedVersion == version && len(cached.Checksums) > 0 {
			t.Version = version
			t.Checksums = cached.Checksums
			continue
		}

		// Collect all platforms required by images that include this tool.
		platforms := toolPlatforms(cfg, t)
		if len(platforms) == 0 {
			return false, fmt.Errorf("tool %q: no images include this tool — cannot determine required platforms", t.Name)
		}

		// Fetch checksums from the upstream checksum file.
		checksums, err := resolveChecksums(t, version, platforms)
		if err != nil {
			return false, fmt.Errorf("tool %q: %w", t.Name, err)
		}

		// Mutate cfg so downstream generation uses the resolved data,
		// and persist checksums to the lock so future runs can skip the fetch.
		t.Version = version
		t.Checksums = checksums
		lk.Tools[t.Name] = lock.Entry{
			ResolvedVersion: version,
			ResolvedAt:      time.Now().UTC(),
			Checksums:       checksums,
		}
	}
	return any, nil
}

// toolPlatforms returns the sorted union of platforms across all images that
// include t (either via universal or image.tools).
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

// resolveChecksums fetches the upstream checksum file for the given tool at
// version and returns a platform → sha256 map for the requested platforms.
//
// If release.checksum_template is set, a single checksum file is fetched and
// all platforms are looked up within it by download filename.
// If unset, each platform's download URL is fetched with ".sha256sum" appended.
func resolveChecksums(t *config.Tool, version string, platforms []string) (map[string]string, error) {
	baseVars := renderer.Vars{
		Name:    t.Name,
		Source:  t.Source,
		Version: version,
	}

	result := make(map[string]string, len(platforms))

	rel := t.EffectiveRelease()
	if rel == nil {
		return nil, fmt.Errorf("no release config (set release: block or use a GitHub source)")
	}

	if rel.ChecksumTemplate != "" {
		// Single aggregated checksum file for all platforms.
		checksumTmpl := gh.ExpandGitHubTemplate(rel.ChecksumTemplate, t.Source)
		checksumURL, err := renderer.Render(checksumTmpl, baseVars)
		if err != nil {
			return nil, fmt.Errorf("checksum_template: %w", err)
		}
		fileMap, err := gh.FetchChecksumFile(checksumURL)
		if err != nil {
			return nil, err
		}
		for _, platform := range platforms {
			dlURL, filename, err := platformDownloadFilename(t, version, platform, baseVars)
			if err != nil {
				return nil, err
			}
			checksum, ok := fileMap[filename]
			if !ok {
				return nil, fmt.Errorf("checksum file %s has no entry for %s (filename: %s)", checksumURL, platform, filename)
			}
			_ = dlURL
			result[platform] = checksum
		}
		return result, nil
	}

	// No checksum_template: fetch a per-platform checksum file at {download_url}.sha256sum.
	for _, platform := range platforms {
		dlURL, filename, err := platformDownloadFilename(t, version, platform, baseVars)
		if err != nil {
			return nil, err
		}
		checksumURL := dlURL + ".sha256sum"
		fileMap, err := gh.FetchChecksumFile(checksumURL)
		if err != nil {
			return nil, err
		}
		checksum, ok := fileMap[filename]
		if !ok {
			// Some tools emit bare checksums (just the hash, no filename).
			if len(fileMap) == 1 {
				for _, v := range fileMap {
					checksum = v
				}
			} else {
				return nil, fmt.Errorf("checksum file %s has no entry for %s", checksumURL, filename)
			}
		}
		result[platform] = checksum
	}
	return result, nil
}

// platformDownloadFilename renders the download URL for a platform and returns
// both the full URL and the basename (used as key in checksum files).
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
	filename = path.Base(dlURL)
	return dlURL, filename, nil
}

const defaultSourceRepo = "rancher/ci-image"

func defaultSourceURL(override string) string {
	repo := defaultSourceRepo
	if override != "" {
		repo = override
	}
	return fmt.Sprintf("https://github.com/%s", repo)
}
