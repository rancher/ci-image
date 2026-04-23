package changelog

import (
	"os"
	"path/filepath"
	"testing"
)

// --- ReadFromGit (filesystem, ref="") ---

func TestReadFromGit_FilesystemPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images-lock.yaml")
	content := "images:\n- foo\nconfigs:\n  foo:\n    base: alpine\n    platforms: [linux/amd64]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lk, err := ReadFromGit("", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lk == nil || len(lk.Images) != 1 || lk.Images[0] != "foo" {
		t.Errorf("unexpected lock: %+v", lk)
	}
}

func TestReadFromGit_FilesystemMissing(t *testing.T) {
	lk, err := ReadFromGit("", "/nonexistent/path/images-lock.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if lk != nil {
		t.Fatalf("expected nil ImagesLock for missing file, got: %+v", lk)
	}
}

func TestReadFromGit_FilesystemInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images-lock.yaml")
	if err := os.WriteFile(path, []byte("{bad yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFromGit("", path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- Diff ---

func TestDiff_NilNext(t *testing.T) {
	c := Diff(&ImagesLock{Images: []string{"a"}}, nil)
	if !c.IsEmpty() {
		t.Errorf("expected empty Changes when next is nil, got %+v", c)
	}
}

func TestDiff_NilPrev_FirstRun(t *testing.T) {
	next := &ImagesLock{Images: []string{"a", "b"}}
	c := Diff(nil, next)
	if c.IsEmpty() {
		t.Fatal("expected non-empty Changes for first run")
	}
	if len(c.ImagesAdded) != 2 {
		t.Errorf("expected 2 images added, got %v", c.ImagesAdded)
	}
	if len(c.ImagesRemoved) != 0 || len(c.ImageChanges) != 0 || len(c.PackagesAdded) != 0 {
		t.Errorf("unexpected fields set: %+v", c)
	}
}

func TestDiff_NilPrevNilNext(t *testing.T) {
	c := Diff(nil, nil)
	if !c.IsEmpty() {
		t.Errorf("expected empty Changes for both nil, got %+v", c)
	}
}

func TestDiff_ImagesAdded(t *testing.T) {
	prev := &ImagesLock{Images: []string{"a"}, Configs: map[string]ImageConfig{"a": {Base: "alpine"}}}
	next := &ImagesLock{Images: []string{"a", "b"}, Configs: map[string]ImageConfig{
		"a": {Base: "alpine"},
		"b": {Base: "ubuntu"},
	}}

	c := Diff(prev, next)
	if len(c.ImagesAdded) != 1 || c.ImagesAdded[0] != "b" {
		t.Errorf("expected [b] added, got %v", c.ImagesAdded)
	}
	if len(c.ImagesRemoved) != 0 {
		t.Errorf("unexpected removals: %v", c.ImagesRemoved)
	}
}

func TestDiff_ImagesRemoved(t *testing.T) {
	prev := &ImagesLock{Images: []string{"a", "b"}, Configs: map[string]ImageConfig{
		"a": {Base: "alpine"},
		"b": {Base: "ubuntu"},
	}}
	next := &ImagesLock{Images: []string{"a"}, Configs: map[string]ImageConfig{"a": {Base: "alpine"}}}

	c := Diff(prev, next)
	if len(c.ImagesRemoved) != 1 || c.ImagesRemoved[0] != "b" {
		t.Errorf("expected [b] removed, got %v", c.ImagesRemoved)
	}
	if len(c.ImagesAdded) != 0 {
		t.Errorf("unexpected additions: %v", c.ImagesAdded)
	}
}

func TestDiff_UniversalPackagesAddedRemoved(t *testing.T) {
	prev := &ImagesLock{Packages: []string{"curl", "wget"}, Configs: map[string]ImageConfig{}}
	next := &ImagesLock{Packages: []string{"curl", "jq"}, Configs: map[string]ImageConfig{}}

	c := Diff(prev, next)
	if len(c.PackagesAdded) != 1 || c.PackagesAdded[0] != "jq" {
		t.Errorf("expected [jq] added, got %v", c.PackagesAdded)
	}
	if len(c.PackagesRemoved) != 1 || c.PackagesRemoved[0] != "wget" {
		t.Errorf("expected [wget] removed, got %v", c.PackagesRemoved)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	lock := &ImagesLock{
		Images:   []string{"img"},
		Packages: []string{"curl"},
		Tools:    map[string]string{"helm": "3.0.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Platforms: []string{"linux/amd64"}, Tools: []string{"helm"}},
		},
	}
	c := Diff(lock, lock)
	if !c.IsEmpty() {
		t.Errorf("expected no changes when prev==next, got %+v", c)
	}
}

// --- computeImageChanges (exercised via Diff) ---

func TestDiff_ImageBaseChange(t *testing.T) {
	prev := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine:3.18", Platforms: []string{"linux/amd64"}}},
	}
	next := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine:3.19", Platforms: []string{"linux/amd64"}}},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	ic := c.ImageChanges[0]
	if ic.BaseImageUpdated == nil {
		t.Fatal("expected BaseImageUpdated to be set")
	}
	if ic.BaseImageUpdated.From != "alpine:3.18" || ic.BaseImageUpdated.To != "alpine:3.19" {
		t.Errorf("unexpected base change: %+v", ic.BaseImageUpdated)
	}
}

func TestDiff_ImagePlatformsChange(t *testing.T) {
	prev := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine", Platforms: []string{"linux/amd64"}}},
	}
	next := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine", Platforms: []string{"linux/amd64", "linux/arm64"}}},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	if c.ImageChanges[0].PlatformsChanged == nil {
		t.Fatal("expected PlatformsChanged to be set")
	}
}

