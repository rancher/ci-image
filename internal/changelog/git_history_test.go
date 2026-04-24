package changelog

// Tests using real commits from the repository's git history.
//
// FROM: 082c50719417d331cea56b03484fb5b2c4f9c01b  (PR #17 — add-git-safe-dir)
// TO:   e7f6b5c5c889aed736ef2d2da072b763217b1dc2  (PR #18 — oras-tool)
//
// The only change between these two commits in images-lock.yaml was adding
// `oras v1.3.1` to the global tools map and to the per-image tool list for
// go1.25, go1.26, and charts. The other four images (python3.11, python3.13,
// node22, node24) did not receive oras.

import (
	"strings"
	"testing"
	"time"

	"github.com/rancher/ci-image/internal/gitutil"
)

const (
	shaFrom = "082c50719417d331cea56b03484fb5b2c4f9c01b"
	shaTo   = "e7f6b5c5c889aed736ef2d2da072b763217b1dc2"
	lock    = "images-lock.yaml"
)

// skipIfCommitMissing skips the test if the given commit can't be found
// locally (e.g. shallow clone).
func skipIfCommitMissing(t *testing.T, ref string) {
	t.Helper()
	data, err := gitutil.ReadFileAtRef(ref, lock)
	if err == gitutil.ErrGitNotFound {
		t.Skip("git not available")
	}
	if data == nil {
		t.Skipf("commit %s not present in local git history", ref)
	}
}

func TestGitHistory_OrasPR_Diff(t *testing.T) {
	skipIfCommitMissing(t, shaFrom)

	prev, err := ReadFromGit(shaFrom, lock)
	if err != nil {
		t.Fatalf("ReadFromGit(from): %v", err)
	}
	next, err := ReadFromGit(shaTo, lock)
	if err != nil {
		t.Fatalf("ReadFromGit(to): %v", err)
	}

	// Sanity-check the raw lock contents before diffing.
	if _, ok := next.Tools["oras"]; !ok {
		t.Fatal("expected oras in next tools map")
	}
	if _, ok := prev.Tools["oras"]; ok {
		t.Fatal("expected oras absent from prev tools map")
	}

	c := Diff(prev, next)

	// No universal package changes.
	if len(c.PackagesAdded) != 0 || len(c.PackagesRemoved) != 0 {
		t.Errorf("unexpected universal package changes: added=%v removed=%v", c.PackagesAdded, c.PackagesRemoved)
	}

	// No images added or removed.
	if len(c.ImagesAdded) != 0 || len(c.ImagesRemoved) != 0 {
		t.Errorf("unexpected image list changes: added=%v removed=%v", c.ImagesAdded, c.ImagesRemoved)
	}

	// AllImages must be the full set of 7 images from the TO state.
	if len(c.AllImages) != 7 {
		t.Errorf("expected 7 AllImages, got %d: %v", len(c.AllImages), c.AllImages)
	}

	// Exactly go1.25, go1.26, charts received oras — the other 4 did not.
	if len(c.ImageChanges) != 3 {
		t.Fatalf("expected 3 image changes (go1.25, go1.26, charts), got %d: %v",
			len(c.ImageChanges), imageNames(c.ImageChanges))
	}

	wantChanged := map[string]bool{"go1.25": true, "go1.26": true, "charts": true}
	for _, ic := range c.ImageChanges {
		if !wantChanged[ic.Image] {
			t.Errorf("unexpected image in changes: %q", ic.Image)
		}
		if len(ic.ToolsAdded) != 1 || ic.ToolsAdded[0].Tool != "oras" || ic.ToolsAdded[0].Version != "v1.3.1" {
			t.Errorf("image %q: expected oras v1.3.1 added, got ToolsAdded=%+v", ic.Image, ic.ToolsAdded)
		}
		if len(ic.ToolsRemoved) != 0 || len(ic.ToolVersionChanged) != 0 ||
			len(ic.PackagesAdded) != 0 || len(ic.PackagesRemoved) != 0 ||
			ic.BaseImageUpdated != nil || ic.PlatformsChanged != nil {
			t.Errorf("image %q: unexpected extra changes: %+v", ic.Image, ic)
		}
	}
}

func TestGitHistory_OrasPR_RenderedEntry(t *testing.T) {
	skipIfCommitMissing(t, shaFrom)

	prev, err := ReadFromGit(shaFrom, lock)
	if err != nil {
		t.Fatalf("ReadFromGit(from): %v", err)
	}
	next, err := ReadFromGit(shaTo, lock)
	if err != nil {
		t.Fatalf("ReadFromGit(to): %v", err)
	}

	entry := Entry{
		Version: "20260424-5",
		Date:    time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		Changes: Diff(prev, next),
	}

	got := renderEntry(entry)

	// Header.
	if !strings.Contains(got, "## Revision: 20260424-5 (2026-04-24)") {
		t.Errorf("missing version header in:\n%s", got)
	}

	// The three images that got oras must have a section with the tool addition.
	for _, img := range []string{"go1.25", "go1.26", "charts"} {
		section := "### Image: " + img + ":20260424-5"
		if !strings.Contains(got, section) {
			t.Errorf("missing section %q in:\n%s", section, got)
		}
	}
	if !strings.Contains(got, "- Added: `oras` `v1.3.1`") {
		t.Errorf("missing oras addition line in:\n%s", got)
	}

	// The four images that did NOT get oras must not appear at all.
	for _, img := range []string{"python3.11", "python3.13", "node22", "node24"} {
		if strings.Contains(got, img) {
			t.Errorf("image %q should not appear in entry (no changes), got:\n%s", img, got)
		}
	}

	// No universal package section — this was a tool-only change.
	if strings.Contains(got, "Universal Packages") {
		t.Errorf("unexpected universal packages section in:\n%s", got)
	}
}

func imageNames(ics []ImageChanges) []string {
	names := make([]string, len(ics))
	for i, ic := range ics {
		names[i] = ic.Image
	}
	return names
}
