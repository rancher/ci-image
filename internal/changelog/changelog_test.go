package changelog

import (
	"strings"
	"testing"
	"time"
)

var testDate = time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)

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
