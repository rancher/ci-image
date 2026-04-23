package dockerfile

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/config/renderer"
	gh "github.com/rancher/ci-image/internal/github"
)

// NewDockerfileVars builds a fully-resolved DockerfileVars for img.
// All template rendering is performed here; if construction succeeds,
// Render() is guaranteed to succeed.
//
// cfg.Tools must already have checksums populated for release-checksums tools
// (call resolveReleaseChecksums before this).
func NewDockerfileVars(cfg *config.Config, img config.Image, sourceURL string) (DockerfileVars, error) {
	// Collect tools: universal first (in config order), then image-specific.
	toolsByName := make(map[string]config.Tool, len(cfg.Tools))
	for _, t := range cfg.Tools {
		toolsByName[t.Name] = t
	}
	var tools []config.Tool
	for _, t := range cfg.Tools {
		if t.Universal {
			tools = append(tools, t)
		}
	}
	for _, name := range img.Tools {
		if t, ok := toolsByName[name]; ok {
			tools = append(tools, t)
		}
	}

	imgPlatforms := make(map[string]bool, len(img.Platforms))
	for _, p := range img.Platforms {
		imgPlatforms[p] = true
	}

	var toolInstalls []ToolInstall
	var errs []string
	for _, t := range tools {
		install, err := buildItemInstall(t, imgPlatforms)
		if err != nil {
			errs = append(errs, fmt.Sprintf("tool %q: %s", t.Name, err))
			continue
		}
		toolInstalls = append(toolInstalls, ToolInstall{
			Name:    t.Name,
			Version: t.Version,
			Install: install,
		})
	}
	if len(errs) > 0 {
		return DockerfileVars{}, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	return DockerfileVars{
		Base:        img.Base,
		Packages:    img.Packages,
		Tools:       toolInstalls,
		SourceURL:   sourceURL,
		Title:       "Rancher " + img.Name + " CI image",
		Description: img.Description,
	}, nil
}

func buildItemInstall(t config.Tool, imgPlatforms map[string]bool) (ItemInstall, error) {
	switch t.Install.EffectiveMethod() {
	case "curl":
		return buildCurlInstall(t, imgPlatforms)
	case "go-install":
		return buildGoInstall(t)
	default:
		return nil, fmt.Errorf("unknown install method %q", t.Install.EffectiveMethod())
	}
}

func buildCurlInstall(t config.Tool, imgPlatforms map[string]bool) (CurlInstall, error) {
	rel := t.EffectiveRelease() // non-nil guaranteed by config validation

	downloadTmpl := gh.ExpandGitHubTemplate(rel.DownloadTemplate, t.Source)
	extractTmpl := rel.Extract

	baseVars := renderer.Vars{
		Name:          t.Name,
		Source:        t.Source,
		Version:       t.Version,
		VersionCommit: t.VersionCommit,
	}

	// Intersect tool checksums with image platforms, sorted for determinism.
	allPlatforms := slices.Sorted(maps.Keys(t.Checksums))
	var platforms []PlatformInstall
	for _, platform := range allPlatforms {
		if !imgPlatforms[platform] {
			continue
		}
		parts := strings.SplitN(platform, "/", 2)
		if len(parts) != 2 {
			return CurlInstall{}, fmt.Errorf("invalid platform format %q", platform)
		}
		vars := baseVars
		vars.OS = parts[0]
		vars.Arch = parts[1]

		dlURL, err := renderer.Render(downloadTmpl, vars)
		if err != nil {
			return CurlInstall{}, fmt.Errorf("download_template: %w", err)
		}
		extract, err := renderer.Render(extractTmpl, vars)
		if err != nil {
			return CurlInstall{}, fmt.Errorf("extract: %w", err)
		}
		platforms = append(platforms, PlatformInstall{
			Arch:        parts[1],
			DownloadURL: dlURL,
			Extract:     extract,
			Checksum:    t.Checksums[platform],
		})
	}

	if len(platforms) == 0 {
		return CurlInstall{}, fmt.Errorf("no platforms in common between tool checksums and image platforms")
	}

	// Format is uniform across platforms — derive from the first rendered URL.
	format, ext := detectFormat(platforms[0].DownloadURL)

	return CurlInstall{
		Name:       t.Name,
		Format:     format,
		ArchiveExt: ext,
		Platforms:  platforms,
	}, nil
}

func buildGoInstall(t config.Tool) (GoInstall, error) {
	vars := renderer.Vars{
		Name:          t.Name,
		Source:        t.Source,
		Version:       t.Version,
		VersionCommit: t.VersionCommit,
	}
	pkg, err := renderer.Render(t.Install.Package, vars)
	if err != nil {
		return GoInstall{}, fmt.Errorf("install.package: %w", err)
	}
	return GoInstall{Package: pkg}, nil
}

// detectFormat classifies a rendered download URL as "archive", "gzip", or "binary",
// and returns the archive extension (non-empty only for "archive").
func detectFormat(url string) (format, ext string) {
	if ext = archiveExt(url); ext != "" {
		return "archive", ext
	}
	if isGzipBinaryURL(url) {
		return "gzip", ""
	}
	return "binary", ""
}
