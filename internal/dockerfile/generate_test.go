package dockerfile

import (
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config"
)

func TestZypperBlock(t *testing.T) {
	packages := []string{"cosign", "gawk", "git-core", "jq", "wget"}
	got := executeTemplate("zypper.tmpl", packages)

	checks := []string{
		"RUN zypper -n refresh",
		"zypper -n install",
		"zypper -n clean -a",
		"rm -rf /var/log/",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("zypper.tmpl output missing %q\n\nFull output:\n%s", want, got)
		}
	}
	for _, pkg := range packages {
		if !strings.Contains(got, pkg) {
			t.Errorf("zypper.tmpl output missing package %q", pkg)
		}
	}
}

func validTestConfig() *config.Config {
	return &config.Config{
		Images: []config.Image{
			{
				Name:      "go1.26",
				Base:      "registry.suse.com/bci/golang:1.26.2@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64", "linux/arm64"},
				Packages:  []string{"git-core", "wget"},
				Tools:     []string{"govulncheck"},
			},
		},
		Tools: []config.Tool{
			{
				Name:      "golangci-lint",
				Source:    "golangci/golangci-lint",
				Version:   "v2.11.4",
				Universal: true,
				Checksums: map[string]string{
					"linux/amd64": strings.Repeat("a", 64),
					"linux/arm64": strings.Repeat("b", 64),
				},
				Release: &config.ReleaseConfig{
					DownloadTemplate: "https://github.com/{source}/releases/download/{version}/golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz",
					Extract:          "golangci-lint-{version|trimprefix:v}-{os}-{arch}/golangci-lint",
				},
				Install: config.InstallConfig{Method: "curl"},
			},
			{
				Name:          "govulncheck",
				Source:        "golang/vuln",
				Version:       "v1.2.0",
				VersionCommit: "abc123",
				Install: config.InstallConfig{
					Method:  "go-install",
					Package: "golang.org/x/vuln/cmd/govulncheck@{version_commit}",
				},
			},
		},
	}
}

func TestGenerate_Structure(t *testing.T) {
	cfg := validTestConfig()
	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	content, ok := result["go1.26"]
	if !ok {
		t.Fatal("Generate() missing 'go1.26' key")
	}

	// Check structural elements.
	checks := []string{
		"FROM registry.suse.com/bci/golang:1.26.2@sha256:",
		"ARG TARGETARCH",
		"ENV ARCH=$TARGETARCH",
		"zypper -n refresh",
		"zypper -n install",
		"git-core",
		"wget",
		"zypper -n clean -a",
		"# golangci-lint v2.11.4",
		"# govulncheck v1.2.0",
		"go install golang.org/x/vuln/cmd/govulncheck@abc123",
		"# Cleanup Go caches",
		"go clean -cache -modcache",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("Generate() output missing %q\n\nFull output:\n%s", want, content)
		}
	}
}

func TestGenerate_UniversalToolOrder(t *testing.T) {
	// Universal tools appear before image.tools tools.
	cfg := validTestConfig()
	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["go1.26"]

	lintIdx := strings.Index(content, "# golangci-lint")
	vulnIdx := strings.Index(content, "# govulncheck")
	if lintIdx == -1 || vulnIdx == -1 {
		t.Fatal("Generate() missing expected tool blocks")
	}
	if lintIdx > vulnIdx {
		t.Error("Generate() universal tool (golangci-lint) should appear before image tool (govulncheck)")
	}
}

func TestGenerate_PlatformIntersection(t *testing.T) {
	// Image declares only amd64; tool has both amd64 and arm64.
	// Generated Dockerfile should only contain amd64 case entry.
	cfg := validTestConfig()
	cfg.Images[0].Platforms = []string{"linux/amd64"}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["go1.26"]

	if strings.Contains(content, "arm64") {
		t.Error("Generate() should not emit arm64 when image only declares amd64")
	}
	if !strings.Contains(content, "amd64") {
		t.Error("Generate() should emit amd64")
	}
}

