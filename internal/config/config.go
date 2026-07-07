package config

import (
	"fmt"
	"strings"
)

// Mode describes how tool versions are managed.
type Mode string

const (
	ModePinned           Mode = "pinned"            // version and checksums are static in deps.yaml
	ModeStatic           Mode = "static"            // version is static, checksums are in deps.yaml
	ModeReleaseChecksums Mode = "release-checksums" // checksums fetched from release artifacts at generate time
)

// String returns the mode as a string.
func (m Mode) String() string {
	return string(m)
}

// Validate checks if the mode is a known value.
func (m Mode) Validate() error {
	switch m {
	case ModePinned, ModeStatic, ModeReleaseChecksums:
		return nil
	default:
		return fmt.Errorf("invalid mode: %q (must be one of: pinned, static, release-checksums)", m)
	}
}

// Method describes how a tool is installed in the Dockerfile.
type Method string

const (
	MethodCurl      Method = "curl"       // download via curl, verify checksums
	MethodGoInstall Method = "go-install" // install via go install
)

// String returns the method as a string.
func (m Method) String() string {
	return string(m)
}

// Validate checks if the method is a known value.
func (m Method) Validate() error {
	switch m {
	case MethodCurl, MethodGoInstall:
		return nil
	default:
		return fmt.Errorf("invalid method: %q (must be one of: curl, go-install)", m)
	}
}

// Config is the top-level structure of deps.yaml.
type Config struct {
	Images    []Image  `yaml:"images"`
	Packages  []string `yaml:"packages"`  // zypper packages installed in every image
	Universal []Tool   `yaml:"universal"` // tools installed in every image
	Tools     []Tool   `yaml:"tools"`     // tools added by name in image.tools

	// Precomputed indices populated during Load()/validation; not serialized.
	// Used for family expansion, validation, and dockerfile generation.
	toolsByName   map[string]*Tool           // all tools indexed by name
	toolsByFamily map[string][]*Tool         // non-empty family → tools in that family
	families      map[string]*familyMetadata // family metadata (defaults, etc.)
}

// familyMetadata holds precomputed information about a tool family.
type familyMetadata struct {
	name         string   // family name
	tools        []string // tool names in this family
	defaultTool  string   // tool marked with family_default: true (empty if none)
	defaultCount int      // count of tools with family_default: true (for validation)
}

// ensureIndices ensures the precomputed indices are built.
// Safe to call multiple times (idempotent).
func (cfg *Config) ensureIndices() {
	if cfg.toolsByName != nil {
		return // already built
	}
	cfg.buildIndices()
}

// buildIndices populates the precomputed index fields in cfg.
// Should be called after unmarshaling but before validation.
// This centralizes the dependency graph computation so it's available
// throughout validation, dockerfile generation, and lock generation.
func (cfg *Config) buildIndices() {
	// Build toolsByName map
	allTools := make([]*Tool, 0, len(cfg.Universal)+len(cfg.Tools))
	for i := range cfg.Universal {
		cfg.Universal[i].Universal = true
		allTools = append(allTools, &cfg.Universal[i])
	}
	for i := range cfg.Tools {
		allTools = append(allTools, &cfg.Tools[i])
	}

	cfg.toolsByName = make(map[string]*Tool, len(allTools))
	for _, t := range allTools {
		cfg.toolsByName[t.Name] = t
	}

	// Build toolsByFamily and families maps
	cfg.toolsByFamily = make(map[string][]*Tool)
	cfg.families = make(map[string]*familyMetadata)

	for _, t := range allTools {
		if t.Family == "" {
			continue
		}

		cfg.toolsByFamily[t.Family] = append(cfg.toolsByFamily[t.Family], t)

		if _, ok := cfg.families[t.Family]; !ok {
			cfg.families[t.Family] = &familyMetadata{
				name:  t.Family,
				tools: []string{},
			}
		}

		fm := cfg.families[t.Family]
		fm.tools = append(fm.tools, t.Name)
		if t.FamilyDefault {
			fm.defaultTool = t.Name
			fm.defaultCount++
		}
	}
}

