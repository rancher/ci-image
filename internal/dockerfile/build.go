package dockerfile

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/config/renderer"
	gh "github.com/rancher/ci-image/internal/github"
)

// Format describes the packaging/compression type of a download artifact.
type Format string

const (
	FormatArchive    Format = "archive" // tar/zip archive
	FormatGzip       Format = "gzip"    // gzipped file (.gz)
	FormatExecutable Format = "binary"  // raw executable file (binary or script)
)

// String returns the format as a string.
func (f Format) String() string {
	return string(f)
}

// Validate checks if the format is a known value.
func (f Format) Validate() error {
	switch f {
	case FormatArchive, FormatGzip, FormatExecutable:
		return nil
	default:
		return fmt.Errorf("invalid format: %q (must be one of: archive, gzip, binary)", f)
	}
}

// NewDockerfileVars builds a fully-resolved DockerfileVars for img.
// All template rendering is performed here; if construction succeeds,
// Render() is guaranteed to succeed.
//
// cfg.Tools must already have checksums populated for release-checksums tools
// (call resolveReleaseChecksums before this).
func NewDockerfileVars(cfg *config.Config, img config.Image, sourceURL string) (DockerfileVars, error) {
	// Load hook templates from hooks/ directory (if it exists)
	hookTemplates, err := loadHookTemplates()
	if err != nil {
		return DockerfileVars{}, fmt.Errorf("loading hook templates: %w", err)
	}

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
			Setup:   buildToolSetup(t, hookTemplates),
		})
	}
	if len(errs) > 0 {
		return DockerfileVars{}, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	// Collect family selectors: one per unique family across all tools in this image.
	type familySel struct {
		defaultTool string
		validTools  []string
	}
	familyMap := make(map[string]*familySel)
	for _, t := range tools {
		if t.Family == "" {
			continue
		}
		if _, ok := familyMap[t.Family]; !ok {
			familyMap[t.Family] = &familySel{}
		}
		fs := familyMap[t.Family]
		fs.validTools = append(fs.validTools, t.Name)
		if t.FamilyDefault {
			fs.defaultTool = t.Name
		}
	}
	selectors := make([]SelectorInstall, 0, len(familyMap))
	for family, fs := range familyMap {
		slices.Sort(fs.validTools)
		selectors = append(selectors, SelectorInstall{
			Family:      family,
			DefaultTool: fs.defaultTool,
			ValidTools:  fs.validTools,
		})
	}
	slices.SortFunc(selectors, func(a, b SelectorInstall) int { return strings.Compare(a.Family, b.Family) })

	aliases := make([]AliasInstall, 0, len(img.Aliases))
	for name, target := range img.Aliases {
		aliases = append(aliases, AliasInstall{Name: name, Target: target})
	}
	slices.SortFunc(aliases, func(a, b AliasInstall) int { return strings.Compare(a.Name, b.Name) })

	return DockerfileVars{
		Base:        img.Base,
		Packages:    img.Packages,
		Tools:       toolInstalls,
		Selectors:   selectors,
		Aliases:     aliases,
		SourceURL:   sourceURL,
		Title:       "Rancher " + img.Name + " CI image",
		Description: img.Description,
	}, nil
}

func buildItemInstall(t config.Tool, imgPlatforms map[string]bool) (ItemInstall, error) {
	switch t.Install.EffectiveMethod() {
	case config.MethodCurl:
		return buildCurlInstall(t, imgPlatforms)
	case config.MethodGoInstall:
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
		Name:          t.Name,
		Format:        format,
		ArchiveExt:    ext,
		Platforms:     platforms,
		InstallToPath: rel.ShouldInstallToPath(),
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

// buildToolSetup checks for pre/post hook templates for the given tool.
// Returns nil if no hooks exist, otherwise returns a ToolSetup with template names set.
func buildToolSetup(t config.Tool, tmpl *template.Template) *ToolSetup {
	preTemplate := t.Name + "-pre.tmpl"
	postTemplate := t.Name + "-post.tmpl"

	hasPre := tmpl.Lookup(preTemplate) != nil
	hasPost := tmpl.Lookup(postTemplate) != nil

	if !hasPre && !hasPost {
		return nil // No hooks for this tool
	}

	setup := &ToolSetup{
		templates: tmpl,
	}
	if hasPre {
		setup.PreTemplate = preTemplate
	}
	if hasPost {
		setup.PostTemplate = postTemplate
	}
	return setup
}

// detectFormat classifies a download URL into a format based on packaging/compression.
// Returns one of: FormatArchive, FormatGzip, FormatExecutable.
// For archives, also returns the archive extension (.tar.gz, .zip, etc).
//
// Detection logic:
// 1. If URL is an archive (.tar.gz, .zip, etc) → FormatArchive
// 2. If URL ends with .gz (but not .tar.gz) → FormatGzip
// 3. Otherwise → FormatExecutable
//
// Format describes the artifact packaging, NOT what to do with it (copy vs run).
// That distinction is handled at template selection time based on ScriptArgs.
func detectFormat(url string) (Format, string) {
	// Check for archives first
	if ext := archiveExt(url); ext != "" {
		return FormatArchive, ext
	}

	// Gzipped executable
	if isGzipBinaryURL(url) {
		return FormatGzip, ""
	}

	// Default to raw executable (binary or script)
	return FormatExecutable, ""
}