func TestDiff_ImagePackagesAddedRemoved(t *testing.T) {
	prev := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine", Packages: []string{"curl", "wget"}}},
	}
	next := &ImagesLock{
		Images:  []string{"img"},
		Configs: map[string]ImageConfig{"img": {Base: "alpine", Packages: []string{"curl", "jq"}}},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	ic := c.ImageChanges[0]
	if len(ic.PackagesAdded) != 1 || ic.PackagesAdded[0] != "jq" {
		t.Errorf("expected [jq] added, got %v", ic.PackagesAdded)
	}
	if len(ic.PackagesRemoved) != 1 || ic.PackagesRemoved[0] != "wget" {
		t.Errorf("expected [wget] removed, got %v", ic.PackagesRemoved)
	}
}

func TestDiff_ImageToolAdded(t *testing.T) {
	prev := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.0.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm"}},
		},
	}
	next := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.0.0", "kubectl": "1.28.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm", "kubectl"}},
		},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	ic := c.ImageChanges[0]
	if len(ic.ToolsAdded) != 1 || ic.ToolsAdded[0].Tool != "kubectl" || ic.ToolsAdded[0].Version != "1.28.0" {
		t.Errorf("unexpected ToolsAdded: %+v", ic.ToolsAdded)
	}
}

func TestDiff_ImageToolRemoved(t *testing.T) {
	prev := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.0.0", "kubectl": "1.28.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm", "kubectl"}},
		},
	}
	next := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.0.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm"}},
		},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	ic := c.ImageChanges[0]
	if len(ic.ToolsRemoved) != 1 || ic.ToolsRemoved[0].Tool != "kubectl" || ic.ToolsRemoved[0].Version != "1.28.0" {
		t.Errorf("unexpected ToolsRemoved: %+v", ic.ToolsRemoved)
	}
}

func TestDiff_ImageToolVersionChanged(t *testing.T) {
	prev := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.0.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm"}},
		},
	}
	next := &ImagesLock{
		Images: []string{"img"},
		Tools:  map[string]string{"helm": "3.1.0"},
		Configs: map[string]ImageConfig{
			"img": {Base: "alpine", Tools: []string{"helm"}},
		},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Fatalf("expected 1 image change, got %d", len(c.ImageChanges))
	}
	ic := c.ImageChanges[0]
	if len(ic.ToolVersionChanged) != 1 {
		t.Fatalf("expected 1 tool version change, got %d", len(ic.ToolVersionChanged))
	}
	tvc := ic.ToolVersionChanged[0]
	if tvc.Tool != "helm" || tvc.From != "3.0.0" || tvc.To != "3.1.0" {
		t.Errorf("unexpected tool version change: %+v", tvc)
	}
	if len(ic.ToolsAdded) != 0 || len(ic.ToolsRemoved) != 0 {
		t.Errorf("tool version change should not appear in add/remove lists: added=%v removed=%v", ic.ToolsAdded, ic.ToolsRemoved)
	}
}

func TestDiff_OnlyChangedImageAppears(t *testing.T) {
	prev := &ImagesLock{
		Images: []string{"a", "b"},
		Configs: map[string]ImageConfig{
			"a": {Base: "alpine:3.18"},
			"b": {Base: "ubuntu:22.04"},
		},
	}
	next := &ImagesLock{
		Images: []string{"a", "b"},
		Configs: map[string]ImageConfig{
			"a": {Base: "alpine:3.19"},  // changed
			"b": {Base: "ubuntu:22.04"}, // unchanged
		},
	}

	c := Diff(prev, next)
	if len(c.ImageChanges) != 1 {
		t.Errorf("expected only 1 image change (for 'a'), got %d", len(c.ImageChanges))
	}
	if c.ImageChanges[0].Image != "a" {
		t.Errorf("expected change for image 'a', got %q", c.ImageChanges[0].Image)
	}
}
