package config

import "strings"

// Config is the top-level structure of deps.yaml.
type Config struct {
	Images    []Image  `yaml:"images"`
	Packages  []string `yaml:"packages"`  // zypper packages installed in every image
	Universal []Tool   `yaml:"universal"` // tools installed in every image
	Tools     []Tool   `yaml:"tools"`     // tools added by name in image.tools
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
	Mode          string         `yaml:"mode,omitempty"` // default: "pinned"
	Universal     bool           `yaml:"-"`              // set by loader; use universal: section in deps.yaml
	Checksums     ChecksumList   `yaml:"checksums,omitempty"`
	Release       *ReleaseConfig `yaml:"release,omitempty"`
	Install       InstallConfig  `yaml:"install"`
}

// EffectiveMode returns the tool's mode, defaulting to "pinned".
func (t *Tool) EffectiveMode() string {
	if t.Mode == "" {
		return "pinned"
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
	if t.EffectiveMode() == "release-checksums" && isGitHubSource(t.Source) {
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
		}
		return &merged
	}
	return t.Release
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
	DownloadTemplate string `yaml:"download_template"`
	ChecksumTemplate string `yaml:"checksum_template,omitempty"`
	Extract          string `yaml:"extract"`
}

// InstallConfig specifies how to install the tool in a Dockerfile.
type InstallConfig struct {
	Method  string `yaml:"method,omitempty"`  // "curl" | "go-install"; defaults to "curl"
	Package string `yaml:"package,omitempty"` // required for go-install; {var|modifier} template
}

// EffectiveMethod returns the install method, defaulting to "curl".
func (i InstallConfig) EffectiveMethod() string {
	if i.Method == "" {
		return "curl"
	}
	return i.Method
}
