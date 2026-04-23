package dockerfile

import (
	"embed"
	"strings"
	"text/template"
)

//go:embed tmpl
var templateFS embed.FS

var templates = template.Must(
	template.New("").Funcs(template.FuncMap{
		"extractCmd": archiveExtractCmd,
	}).ParseFS(templateFS, "tmpl/*.tmpl"),
)

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
	Name       string            // tool name; used in shell commands
	Format     string            // "archive" | "gzip" | "binary"
	ArchiveExt string            // ".tar.gz", ".zip", etc.; empty unless Format == "archive"
	Platforms  []PlatformInstall // one entry per platform, sorted by Arch
}

func (c CurlInstall) Method() string { return "curl" }

func (c CurlInstall) Render() string {
	return executeTemplate("curl_"+c.Format+".tmpl", c)
}

// GoInstall is the resolved spec for a go-install tool.
// Implements ItemInstall.
type GoInstall struct {
	Package string // fully rendered go package path
}

func (g GoInstall) Method() string { return "go-install" }
func (g GoInstall) Render() string { return "RUN go install " + g.Package }

// ToolInstall is one resolved tool entry in a Dockerfile.
type ToolInstall struct {
	Name    string
	Version string
	Install ItemInstall // CurlInstall or GoInstall
}

// DockerfileVars is the fully-resolved spec for one image's Dockerfile.
// Once constructed, Render() cannot fail — all template rendering and
// checksum resolution has already been performed.
// Implements Renderer.
type DockerfileVars struct {
	Base        string
	Packages    []string
	Tools       []ToolInstall
	SourceURL   string // org.opencontainers.image.source
	Title       string // org.opencontainers.image.title
	Description string // org.opencontainers.image.description; empty → no label emitted
}

// HasGoInstall reports whether any tool in the image uses go-install.
// Called by dockerfile.tmpl to conditionally emit the Go cache cleanup block.
func (v DockerfileVars) HasGoInstall() bool {
	for _, t := range v.Tools {
		if t.Install.Method() == "go-install" {
			return true
		}
	}
	return false
}

func (v DockerfileVars) Render() string {
	return executeTemplate("dockerfile.tmpl", v) + "\n"
}
