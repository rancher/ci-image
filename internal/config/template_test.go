package config

import (
	"errors"
	"strings"
	"testing"

	ghrelease "github.com/mallardduck/ghreleases"
)

var defaultVars = TemplateVars("golangci-lint", "golangci/golangci-lint", "v2.11.4", "abc123", "linux", "amd64")

func TestRenderTemplate_RealWorldTemplates(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "golangci-lint download URL",
			tmpl: "https://github.com/{source}/releases/download/{version}/golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz",
			want: "https://github.com/golangci/golangci-lint/releases/download/v2.11.4/golangci-lint-2.11.4-linux-amd64.tar.gz",
		},
		{
			name: "golangci-lint extract path",
			tmpl: "golangci-lint-{version|trimprefix:v}-{os}-{arch}/golangci-lint",
			want: "golangci-lint-2.11.4-linux-amd64/golangci-lint",
		},
		{
			name: "goreleaser with title and replace",
			tmpl: "https://github.com/{source}/releases/download/{version}/goreleaser_{os|title}_{arch|replace:amd64=x86_64}.tar.gz",
			want: "https://github.com/golangci/golangci-lint/releases/download/v2.11.4/goreleaser_Linux_x86_64.tar.gz",
		},
		{
			name: "go-install package with version_commit",
			tmpl: "golang.org/x/vuln/cmd/govulncheck@{version_commit}",
			want: "golang.org/x/vuln/cmd/govulncheck@abc123",
		},
		{name: "name variable", tmpl: "{name}-{name|upper}", want: "golangci-lint-GOLANGCI-LINT"},
		{name: "empty template", tmpl: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.tmpl, defaultVars)
			if err != nil {
				t.Fatalf("RenderTemplate() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("RenderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTemplate_EmptyExtrasAreKnownVariables(t *testing.T) {
	// Most tools have no version_commit; a template naming it must render empty, not fail.
	v := TemplateVars("t", "o/r", "v1", "", "linux", "amd64")
	got, err := RenderTemplate("x{version_commit}y{source}", v)
	if err != nil || got != "xyo/r" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRenderTemplate_StrictErrorsDiscardOutput(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want error
	}{
		{"unknown variable", "prefix-{unknown}", ghrelease.ErrUnknownVariable},
		{"unknown modifier", "prefix-{name|bogus}", ghrelease.ErrUnknownModifier},
		{"replace missing equals", "prefix-{name|replace:foo}", ghrelease.ErrInvalidModifier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.tmpl, defaultVars)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if got != "" {
				t.Errorf("partial output %q must be discarded on error", got)
			}
		})
	}
}

func TestExpandGitHubTemplate(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   string
		source string
		want   string
	}{
		{
			name:   "full URL unchanged",
			tmpl:   "https://github.com/org/repo/releases/download/{version}/file.tar.gz",
			source: "org/repo",
			want:   "https://github.com/org/repo/releases/download/{version}/file.tar.gz",
		},
		{
			name:   "non-GitHub full URL unchanged",
			tmpl:   "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz",
			source: "https://get.helm.sh",
			want:   "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz",
		},
		{
			name:   "short filename with org/repo source",
			tmpl:   "yq_{os}_{arch}.tar.gz",
			source: "mikefarah/yq",
			want:   "https://github.com/mikefarah/yq/releases/download/{version}/yq_{os}_{arch}.tar.gz",
		},
		{
			name:   "short filename with full github.com source",
			tmpl:   "golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz",
			source: "https://github.com/golangci/golangci-lint",
			want:   "https://github.com/golangci/golangci-lint/releases/download/{version}/golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz",
		},
		{
			name:   "source with trailing slash and .git",
			tmpl:   "f",
			source: "https://github.com/org/repo.git/",
			want:   "https://github.com/org/repo/releases/download/{version}/f",
		},
		{
			name:   "short checksum filename",
			tmpl:   "sha256sum.txt",
			source: "rancher/charts-build-scripts",
			want:   "https://github.com/rancher/charts-build-scripts/releases/download/{version}/sha256sum.txt",
		},
		{
			name:   "non-GitHub source with bare filename left unchanged",
			tmpl:   "helm-{version}-{os}-{arch}.tar.gz",
			source: "https://get.helm.sh",
			want:   "helm-{version}-{os}-{arch}.tar.gz",
		},
		{
			name:   "look-alike host is not GitHub",
			tmpl:   "f",
			source: "https://notgithub.com/org/repo",
			want:   "f",
		},
		{name: "empty template", tmpl: "", source: "org/repo", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandGitHubTemplate(tt.tmpl, tt.source); got != tt.want {
				t.Errorf("ExpandGitHubTemplate(%q, %q)\n  got  %q\n  want %q", tt.tmpl, tt.source, got, tt.want)
			}
		})
	}
}

func TestExpandGitHubTemplate_ExpandedTemplateRenders(t *testing.T) {
	got := ExpandGitHubTemplate("ob-charts-tool_{version|trimprefix:v}_checksums.txt", "rancher/ob-charts-tool")
	rendered, err := RenderTemplate(got, TemplateVars("ob-charts-tool", "rancher/ob-charts-tool", "v1.2.3", "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/rancher/ob-charts-tool/releases/download/v1.2.3/ob-charts-tool_1.2.3_checksums.txt"
	if rendered != want {
		t.Errorf("got  %q\nwant %q", rendered, want)
	}
	if strings.Contains(rendered, "{") {
		t.Errorf("unrendered token in %q", rendered)
	}
}

func TestIsGitHubSource(t *testing.T) {
	for source, want := range map[string]bool{
		"org/repo":                        true,
		"https://github.com/org/repo":     true,
		"https://github.com/org/repo/":    true,
		"https://get.helm.sh":             false,
		"https://dl.k8s.io/release":       false,
		"https://notgithub.com/org/repo":  false,
		"https://github.com.evil.com/a/b": false,
		"org/repo/extra":                  false,
		"org":                             false,
		"":                                false,
		"https://releases.nixos.org/nix":  false,
	} {
		if got := isGitHubSource(source); got != want {
			t.Errorf("isGitHubSource(%q) = %v, want %v", source, got, want)
		}
	}
}
