package updater

import (
	"os"
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config"
)

func realDeps(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../deps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hex64(c string) string { return strings.Repeat(c, 64) }

func TestApplyTool_RealDepsYAML(t *testing.T) {
	data := realDeps(t)
	edits := []ToolEdit{
		{Name: "golangci-lint", Version: "v9.9.9", Checksums: map[string]string{"linux/amd64": hex64("a"), "linux/arm64": hex64("b")}},
		// nix is both an image name and a tool name; only the tool may change.
		{Name: "nix", Version: "9.9.9", Checksums: map[string]string{"linux/amd64": hex64("c"), "linux/arm64": hex64("d")}},
		{Name: "govulncheck", Version: "v9.0.0", VersionCommit: strings.Repeat("e", 40)},
	}
	out, err := Apply(data, edits, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(out, edits, nil); err != nil {
		t.Fatal(err)
	}

	// Untouched content (comments, other tools) is byte-for-byte identical.
	oldLines, newLines := strings.Split(string(data), "\n"), strings.Split(string(out), "\n")
	if len(oldLines) != len(newLines) {
		t.Fatalf("line count changed: %d → %d", len(oldLines), len(newLines))
	}
	changed := 0
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			changed++
		}
	}
	// golangci-lint: version + 2 sums; nix: version + 2 sums; govulncheck: version + commit.
	if changed != 8 {
		t.Fatalf("changed %d lines, want 8", changed)
	}
	if !strings.Contains(string(out), "# charts-build-scripts is a first-party Rancher tool") {
		t.Error("comment lost")
	}

	cfg, err := config.Parse(out, "x")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolByName("kubectl1.36").Version != "v1.36.3" {
		t.Error("unrelated tool changed")
	}
}

func TestApplyTool_PreservesQuotingAndTrailingComment(t *testing.T) {
	in := `images:
  - name: a
    base: "registry.suse.com/bci/golang:1.2.3@sha256:` + hex64("1") + `"
    tools:
      - t
tools:
  - name: t
    source: o/r
    version: v1.0.0 # keep me
    checksums:
      linux/amd64: "` + hex64("2") + `"
      linux/arm64: ` + hex64("3") + `
    release:
      download_template: "t_{os}_{arch}"
`
	out, err := Apply([]byte(in), []ToolEdit{{Name: "t", Version: "v1.0.1", Checksums: map[string]string{"linux/amd64": hex64("4"), "linux/arm64": hex64("5")}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`version: v1.0.1 # keep me`, `linux/amd64: "` + hex64("4") + `"`, `linux/arm64: ` + hex64("5") + "\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestApplyTool_Errors(t *testing.T) {
	data := realDeps(t)
	cases := map[string]ToolEdit{
		"unknown tool":      {Name: "nope", Version: "v1"},
		"missing platform":  {Name: "yq", Version: "v1", Checksums: map[string]string{"linux/s390x": hex64("a")}},
		"no commit line":    {Name: "yq", VersionCommit: strings.Repeat("a", 40)},
		"image-only name":   {Name: "go1.25", Version: "v1"},
		"no checksums blok": {Name: "govulncheck", Checksums: map[string]string{"linux/amd64": hex64("a")}},
	}
	for name, e := range cases {
		if _, err := Apply(data, []ToolEdit{e}, nil); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestVerifyCatchesPartialChecksumEdit(t *testing.T) {
	data := realDeps(t)
	// Only one of the two platforms supplied: edit succeeds line-wise, but the
	// re-read config must not match a two-platform proposal.
	edit := ToolEdit{Name: "yq", Version: "v9.9.9", Checksums: map[string]string{"linux/amd64": hex64("a")}}
	out, err := Apply(data, []ToolEdit{edit}, nil)
	if err != nil {
		t.Fatal(err)
	}
	full := edit
	full.Checksums = map[string]string{"linux/amd64": hex64("a"), "linux/arm64": hex64("b")}
	if err := Verify(out, []ToolEdit{full}, nil); err == nil {
		t.Fatal("Verify should reject a stale arm64 checksum")
	}
}

func TestApplyBase_AllImagesSharingIt(t *testing.T) {
	data := realDeps(t)
	cfg, _ := config.Parse(data, "x")
	var old string
	for _, img := range cfg.Images {
		if img.Name == "charts" {
			old = img.Base
		}
	}
	nw := old[:len(old)-4] + "abcd" // same length, different digest
	edits := []BaseEdit{{Old: old, New: nw}}
	out, err := Apply(data, nil, edits)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(out, nil, edits); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), nw); n != 2 { // charts and nix
		t.Errorf("rewrote %d bases, want 2", n)
	}
	if _, err := Apply(data, nil, []BaseEdit{{Old: "registry.suse.com/bci/none:1", New: "x"}}); err == nil {
		t.Error("expected error for unknown base")
	}
}