// ToolByName looks up a tool by name from the precomputed index.
// Returns nil if not found.
func (cfg *Config) ToolByName(name string) *Tool {
	return cfg.toolsByName[name]
}

// ToolsByFamily returns all tools in the given family from the precomputed index.
// Returns nil if the family doesn't exist.
func (cfg *Config) ToolsByFamily(family string) []*Tool {
	return cfg.toolsByFamily[family]
}

// FamilyMetadata returns metadata about a family from the precomputed index.
// Returns nil if the family doesn't exist.
func (cfg *Config) FamilyMetadata(family string) *familyMetadata {
	return cfg.families[family]
}

// ResolveToolReference resolves a tool reference (which may be a tool name or family name)
// into a list of concrete tools. Returns nil if the reference is invalid.
// This handles family expansion: "kubectl" → [kubectl1.28, kubectl1.29, kubectl1.30, kubectl1.31]
func (cfg *Config) ResolveToolReference(ref string) []*Tool {
	cfg.ensureIndices()

	// Try direct tool lookup first
	if t := cfg.toolsByName[ref]; t != nil {
		return []*Tool{t}
	}

	// Try family expansion
	if tools := cfg.toolsByFamily[ref]; tools != nil {
		return tools
	}

	return nil
}

// ResolveImageTools returns all concrete tools included in the given image,
// after expanding family references and including universal tools.
// Returns tools in a deterministic order.
func (cfg *Config) ResolveImageTools(img Image) []*Tool {
	cfg.ensureIndices()

	seen := make(map[string]bool)
	var result []*Tool

	// Add universal tools first
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.Universal {
			result = append(result, t)
			seen[t.Name] = true
		}
	}

	// Add image-specific tools (with family expansion)
	for _, ref := range img.Tools {
		resolved := cfg.ResolveToolReference(ref)
		for _, t := range resolved {
			if !seen[t.Name] {
				result = append(result, t)
				seen[t.Name] = true
			}
		}
	}

	return result
}

// Image defines a Docker image to generate.
type Image struct {
	Name        string            `yaml:"name"`
	Base        string            `yaml:"base"`
	Platforms   []string          `yaml:"platforms"`
	Packages    []string          `yaml:"packages"`
	Tools       []string          `yaml:"tools,omitempty"`       // tool names; must not include universal tools
	Aliases     map[string]string `yaml:"aliases,omitempty"`     // symlink_name: tool_name; creates /usr/local/bin symlinks
	Description string            `yaml:"description,omitempty"` // org.opencontainers.image.description; optional
}

// ChecksumList is a map of checksums for tools - where key is platform and value is checksum
type ChecksumList map[string]string

// Tool defines a binary tool available for inclusion in images.
type Tool struct {
	Name          string         `yaml:"name"`
	Family        string         `yaml:"family,omitempty"`         // for grouping tools (e.g. "helm"); tools sharing a family get a runtime selector script
	FamilyDefault bool           `yaml:"family_default,omitempty"` // this tool is used when the selector env var is not set; requires family to be set
	Source        string         `yaml:"source"`
	Version       string         `yaml:"version"`
	VersionCommit string         `yaml:"version_commit,omitempty"`
	Mode          Mode           `yaml:"mode,omitempty"` // defaults to ModePinned
	Universal     bool           `yaml:"-"`              // set by loader; use universal: section in deps.yaml
	Checksums     ChecksumList   `yaml:"checksums,omitempty"`
	Release       *ReleaseConfig `yaml:"release,omitempty"`
	Install       InstallConfig  `yaml:"install"`
}

// EffectiveMode returns the tool's mode, defaulting to ModePinned.
func (t *Tool) EffectiveMode() Mode {
	if t.Mode == "" {
		return ModePinned
	}
	return t.Mode
}

