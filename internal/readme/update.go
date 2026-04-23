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
	GoVersion   string
	Description string
}

// UpdateTable re-renders the Available Images table in readmeFile using rows,
// splicing the result between the BEGIN/END IMAGES TABLE markers.
// Uses fileutil.WriteIfChanged — no write occurs if content is identical.
func UpdateTable(readmeFile string, rows []ImageRow) error {
	raw, err := os.ReadFile(readmeFile)
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmeFile, err)
	}

	before, rest, ok := strings.Cut(string(raw), beginTag)
	if !ok {
		return fmt.Errorf("%s: missing %s marker", readmeFile, beginTag)
	}
	_, after, ok := strings.Cut(rest, endTag)
	if !ok {
		return fmt.Errorf("%s: missing %s marker", readmeFile, endTag)
	}
	// `after` starts with the \n that terminated the end-marker line; the
	// template already ends with that \n, so strip one to stay idempotent.
	after = strings.TrimPrefix(after, "\n")

	var tb strings.Builder
	if err := templates.ExecuteTemplate(&tb, "images_table.tmpl", rows); err != nil {
		return fmt.Errorf("rendering images table: %w", err)
	}

	_, err = fileutil.WriteIfChanged(readmeFile, []byte(before+tb.String()+after), 0o644)
	return err
}