func TestGenerate_NoGoCleanWithoutGoInstall(t *testing.T) {
	// When no go-install tool is present, go clean should not appear.
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test",
				Base:      "registry.suse.com/bci/base:latest@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
			},
		},
		Tools: []config.Tool{
			{
				Name:      "mytool",
				Source:    "org/mytool",
				Version:   "v1.0.0",
				Universal: true,
				Checksums: map[string]string{
					"linux/amd64": strings.Repeat("c", 64),
				},
				Release: &config.ReleaseConfig{
					DownloadTemplate: "https://example.com/{version}/{arch}.tar.gz",
					Extract:          "mytool",
				},
				Install: config.InstallConfig{Method: "curl"},
			},
		},
	}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["test"]

	if strings.Contains(content, "go clean") {
		t.Error("Generate() should not emit 'go clean' when no go-install tools are present")
	}
}

func TestGenerate_MultipleImages(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "img1",
				Base:      "base1@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
			},
			{
				Name:      "img2",
				Base:      "base2@sha256:" + strings.Repeat("b", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"git-core"},
			},
		},
		Tools: []config.Tool{
			{
				Name:      "shared",
				Source:    "org/shared",
				Version:   "v1.0.0",
				Universal: true,
				Checksums: map[string]string{"linux/amd64": strings.Repeat("c", 64)},
				Release: &config.ReleaseConfig{
					DownloadTemplate: "https://example.com/{version}.tar.gz",
					Extract:          "shared",
				},
				Install: config.InstallConfig{Method: "curl"},
			},
		},
	}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Generate() returned %d images, want 2", len(result))
	}
	if _, ok := result["img1"]; !ok {
		t.Error("Generate() missing img1")
	}
	if _, ok := result["img2"]; !ok {
		t.Error("Generate() missing img2")
	}
	// Both images should include the universal tool.
	for name, content := range result {
		if !strings.Contains(content, "# shared v1.0.0") {
			t.Errorf("Generate() image %q missing universal tool block", name)
		}
	}
}

func TestHasAnyOfPackages(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
		check    []string
		want     bool
	}{
		{
			name:     "empty packages and check",
			packages: []string{},
			check:    []string{},
			want:     false,
		},
		{
			name:     "empty packages with check",
			packages: []string{},
			check:    []string{"git"},
			want:     false,
		},
		{
			name:     "has single package",
			packages: []string{"git", "wget", "curl"},
			check:    []string{"git"},
			want:     true,
		},
		{
			name:     "has one of multiple packages - first match",
			packages: []string{"git", "wget", "curl"},
			check:    []string{"git", "git-core"},
			want:     true,
		},
		{
			name:     "has one of multiple packages - second match",
			packages: []string{"git-core", "wget", "curl"},
			check:    []string{"git", "git-core"},
			want:     true,
		},
		{
			name:     "has both packages",
			packages: []string{"git", "git-core", "wget"},
			check:    []string{"git", "git-core"},
			want:     true,
		},
		{
			name:     "does not have any package",
			packages: []string{"wget", "curl", "jq"},
			check:    []string{"git", "git-core"},
			want:     false,
		},
		{
			name:     "partial name does not match",
			packages: []string{"git-lfs"},
			check:    []string{"git"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := DockerfileVars{
				Packages: tt.packages,
			}
			got := vars.HasAnyOfPackages(tt.check...)
			if got != tt.want {
				t.Errorf("HasAnyOfPackages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerate_GitConfigWithGit(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test-git",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"git", "wget"},
			},
		},
		Tools: []config.Tool{},
	}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	content := result["test-git"]
	expectedGitConfig := "git config --system --add safe.directory '*'"
	if !strings.Contains(content, expectedGitConfig) {
		t.Errorf("Generate() should emit git config when 'git' package is present\n\nFull output:\n%s", content)
	}
}

func TestGenerate_GitConfigWithGitCore(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test-git-core",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"git-core", "wget"},
			},
		},
		Tools: []config.Tool{},
	}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	content := result["test-git-core"]
	expectedGitConfig := "git config --system --add safe.directory '*'"
	if !strings.Contains(content, expectedGitConfig) {
		t.Errorf("Generate() should emit git config when 'git-core' package is present\n\nFull output:\n%s", content)
	}
}

// --- Alias rendering tests ---