// EffectiveRelease returns the ReleaseConfig to use for this tool.
// For GitHub-sourced release-checksums tools, any fields not set in the
// release: block are filled from these defaults:
//
//	download_template: {name}_{os}_{arch}
//	checksum_template: checksums.txt
//	extract:           {name}  (direct binary, no archive)
//
// For non-GitHub or non-release-checksums tools, the release block is returned
// as-is (or nil if absent).
func (t *Tool) EffectiveRelease() *ReleaseConfig {
	if t.EffectiveMode() == ModeReleaseChecksums && isGitHubSource(t.Source) {
		merged := ReleaseConfig{
			DownloadTemplate: "{name}_{os}_{arch}",
			ChecksumTemplate: "checksums.txt",
			Extract:          "{name}",
		}
		if t.Release != nil {
			if t.Release.DownloadTemplate != "" {
				merged.DownloadTemplate = t.Release.DownloadTemplate
			}
			if t.Release.ChecksumTemplate != "" {
				merged.ChecksumTemplate = t.Release.ChecksumTemplate
			}
			if t.Release.Extract != "" {
				merged.Extract = t.Release.Extract
			}
			if t.Release.InstallToPath != nil {
				merged.InstallToPath = t.Release.InstallToPath
			}
			if t.Release.TagPrefix != nil {
				merged.TagPrefix = t.Release.TagPrefix
			}
		}
		return &merged
	}
	return t.Release
}

// TagPrefix returns the configured tag prefix for filtering releases.
// Returns "v" by default (filters to semantic version tags like v1.2.3).
// Returns empty string if explicitly configured to "" (no filtering).
// Returns the configured prefix for monorepo sub-packages (e.g. "database/v").
func (t *Tool) TagPrefix() string {
	rel := t.EffectiveRelease()
	if rel != nil && rel.TagPrefix != nil {
		return *rel.TagPrefix
	}
	return "v"
}

// isGitHubSource reports whether source refers to a GitHub repository.
// Accepts both org/repo shorthand and https://github.com/org/repo URLs.
func isGitHubSource(source string) bool {
	if strings.HasPrefix(source, "https://github.com/") || strings.HasPrefix(source, "http://github.com/") {
		return true
	}
	if strings.Contains(source, "://") {
		return false // some other URL scheme
	}
	// org/repo shorthand: require exactly one slash with non-empty parts.
	if strings.Count(source, "/") != 1 {
		return false
	}
	parts := strings.SplitN(source, "/", 2)
	return parts[0] != "" && parts[1] != ""
}

// ReleaseConfig holds URL templates for downloading tool releases.
type ReleaseConfig struct {
	TagPrefix        *string `yaml:"tag_prefix,omitempty"` // filter releases by tag prefix; defaults to "v" if nil
	DownloadTemplate string  `yaml:"download_template"`
	ChecksumTemplate string  `yaml:"checksum_template,omitempty"`
	Extract          string  `yaml:"extract"`
	InstallToPath    *bool   `yaml:"install_to_path,omitempty"` // if false, extract to /var/ci-tools/{name} and leave for hooks; defaults to true
}

// ShouldInstallToPath returns whether the tool should be installed to /usr/local/bin after extraction.
// Defaults to true; set to false for self-installing archives that hooks will handle.
func (r *ReleaseConfig) ShouldInstallToPath() bool {
	if r == nil || r.InstallToPath == nil {
		return true
	}
	return *r.InstallToPath
}

// InstallConfig specifies how to install the tool in a Dockerfile.
type InstallConfig struct {
	Method  Method `yaml:"method,omitempty"`  // defaults to MethodCurl
	Package string `yaml:"package,omitempty"` // required for MethodGoInstall; {var|modifier} template
}

// EffectiveMethod returns the install method, defaulting to MethodCurl.
func (i InstallConfig) EffectiveMethod() Method {
	if i.Method == "" {
		return MethodCurl
	}
	return i.Method
}
