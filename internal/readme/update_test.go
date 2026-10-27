package readme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateTable_RendersRowsBetweenMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("intro\n"+beginTag+"\nold\n"+endTag+"\noutro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []ImageRow{
		{Name: "go1.26", GoVersion: "1.26.10", Description: "Go"},
		{Name: "charts", Description: "Charts"},
	}
	if err := UpdateTable(path, rows); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	got := string(raw)
	for _, want := range []string{
		"| Image | Go Version | Description |",
		"| `go1.26` | 1.26.10 | Go |",
		"| `charts` | none | Charts |",
		"intro\n", "outro\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "old") {
		t.Errorf("stale content kept:\n%s", got)
	}
}
