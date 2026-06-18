package lockfile

// ImagesLock is the canonical structure for images-lock.yaml.
// It records active images, universal packages, tool versions, generated scripts,
// hooks, and per-image configuration.
type ImagesLock struct {
	Images    []string               `yaml:"images"`
	Packages  []string               `yaml:"packages,omitempty"`  // universal packages installed in every image
	Tools     map[string]string      `yaml:"tools,omitempty"`     // name → version, all tools across all images
	Selectors []string               `yaml:"selectors,omitempty"` // active family selector names, e.g. ["helm"]
	Scripts   map[string]ScriptFile  `yaml:"scripts,omitempty"`   // generated selector scripts with checksums
	Hooks     map[string]HookFiles   `yaml:"hooks,omitempty"`     // tool_name → hook files with checksums
	Configs   map[string]ImageConfig `yaml:"configs"`             // per-image configuration
}

// ImageConfig holds the resolved configuration for one image.
type ImageConfig struct {
	Base            string            `yaml:"base"`
	Platforms       []string          `yaml:"platforms"`
	Packages        []string          `yaml:"packages,omitempty"`         // image-specific packages only (excludes universal)
	Tools           []string          `yaml:"tools,omitempty"`            // tool names only; versions in top-level tools map
	Aliases         map[string]string `yaml:"aliases,omitempty"`          // symlink_name: tool_name
	FamilySelectors map[string]string `yaml:"family_selectors,omitempty"` // family → default tool
	GoVersion       string            `yaml:"go_version,omitempty"`
	Description     string            `yaml:"description,omitempty"`
}

// FileMetadata is the base structure for tracking generated files with checksums.
// Used by hooks, scripts, and other generated artifacts that need version tracking.
type FileMetadata struct {
	Name     string `yaml:"name"`
	Checksum string `yaml:"checksum"` // MD5 hex
}

// Specific file types - semantically distinct but share the same structure.
type HookFile FileMetadata
type ScriptFile FileMetadata

// HookFiles holds pre and post hook files for a tool.
type HookFiles struct {
	Pre  *HookFile `yaml:"pre,omitempty"`
	Post *HookFile `yaml:"post,omitempty"`
}
