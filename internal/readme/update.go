package readme

import (
	"embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/rancher/ci-image/internal/fileutil"
)

//go:embed tmpl
var templateFS embed.FS

var templates = template.Must(
	template.New("").ParseFS(templateFS, "tmpl/*.tmpl"),
)

const (
	beginTag = "<!-- BEGIN IMAGES TABLE -->"
	endTag   = "<!-- END IMAGES TABLE -->"
)

// ImageRow is one row of data for the Available Images table.
type ImageRow struct {
	Name        string
	Language    string // empty for images that don't ship a language
	Version     string // full language version; empty when unknown
	Description string
}

// UpdateTable re-renders the Available Images table in readmeFile using rows,
// splicing the result between the BEGIN/END IMAGES TABLE markers.
// Uses fileutil.WriteIfChanged — no write occurs if content is identical.
// changed reports whether the file was rewritten.
func UpdateTable(readmeFile string, rows []ImageRow) (changed bool, err error) {
	raw, err := os.ReadFile(readmeFile)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", readmeFile, err)
	}

	before, rest, ok := strings.Cut(string(raw), beginTag)
	if !ok {
		return false, fmt.Errorf("%s: missing %s marker", readmeFile, beginTag)
	}
	_, after, ok := strings.Cut(rest, endTag)
	if !ok {
		return false, fmt.Errorf("%s: missing %s marker", readmeFile, endTag)
	}
	// `after` starts with the \n that terminated the end-marker line; the
	// template already ends with that \n, so strip one to stay idempotent.
	after = strings.TrimPrefix(after, "\n")

	var tb strings.Builder
	if err := templates.ExecuteTemplate(&tb, "images_table.tmpl", rows); err != nil {
		return false, fmt.Errorf("rendering images table: %w", err)
	}

	_, err = fileutil.WriteIfChanged(readmeFile, []byte(before+tb.String()+after), 0o644)
	return false, err
}
