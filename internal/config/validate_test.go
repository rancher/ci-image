package config

import (
	"strings"
	"testing"
)

// validConfig returns a minimal but fully valid Config for use as a test baseline.
func validConfig() *Config {
	return &Config{
		Images: []Image{
			{
				Name:      "go1.26",
				Base:      "registry.suse.com/bci/golang:1.26.2@sha256:" + strings.Repeat("a", 64),
				Platforms: []string{"linux/amd64", "linux/arm64"},
				Packages:  []string{"git-core", "wget"},
			},
		},
		Tools: []Tool{
			{
				Name:      "golangci-lint",
				Source:    "golangci/golangci-lint",
				Version:   "v2.11.4",
				Universal: true,
				Checksums: map[string]string{
					"linux/amd64": strings.Repeat("a", 64),
					"linux/arm64": strings.Repeat("b", 64),
				},
				Release: &ReleaseConfig{
					DownloadTemplate: "https://github.com/{source}/releases/download/{version}/golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz",
					Extract:          "golangci-lint-{version|trimprefix:v}-{os}-{arch}/golangci-lint",
				},
				Install: InstallConfig{Method: "curl"},
			},
		},
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	if err := validateConfig(validConfig()); err != nil {
		t.Fatalf("validateConfig() unexpected error: %v", err)
	}
}

func TestValidateConfig_ValidWithGoInstall(t *testing.T) {
	cfg := validConfig()
	cfg.Images[0].Tools = []string{"govulncheck"}
	cfg.Tools = append(cfg.Tools, Tool{
		Name:    "govulncheck",
		Source:  "golang/vuln",
		Version: "v1.2.0",
		Install: InstallConfig{
			Method:  "go-install",
			Package: "golang.org/x/vuln/cmd/govulncheck@{version_commit}",
		},
		VersionCommit: "abc123",
	})
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() unexpected error: %v", err)
	}
}

func TestValidateConfig_ValidReleaseChecksums(t *testing.T) {
	cfg := validConfig()
	cfg.Tools = append(cfg.Tools, Tool{
		Name:      "charts-build-scripts",
		Source:    "rancher/charts-build-scripts",
		Mode:      "release-checksums",
		Version:   "latest",
		Universal: true,
		Release: &ReleaseConfig{
			DownloadTemplate: "https://github.com/{source}/releases/download/{version}/charts-build-scripts_{os}_{arch}",
			ChecksumTemplate: "https://github.com/{source}/releases/download/{version}/sha256sum.txt",
			Extract:          "charts-build-scripts",
		},
		Install: InstallConfig{Method: "curl"},
	})
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() unexpected error for release-checksums tool: %v", err)
	}
}

func TestValidateConfig_ValidStaticMode(t *testing.T) {
	cfg := validConfig()
	cfg.Images[0].Tools = []string{"helm"}
	cfg.Tools = append(cfg.Tools, Tool{
		Name:    "helm",
		Source:  "https://get.helm.sh",
		Mode:    "static",
		Version: "v3.17.0",
		Checksums: map[string]string{
			"linux/amd64": strings.Repeat("c", 64),
			"linux/arm64": strings.Repeat("d", 64),
		},
		Release: &ReleaseConfig{
			DownloadTemplate: "https://get.helm.sh/helm-{version}-{os}-{arch}.tar.gz",
			Extract:          "{os}-{arch}/helm",
		},
		Install: InstallConfig{Method: "curl"},
	})
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() unexpected error for static tool: %v", err)
	}
}

