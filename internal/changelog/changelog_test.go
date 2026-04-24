package changelog

import (
	"os"
	"strings"
	"testing"
	"time"
)

var testDate = time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)

// TestPrepend_Idempotent verifies that calling Prepend twice with the same
// version does not produce a duplicate entry.
func TestPrepend_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/CHANGELOG.md"

	entry := Entry{
		Version: "20260424-5",
		Date:    testDate,
		Changes: &Changes{
			ImageChanges: []ImageChanges{
				{Image: "go1.25", ToolsAdded: []ToolChange{{Tool: "oras", Version: "v1.3.1"}}},
			},
			AllImages: []string{"go1.25"},
		},
	}

	if err := Prepend(path, entry); err != nil {
		t.Fatalf("first Prepend: %v", err)
	}
	if err := Prepend(path, entry); err != nil {
		t.Fatalf("second Prepend: %v", err)
	}

	raw, _ := os.ReadFile(path)
	content := string(raw)

	count := strings.Count(content, "## Revision: 20260424-5")
	if count != 1 {
		t.Errorf("expected version header to appear once, got %d times:\n%s", count, content)
	}
}

// TestRenderEntry_UniversalPackageOnlyShowsAllImages verifies that when only
// universal packages change (no per-image diffs), every image in AllImages
// gets a minimal section in the rendered output so readers can see which
// images were rebuilt.
func TestRenderEntry_UniversalPackageOnlyShowsAllImages(t *testing.T) {
	entry := Entry{
		Version: "20260424-1",
		Date:    testDate,
		Changes: &Changes{
			PackagesAdded: []string{"jq"},
			AllImages:     []string{"go1.25", "go1.26", "node22"},
		},
	}

	got := renderEntry(entry)

	for _, img := range []string{"go1.25", "go1.26", "node22"} {
		if !strings.Contains(got, img) {
			t.Errorf("expected image %q in rendered output, got:\n%s", img, got)
		}
	}
	if !strings.Contains(got, "Universal package changes") {
		t.Errorf("expected 'Universal package changes' note in rendered output, got:\n%s", got)
	}
}

// TestRenderEntry_UniversalPackageWithPerImageChanges verifies that images
// with their own per-image diff are not given the redundant universal-only
// section on top of their real section.
func TestRenderEntry_UniversalPackageWithPerImageChanges(t *testing.T) {
	entry := Entry{
		Version: "20260424-2",
		Date:    testDate,
		Changes: &Changes{
			PackagesAdded: []string{"jq"},
			ImageChanges: []ImageChanges{
				{
					Image:         "go1.25",
					PackagesAdded: []string{"skopeo"},
				},
			},
			AllImages: []string{"go1.25", "go1.26"},
		},
	}

	got := renderEntry(entry)

	// go1.25 has real changes — should appear once, not duplicated
	count := strings.Count(got, "go1.25")
	if count != 1 {
		t.Errorf("expected go1.25 to appear once, got %d occurrences in:\n%s", count, got)
	}

	// go1.26 has no per-image changes — should get the universal note
	if !strings.Contains(got, "go1.26") {
		t.Errorf("expected go1.26 in rendered output, got:\n%s", got)
	}
}

// TestRenderEntry_NoUniversalChanges verifies that the universal-only image
// sections are NOT added when there are no universal package changes.
func TestRenderEntry_NoUniversalChanges(t *testing.T) {
	entry := Entry{
		Version: "20260424-3",
		Date:    testDate,
		Changes: &Changes{
			ImageChanges: []ImageChanges{
				{Image: "go1.25", PackagesAdded: []string{"skopeo"}},
			},
			AllImages: []string{"go1.25", "go1.26"},
		},
	}

	got := renderEntry(entry)

	// go1.26 has no changes and no universal packages changed — must not appear
	if strings.Contains(got, "go1.26") {
		t.Errorf("expected go1.26 absent when no universal changes, got:\n%s", got)
	}
	if strings.Contains(got, "Universal package changes") {
		t.Errorf("unexpected 'Universal package changes' note when no universal changes, got:\n%s", got)
	}
}
