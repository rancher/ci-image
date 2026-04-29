package changelog

// ImagesLock mirrors the structure of images-lock.yaml for diffing purposes.
// It is intentionally separate from the unexported types in internal/cli to
// avoid an import cycle.
type ImagesLock struct {
	Images    []string               `yaml:"images"`
	Packages  []string               `yaml:"packages,omitempty"` // universal packages installed in every image
	Tools     map[string]string      `yaml:"tools,omitempty"`
	Selectors []string               `yaml:"selectors,omitempty"` // active family selector names, e.g. ["helm"]
	Configs   map[string]ImageConfig `yaml:"configs"`
}

// ImageConfig holds the resolved configuration for one image.
type ImageConfig struct {
	Base            string            `yaml:"base"`
	Platforms       []string          `yaml:"platforms"`
	Packages        []string          `yaml:"packages,omitempty"`         // image-specific packages only (excludes universal)
	Tools           []string          `yaml:"tools,omitempty"`
	Aliases         map[string]string `yaml:"aliases,omitempty"`          // symlink_name: tool_name
	FamilySelectors map[string]string `yaml:"family_selectors,omitempty"` // family → default tool
	GoVersion       string            `yaml:"go_version,omitempty"`
	Description     string            `yaml:"description,omitempty"`
}

// Changes summarises what changed between two ImagesLock states.
type Changes struct {
	// Universal package changes affect every image.
	PackagesAdded   []string
	PackagesRemoved []string
	// Family selector changes (global — a selector was introduced or removed).
	SelectorsAdded   []SelectorChange
	SelectorsRemoved []SelectorChange
	// ImageChanges holds per-image diffs (only images with at least one change).
	ImageChanges []ImageChanges
	// ImagesAdded and ImagesRemoved track images that appeared or disappeared.
	ImagesAdded   []string
	ImagesRemoved []string
	// AllImages is the full list of images in the "to" state. Used by the
	// changelog renderer to list images that were rebuilt due to universal
	// package changes but have no per-image diff of their own.
	AllImages []string
	// DockerfileChanges lists images rebuilt solely due to Dockerfile template
	// changes (internal/dockerfile/ or dockerfiles/) with no images-lock.yaml diff.
	DockerfileChanges []string
}

// IsEmpty returns true when there are no changes at all.
func (c *Changes) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.PackagesAdded) == 0 && len(c.PackagesRemoved) == 0 &&
		len(c.SelectorsAdded) == 0 && len(c.SelectorsRemoved) == 0 &&
		len(c.ImageChanges) == 0 && len(c.ImagesAdded) == 0 && len(c.ImagesRemoved) == 0 &&
		len(c.DockerfileChanges) == 0
}

// AffectedImages returns the names of images that have per-image changes.
// Does NOT include images affected only by universal package changes — callers
// that need those should check PackagesAdded/PackagesRemoved separately.
func (c *Changes) AffectedImages() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.ImageChanges))
	for _, ic := range c.ImageChanges {
		names = append(names, ic.Image)
	}
	return names
}

// ImageChanges holds all the changes for a single image.
type ImageChanges struct {
	Image                 string
	BaseImageUpdated      *BaseImageChange
	PlatformsChanged      *PlatformsChange
	PackagesAdded         []string
	PackagesRemoved       []string
	ToolVersionChanged    []ToolVersionChange
	ToolsAdded            []ToolChange
	ToolsRemoved          []ToolChange
	AliasesAdded          []AliasChange
	AliasesRemoved        []AliasChange
	SelectorDefaultChanged []SelectorDefaultChange // family selector default tool changed
}

// HasChanges returns true if the image has any changes.
func (ic ImageChanges) HasChanges() bool {
	return ic.BaseImageUpdated != nil ||
		ic.PlatformsChanged != nil ||
		len(ic.PackagesAdded) > 0 || len(ic.PackagesRemoved) > 0 ||
		len(ic.ToolVersionChanged) > 0 ||
		len(ic.ToolsAdded) > 0 || len(ic.ToolsRemoved) > 0 ||
		len(ic.AliasesAdded) > 0 || len(ic.AliasesRemoved) > 0 ||
		len(ic.SelectorDefaultChanged) > 0
}

// SelectorChange records a family selector being introduced or removed globally.
type SelectorChange struct {
	Family      string
	DefaultTool string // populated for additions; empty for removals
}

// SelectorDefaultChange records the default tool for a family selector changing
// in a specific image.
type SelectorDefaultChange struct {
	Family string
	From   string
	To     string
}

// AliasChange records a symlink alias being added or removed.
type AliasChange struct {
	Name   string // symlink name
	Target string // target tool
}

// PlatformsChange records a change to the set of target platforms.
type PlatformsChange struct {
	From []string
	To   []string
}

// BaseImageChange records a base image reference change.
type BaseImageChange struct {
	From string
	To   string
}

// ToolVersionChange records a tool version bump.
type ToolVersionChange struct {
	Tool string
	From string
	To   string
}

// ToolChange records a tool being added or removed.
type ToolChange struct {
	Tool    string
	Version string
}
