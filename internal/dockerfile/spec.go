package dockerfile

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/rancher/ci-image/internal/config"
)

//go:embed tmpl
var templateFS embed.FS

var templates = template.Must(
	template.New("").Funcs(template.FuncMap{
		"extractCmd": archiveExtractCmd,
	}).ParseFS(templateFS, "tmpl/*.tmpl"),
)

// loadHookTemplates loads optional tool hook templates from the hooks/ directory.
// Called during Dockerfile generation; returns a new template set with hooks added.
// If hooks/ directory doesn't exist, returns the base template set unchanged.
func loadHookTemplates() (*template.Template, error) {
	// Clone base templates so we don't modify the global set
	t := template.Must(templates.Clone())

	hooksDir := "hooks"
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		return t, nil // No hooks directory - return base templates
	}

	// Find all pre-hook templates
	preMatches, err := filepath.Glob(filepath.Join(hooksDir, "*-pre.tmpl"))
	if err != nil {
		return nil, err
	}

	// Find all post-hook templates
	postMatches, err := filepath.Glob(filepath.Join(hooksDir, "*-post.tmpl"))
	if err != nil {
		return nil, err
	}

	// Combine all hook templates
	matches := append(preMatches, postMatches...)

	// If we found hook templates, parse them
	if len(matches) > 0 {
		t, err = t.ParseFiles(matches...)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

// executeTemplate renders a named template against data and returns the result.
// Panics on error: templates are static and data is validated; any failure is a programmer bug.
func executeTemplate(name string, data any) string {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic("dockerfile: executing " + name + ": " + err.Error())
	}
	return strings.TrimRight(b.String(), "\n")
}

// Renderer is anything that can produce its own Dockerfile snippet.
type Renderer interface {
	Render() string
}

// ItemInstall is the interface for a resolved, renderable install method.
// It is satisfied by CurlInstall and GoInstall.
type ItemInstall interface {
	Renderer
	Method() string // "curl", "go-install", etc.
}

// PlatformInstall holds the fully-resolved data for one platform of a curl tool.
type PlatformInstall struct {
	Arch        string
	DownloadURL string // fully rendered
	Extract     string // fully rendered; empty if not an archive
	Checksum    string // SHA-256 hex
}

// CurlInstall is the resolved spec for a curl-installed tool.
// Implements ItemInstall.
type CurlInstall struct {
	Name          string            // tool name; used in shell commands
	Format        Format            // "archive" | "gzip" | "binary"
	ArchiveExt    string            // ".tar.gz", ".zip", etc.; empty unless Format == "archive"
	Platforms     []PlatformInstall // one entry per platform, sorted by Arch
	InstallToPath bool              // if true, install to /usr/local/bin; if false, extract to /var/ci-tools/{name} for hooks
}

func (c CurlInstall) Method() string { return "curl" }

func (c CurlInstall) Render() string {
	suffix := ""
	if c.Format == FormatArchive && !c.InstallToPath {
		suffix = "_extract"
	}
	return executeTemplate("curl_"+c.Format.String()+suffix+".tmpl", c)
}

// GoInstall is the resolved spec for a go-install tool.
// Implements ItemInstall.
type GoInstall struct {
	Package string // fully rendered go package path
}

func (g GoInstall) Method() string { return "go-install" }
func (g GoInstall) Render() string { return "RUN go install " + g.Package }

// ToolSetup describes optional setup steps that run before/after the main install.
// If template names are set, those templates are rendered; otherwise the phase is skipped.
type ToolSetup struct {
	Name         string             // tool name for comment generation
	PreTemplate  string             // optional template name for pre-install steps (e.g., "nix-pre.tmpl")
	PostTemplate string             // optional template name for post-install steps (e.g., "nix-post.tmpl")
	templates    *template.Template // template set with hooks loaded
}

// RenderPre renders the pre-install template if present.
// Returns the hook content with a blank line, comment header, and trailing blank line.
func (s *ToolSetup) RenderPre() string {
	if s == nil || s.PreTemplate == "" {
		return ""
	}
	var b strings.Builder
	if err := s.templates.ExecuteTemplate(&b, s.PreTemplate, nil); err != nil {
		panic("dockerfile: executing " + s.PreTemplate + ": " + err.Error())
	}
	content := strings.TrimRight(b.String(), "\n")
	return "\n# Pre-install setup for " + s.Name + "\n" + content + "\n\n"
}

// RenderPost renders the post-install template if present.
// Returns the hook content with leading blank line and comment header.
func (s *ToolSetup) RenderPost() string {
	if s == nil || s.PostTemplate == "" {
		return ""
	}
	var b strings.Builder
	if err := s.templates.ExecuteTemplate(&b, s.PostTemplate, nil); err != nil {
		panic("dockerfile: executing " + s.PostTemplate + ": " + err.Error())
	}
	content := strings.TrimRight(b.String(), "\n")
	return "\n\n# Post-install setup for " + s.Name + "\n" + content
}

// ToolInstall is one resolved tool entry in a Dockerfile.
type ToolInstall struct {
	Name    string
	Version string
	Install ItemInstall // CurlInstall or GoInstall
	Setup   *ToolSetup  // optional pre/post hooks; nil if no hooks
}

// AliasInstall describes a symlink to create in /usr/local/bin after tools are installed.
type AliasInstall struct {
	Name   string // symlink name  (ln -sf /usr/local/bin/Target /usr/local/bin/Name)
	Target string // target binary name
}

// SelectorInstall describes a family selector that is active for an image.
// At image build time this creates the manifest and default active symlink;
// the runner can later call 'ci-select {Family} <tool>' to change it.
type SelectorInstall struct {
	Family      string   // family name, e.g. "helm"
	DefaultTool string   // tool name that is active by default, e.g. "helmv4"
	ValidTools  []string // all tool names in the family, sorted
}

// DockerfileVars is the fully-resolved spec for one image's Dockerfile.
// Once constructed, Render() cannot fail — all template rendering and
// checksum resolution has already been performed.
// Implements Renderer.
type DockerfileVars struct {
	Base        string
	Packages    []string
	Tools       []ToolInstall
	Selectors   []SelectorInstall // family selectors active in this image; sorted by Family
	Aliases     []AliasInstall    // sorted by Name for determinism
	SourceURL   string            // org.opencontainers.image.source
	Title       string            // org.opencontainers.image.title
	Description string            // org.opencontainers.image.description; empty → no label emitted
}

// SelectorSetupCmd renders the shell command string for the family selector
// infrastructure RUN block. Returns "" if there are no selectors.
// Called by dockerfile.tmpl.
func (v DockerfileVars) SelectorSetupCmd() string {
	if len(v.Selectors) == 0 {
		return ""
	}
	return executeTemplate("selector_setup.tmpl", v.Selectors)
}

// HasGoInstall reports whether any tool in the image uses go-install.
// Called by dockerfile.tmpl to conditionally emit the Go cache cleanup block.
func (v DockerfileVars) HasGoInstall() bool {
	for _, t := range v.Tools {
		if t.Install.Method() == string(config.MethodGoInstall) {
			return true
		}
	}
	return false
}

// HasAnyOfPackages reports whether any of the given packages are in the image.
func (v DockerfileVars) HasAnyOfPackages(pkgs ...string) bool {
	for _, p := range pkgs {
		if slices.Contains(v.Packages, p) {
			return true
		}
	}
	return false
}

func (v DockerfileVars) Render() string {
	return executeTemplate("dockerfile.tmpl", v) + "\n"
}