func TestValidateConfig_Errors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     func() *Config
		wantErr string
	}{
		// Image validation
		{
			name: "image missing name",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Name = ""
				return c
			},
			wantErr: "missing required field 'name'",
		},
		{
			name: "image missing base",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Base = ""
				return c
			},
			wantErr: "missing required field 'base'",
		},
		{
			name: "image invalid platform format",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Platforms = []string{"INVALID"}
				return c
			},
			wantErr: "invalid platform format",
		},
		{
			name: "image missing packages",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Packages = nil
				return c
			},
			wantErr: "'packages' must have at least one entry",
		},
		{
			name: "image tools references undefined tool",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Tools = []string{"nonexistent"}
				return c
			},
			wantErr: "tool \"nonexistent\" is not defined in tools:",
		},
		{
			name: "image lists universal tool explicitly",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Tools = []string{"golangci-lint"}
				return c
			},
			wantErr: "is in the universal: section and must not be listed in image.tools",
		},
		{
			name: "image duplicate tool",
			cfg: func() *Config {
				c := validConfig()
				c.Tools = append(c.Tools, Tool{
					Name:      "mytone",
					Source:    "my/tool",
					Version:   "v1.0.0",
					Checksums: map[string]string{"linux/amd64": strings.Repeat("a", 64)},
					Release:   &ReleaseConfig{DownloadTemplate: "http://x", Extract: "y"},
					Install:   InstallConfig{Method: "curl"},
				})
				c.Images[0].Tools = []string{"mytone", "mytone"}
				return c
			},
			wantErr: "duplicate tool \"mytone\"",
		},

		// Tool validation
		{
			name: "tool missing source",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Source = ""
				return c
			},
			wantErr: "missing required field 'source'",
		},
		{
			name: "tool missing version",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Version = ""
				return c
			},
			wantErr: "missing required field 'version'",
		},
		{
			name: "tool version latest in pinned mode",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Version = "latest"
				return c
			},
			wantErr: "version 'latest' is not allowed in mode",
		},
		{
			name: "tool unsupported mode",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Mode = "unknown-mode"
				return c
			},
			wantErr: "is not supported",
		},
		{
			name: "unknown install method",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Install.Method = "wget"
				return c
			},
			wantErr: "unknown install method \"wget\"",
		},
		{
			name: "curl tool missing release block",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Release = nil
				return c
			},
			wantErr: "requires a 'release:' block",
		},
		{
			name: "curl tool missing download_template",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Release.DownloadTemplate = ""
				return c
			},
			wantErr: "release.download_template is required",
		},
		{
			name: "curl tool missing extract",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Release.Extract = ""
				return c
			},
			wantErr: "release.extract is required",
		},
		{
			name: "curl tool missing checksums",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Checksums = nil
				return c
			},
			wantErr: "requires checksums",
		},
		{
			name: "curl tool invalid checksum",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Checksums["linux/amd64"] = "tooshort"
				return c
			},
			wantErr: "invalid SHA-256 checksum",
		},
		{
			name: "curl tool with install.package",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Install.Package = "some/pkg@v1"
				return c
			},
			wantErr: "install.package is forbidden for method 'curl'",
		},
		{
			name: "go-install tool missing package",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Tools = []string{"gov"}
				c.Tools = append(c.Tools, Tool{
					Name:    "gov",
					Source:  "golang/vuln",
					Version: "v1.0.0",
					Install: InstallConfig{Method: "go-install"},
				})
				return c
			},
			wantErr: "install.package is required for method 'go-install'",
		},
		{
			name: "go-install tool with release block",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Tools = []string{"gov"}
				c.Tools = append(c.Tools, Tool{
					Name:    "gov",
					Source:  "golang/vuln",
					Version: "v1.0.0",
					Release: &ReleaseConfig{DownloadTemplate: "x", Extract: "y"},
					Install: InstallConfig{Method: "go-install", Package: "pkg@v1"},
				})
				return c
			},
			wantErr: "release: block is forbidden for method 'go-install'",
		},
		{
			name: "go-install tool with checksums",
			cfg: func() *Config {
				c := validConfig()
				c.Images[0].Tools = []string{"gov"}
				c.Tools = append(c.Tools, Tool{
					Name:      "gov",
					Source:    "golang/vuln",
					Version:   "v1.0.0",
					Checksums: map[string]string{"linux/amd64": strings.Repeat("a", 64)},
					Install:   InstallConfig{Method: "go-install", Package: "pkg@v1"},
				})
				return c
			},
			wantErr: "checksums are forbidden for method 'go-install'",
		},
		{
			name: "non-universal tool not in any image",
			cfg: func() *Config {
				c := validConfig()
				c.Tools = append(c.Tools, Tool{
					Name:    "unused",
					Source:  "example/unused",
					Version: "v1.0.0",
					Checksums: map[string]string{
						"linux/amd64": strings.Repeat("c", 64),
						"linux/arm64": strings.Repeat("d", 64),
					},
					Release: &ReleaseConfig{DownloadTemplate: "http://x/{version}", Extract: "bin"},
					Install: InstallConfig{Method: "curl"},
				})
				return c
			},
			wantErr: "not universal and not listed in any image.tools",
		},
		{
			name: "curl tool missing checksum for image platform",
			cfg: func() *Config {
				c := validConfig()
				delete(c.Tools[0].Checksums, "linux/arm64")
				return c
			},
			wantErr: "missing checksum for platform linux/arm64",
		},
		// release-checksums validation
		{
			name: "release-checksums source not in allowlist",
			cfg: func() *Config {
				c := validConfig()
				c.Tools = append(c.Tools, Tool{
					Name:      "external-tool",
					Source:    "external/tool",
					Mode:      "release-checksums",
					Version:   "latest",
					Universal: true,
					Release: &ReleaseConfig{
						DownloadTemplate: "https://example.com/{version}.tar.gz",
						Extract:          "tool",
					},
					Install: InstallConfig{Method: "curl"},
				})
				return c
			},
			wantErr: "requires source to be in the allowlist",
		},
		{
			name: "release-checksums with static checksums",
			cfg: func() *Config {
				c := validConfig()
				c.Tools = append(c.Tools, Tool{
					Name:      "charts-build-scripts",
					Source:    "rancher/charts-build-scripts",
					Mode:      "release-checksums",
					Version:   "latest",
					Universal: true,
					Checksums: map[string]string{"linux/amd64": strings.Repeat("a", 64)},
					Release: &ReleaseConfig{
						DownloadTemplate: "https://github.com/{source}/releases/download/{version}/tool",
						Extract:          "tool",
					},
					Install: InstallConfig{Method: "curl"},
				})
				return c
			},
			wantErr: "checksums must be absent for mode 'release-checksums'",
		},
		// Multiple errors collected
		{
			name: "multiple errors reported together",
			cfg: func() *Config {
				c := validConfig()
				c.Tools[0].Source = ""
				c.Tools[0].Version = ""
				return c
			},
			wantErr: "missing required field 'source'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg())
			if err == nil {
				t.Fatal("validateConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateConfig() error = %q\nwant substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateConfig_MultipleErrorsCollected(t *testing.T) {
	cfg := validConfig()
	cfg.Tools[0].Source = ""
	cfg.Tools[0].Version = ""

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'source'") {
		t.Errorf("error missing source complaint: %s", msg)
	}
	if !strings.Contains(msg, "'version'") {
		t.Errorf("error missing version complaint: %s", msg)
	}
}
