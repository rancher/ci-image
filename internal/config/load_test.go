package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeYAML writes content to a temp file and returns the path.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deps.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	sha256A = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 64 hex chars
	sha256B = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sha256C = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	sha256D = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	sha256E = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

func TestLoad_AliasAutoInclude_TargetAddedWhenMissing(t *testing.T) {
	// helm is referenced only via an alias — not listed in image.tools.
	// The loader should auto-include it so validation passes and generation works.
	path := writeYAML(t, `
images:
  - name: test
    base: "base@sha256:`+sha256A+`"
    packages: [wget]
    aliases:
      helm_v3: helm
tools:
  - name: helm
    source: "https://get.helm.sh"
    mode: static
    version: v3.20.0
    checksums:
      linux/amd64: "`+sha256B+`"
      linux/arm64: "`+sha256C+`"
    release:
      download_template: "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz"
      extract: "{os}-{arch}/helm"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !slices.Contains(cfg.Images[0].Tools, "helm") {
		t.Errorf("Load() did not auto-include alias target 'helm' into image.Tools: %v", cfg.Images[0].Tools)
	}
}

func TestLoad_AliasAutoInclude_NoDuplicationWhenAlreadyListed(t *testing.T) {
	// helm is both in image.tools and an alias target — should not be duplicated.
	path := writeYAML(t, `
images:
  - name: test
    base: "base@sha256:`+sha256A+`"
    packages: [wget]
    tools: [helm]
    aliases:
      helm_v3: helm
tools:
  - name: helm
    source: "https://get.helm.sh"
    mode: static
    version: v3.20.0
    checksums:
      linux/amd64: "`+sha256B+`"
      linux/arm64: "`+sha256C+`"
    release:
      download_template: "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz"
      extract: "{os}-{arch}/helm"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	count := 0
	for _, name := range cfg.Images[0].Tools {
		if name == "helm" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Load() duplicated alias target 'helm': found %d times in %v", count, cfg.Images[0].Tools)
	}
}

func TestLoad_AliasAutoInclude_MultipleTargetsInConfigOrder(t *testing.T) {
	// Two alias targets (helm, oras) not in image.tools.
	// Auto-included tools should appear in cfg.Tools order (helm before oras).
	path := writeYAML(t, `
images:
  - name: test
    base: "base@sha256:`+sha256A+`"
    packages: [wget]
    aliases:
      helm_v3: helm
      oras2: oras
tools:
  - name: helm
    source: "https://get.helm.sh"
    mode: static
    version: v3.20.0
    checksums:
      linux/amd64: "`+sha256B+`"
      linux/arm64: "`+sha256C+`"
    release:
      download_template: "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz"
      extract: "{os}-{arch}/helm"
  - name: oras
    source: oras-project/oras
    version: v1.3.1
    checksums:
      linux/amd64: "`+sha256D+`"
      linux/arm64: "`+sha256E+`"
    release:
      download_template: "oras_{version|trimprefix:v}_{os}_{arch}.tar.gz"
      extract: "oras"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	tools := cfg.Images[0].Tools
	helmIdx := slices.Index(tools, "helm")
	orasIdx := slices.Index(tools, "oras")

	if helmIdx == -1 {
		t.Fatalf("Load() did not auto-include 'helm': %v", tools)
	}
	if orasIdx == -1 {
		t.Fatalf("Load() did not auto-include 'oras': %v", tools)
	}
	// helm is declared before oras in tools:, so auto-include must preserve that order.
	if helmIdx > orasIdx {
		t.Errorf("auto-included tools in wrong order: helm(%d) should precede oras(%d) in %v",
			helmIdx, orasIdx, tools)
	}
}

func TestLoad_AliasAutoInclude_UniversalTargetNotDuplicated(t *testing.T) {
	// An alias targeting a universal tool should not add it to image.Tools
	// (universal tools are always present; adding them to image.Tools would fail validation).
	path := writeYAML(t, `
images:
  - name: test
    base: "base@sha256:`+sha256A+`"
    packages: [wget]
    aliases:
      cosign2: cosign
universal:
  - name: cosign
    source: sigstore/cosign
    version: v3.0.6
    checksums:
      linux/amd64: "`+sha256B+`"
      linux/arm64: "`+sha256C+`"
    release:
      download_template: "cosign-{os}-{arch}"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	for _, name := range cfg.Images[0].Tools {
		if name == "cosign" {
			t.Errorf("Load() incorrectly added universal tool 'cosign' to image.Tools: %v", cfg.Images[0].Tools)
		}
	}
}

func TestLoad_AliasAutoInclude_UndefinedTargetPassesToValidation(t *testing.T) {
	// An alias targeting a tool that doesn't exist should produce a validation error,
	// not a panic or silent failure.
	path := writeYAML(t, `
images:
  - name: test
    base: "base@sha256:`+sha256A+`"
    packages: [wget]
    aliases:
      x: nonexistent-tool
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for undefined alias target, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-tool") {
		t.Errorf("Load() error should mention the undefined tool, got: %v", err)
	}
}