// helmUniversalTool returns a minimal valid universal curl tool for alias tests.
func helmUniversalTool() config.Tool {
	return config.Tool{
		Name:      "helm",
		Source:    "https://get.helm.sh",
		Mode:      "static",
		Version:   "v3.20.0",
		Universal: true,
		Checksums: map[string]string{
			"linux/amd64": strings.Repeat("c", 64),
			"linux/arm64": strings.Repeat("d", 64),
		},
		Release: &config.ReleaseConfig{
			DownloadTemplate: "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz",
			Extract:          "{os}-{arch}/helm",
		},
		Install: config.InstallConfig{Method: "curl"},
	}
}

func TestGenerate_SingleAlias(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
				Aliases:   map[string]string{"helm_v3": "helm"},
			},
		},
		Tools: []config.Tool{helmUniversalTool()},
	}

	result, err := Generate(cfg, "")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["test"]

	if !strings.Contains(content, "# Aliases") {
		t.Errorf("Generate() missing '# Aliases' comment\n\nFull output:\n%s", content)
	}
	if !strings.Contains(content, "ln -sf /usr/local/bin/helm /usr/local/bin/helm_v3") {
		t.Errorf("Generate() missing expected symlink\n\nFull output:\n%s", content)
	}
}

func TestGenerate_MultipleAliasesSortedOrder(t *testing.T) {
	// Aliases map is unordered; output must always be sorted by alias name.
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
				Aliases: map[string]string{
					"z_helm": "helm",
					"a_helm": "helm",
				},
			},
		},
		Tools: []config.Tool{helmUniversalTool()},
	}

	result, err := Generate(cfg, "")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["test"]

	aIdx := strings.Index(content, "ln -sf /usr/local/bin/helm /usr/local/bin/a_helm")
	zIdx := strings.Index(content, "ln -sf /usr/local/bin/helm /usr/local/bin/z_helm")
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("Generate() missing expected symlinks\n\nFull output:\n%s", content)
	}
	if aIdx > zIdx {
		t.Errorf("Generate() aliases not in sorted order: a_helm(%d) should precede z_helm(%d)", aIdx, zIdx)
	}
}

func TestGenerate_NoAliasBlockWhenNone(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
			},
		},
		Tools: []config.Tool{helmUniversalTool()},
	}

	result, err := Generate(cfg, "")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["test"]

	if strings.Contains(content, "# Aliases") {
		t.Errorf("Generate() should not emit aliases block when no aliases defined\n\nFull output:\n%s", content)
	}
	if strings.Contains(content, "ln -sf") {
		t.Errorf("Generate() should not emit ln -sf when no aliases defined\n\nFull output:\n%s", content)
	}
}

func TestGenerate_AliasAppearsAfterTools(t *testing.T) {
	// The aliases block must come after all tool install blocks.
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget"},
				Aliases:   map[string]string{"helm_v3": "helm"},
			},
		},
		Tools: []config.Tool{helmUniversalTool()},
	}

	result, err := Generate(cfg, "")
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	content := result["test"]

	helmInstallIdx := strings.Index(content, "# helm v3.20.0")
	aliasIdx := strings.Index(content, "# Aliases")
	if helmInstallIdx == -1 || aliasIdx == -1 {
		t.Fatalf("Generate() missing tool or alias block\n\nFull output:\n%s", content)
	}
	if aliasIdx < helmInstallIdx {
		t.Errorf("Generate() alias block (%d) should appear after tool block (%d)", aliasIdx, helmInstallIdx)
	}
}

func TestGenerate_NoGitConfigWithoutGit(t *testing.T) {
	cfg := &config.Config{
		Images: []config.Image{
			{
				Name:      "test-no-git",
				Base:      "base@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64"},
				Packages:  []string{"wget", "curl"},
			},
		},
		Tools: []config.Tool{},
	}

	defaultSource := ""
	result, err := Generate(cfg, defaultSource)
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}

	content := result["test-no-git"]
	unexpectedGitConfig := "git config --system --add safe.directory"
	if strings.Contains(content, unexpectedGitConfig) {
		t.Errorf("Generate() should not emit git config when neither 'git' nor 'git-core' packages are present\n\nFull output:\n%s", content)
	}
}
