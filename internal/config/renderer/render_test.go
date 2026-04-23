package renderer

import (
	"strings"
	"testing"
)

var defaultVars = Vars{
	Name:          "golangci-lint",
	Source:        "golangci/golangci-lint",
	Version:       "v2.11.4",
	VersionCommit: "abc123",
	OS:            "linux",
	Arch:          "amd64",
}

func TestRender_Variables(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "single variable",
			tmpl: "{name}",
			want: "golangci-lint",
		},
		{
			name: "multiple variables",
			tmpl: "{os}/{arch}",
			want: "linux/amd64",
		},
		{
			name: "variable in URL",
			tmpl: "https://github.com/{source}/releases/download/{version}/golangci-lint-{version}-{os}-{arch}.tar.gz",
			want: "https://github.com/golangci/golangci-lint/releases/download/v2.11.4/golangci-lint-v2.11.4-linux-amd64.tar.gz",
		},
		{
			name: "no variables",
			tmpl: "plain string",
			want: "plain string",
		},
		{
			name: "empty template",
			tmpl: "",
			want: "",
		},
		{
			name: "version_commit variable",
			tmpl: "golang.org/x/vuln/cmd/govulncheck@{version_commit}",
			want: "golang.org/x/vuln/cmd/govulncheck@abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, defaultVars)
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_Modifiers(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{name: "trimprefix removes prefix", tmpl: "{version|trimprefix:v}", want: "2.11.4"},
		{name: "trimprefix no match is no-op", tmpl: "{version|trimprefix:x}", want: "v2.11.4"},
		{name: "trimsuffix removes suffix", tmpl: "{os|trimsuffix:ux}", want: "lin"},
		{name: "trimsuffix no match is no-op", tmpl: "{os|trimsuffix:win}", want: "linux"},
		{name: "upper", tmpl: "{os|upper}", want: "LINUX"},
		{name: "lower", tmpl: "{name|lower}", want: "golangci-lint"},
		{name: "title capitalises first char", tmpl: "{os|title}", want: "Linux"},
		{name: "title on empty string", tmpl: "{os|title}", want: ""},
		{name: "replace exact match", tmpl: "{arch|replace:amd64=x86_64}", want: "x86_64"},
		{name: "replace no match is no-op", tmpl: "{arch|replace:arm64=aarch64}", want: "amd64"},
		{name: "chained trimprefix then upper", tmpl: "{version|trimprefix:v|upper}", want: "2.11.4"},
		{name: "chained replace for multiple arch mappings", tmpl: "{arch|replace:amd64=x86_64|replace:arm64=aarch64}", want: "x86_64"},
	}

	for _, tt := range tests {
		vars := defaultVars
		if tt.name == "title on empty string" {
			vars.OS = ""
		}
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, vars)
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_Errors(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		vars    Vars
		wantErr string
	}{
		{name: "unknown variable", tmpl: "{unknown}", vars: Vars{Name: "foo"}, wantErr: `unknown template variable "unknown"`},
		{name: "unknown modifier", tmpl: "{name|bogus}", vars: Vars{Name: "foo"}, wantErr: `unknown modifier "bogus"`},
		{name: "replace modifier missing equals", tmpl: "{name|replace:foo}", vars: Vars{Name: "foo"}, wantErr: "missing '=' in argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.tmpl, tt.vars)
			if err == nil {
				t.Fatal("Render() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Render() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRender_RealWorldTemplates(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, defaultVars)
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}
