package config

import "testing"

func TestReleaseConfig_ShouldInstallToPath(t *testing.T) {
	tests := []struct {
		name string
		rel  *ReleaseConfig
		want bool
	}{
		{
			name: "nil config defaults to true",
			rel:  nil,
			want: true,
		},
		{
			name: "nil InstallToPath field defaults to true",
			rel: &ReleaseConfig{
				DownloadTemplate: "tool-{version}.tar.gz",
				Extract:          "tool",
			},
			want: true,
		},
		{
			name: "explicit true",
			rel: &ReleaseConfig{
				DownloadTemplate: "tool-{version}.tar.gz",
				Extract:          "tool",
				InstallToPath:    boolPtr(true),
			},
			want: true,
		},
		{
			name: "explicit false",
			rel: &ReleaseConfig{
				DownloadTemplate: "nix-{version}.tar.xz",
				Extract:          "nix-{version}/install",
				InstallToPath:    boolPtr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rel.ShouldInstallToPath()
			if got != tt.want {
				t.Errorf("ShouldInstallToPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mode    Mode
		wantErr bool
	}{
		{
			name:    "valid: pinned",
			mode:    ModePinned,
			wantErr: false,
		},
		{
			name:    "valid: static",
			mode:    ModeStatic,
			wantErr: false,
		},
		{
			name:    "valid: release-checksums",
			mode:    ModeReleaseChecksums,
			wantErr: false,
		},
		{
			name:    "invalid: unknown",
			mode:    Mode("unknown"),
			wantErr: true,
		},
		{
			name:    "invalid: empty",
			mode:    Mode(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mode.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Mode.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMethod_Validate(t *testing.T) {
	tests := []struct {
		name    string
		method  Method
		wantErr bool
	}{
		{
			name:    "valid: curl",
			method:  MethodCurl,
			wantErr: false,
		},
		{
			name:    "valid: go-install",
			method:  MethodGoInstall,
			wantErr: false,
		},
		{
			name:    "invalid: unknown",
			method:  Method("unknown"),
			wantErr: true,
		},
		{
			name:    "invalid: empty",
			method:  Method(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.method.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Method.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTool_EffectiveMode(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
		want Mode
	}{
		{
			name: "empty mode defaults to pinned",
			tool: Tool{Name: "test"},
			want: ModePinned,
		},
		{
			name: "explicit static mode",
			tool: Tool{Name: "test", Mode: ModeStatic},
			want: ModeStatic,
		},
		{
			name: "explicit release-checksums mode",
			tool: Tool{Name: "test", Mode: ModeReleaseChecksums},
			want: ModeReleaseChecksums,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tool.EffectiveMode()
			if got != tt.want {
				t.Errorf("EffectiveMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstallConfig_EffectiveMethod(t *testing.T) {
	tests := []struct {
		name   string
		config InstallConfig
		want   Method
	}{
		{
			name:   "empty method defaults to curl",
			config: InstallConfig{},
			want:   MethodCurl,
		},
		{
			name:   "explicit curl method",
			config: InstallConfig{Method: MethodCurl},
			want:   MethodCurl,
		},
		{
			name:   "explicit go-install method",
			config: InstallConfig{Method: MethodGoInstall},
			want:   MethodGoInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveMethod()
			if got != tt.want {
				t.Errorf("EffectiveMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTool_TagPrefix(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
		want string
	}{
		{
			name: "defaults to v when release is nil",
			tool: Tool{
				Name:    "test-tool",
				Source:  "org/repo",
				Version: "latest",
				Mode:    ModeReleaseChecksums,
			},
			want: "v",
		},
		{
			name: "defaults to v when tag_prefix is nil",
			tool: Tool{
				Name:    "test-tool",
				Source:  "org/repo",
				Version: "latest",
				Mode:    ModeReleaseChecksums,
				Release: &ReleaseConfig{
					DownloadTemplate: "tool_{os}_{arch}",
					Extract:          "tool",
				},
			},
			want: "v",
		},
		{
			name: "returns empty string when explicitly set to empty",
			tool: Tool{
				Name:    "test-tool",
				Source:  "org/repo",
				Version: "latest",
				Mode:    ModeReleaseChecksums,
				Release: &ReleaseConfig{
					TagPrefix:        strPtr(""),
					DownloadTemplate: "tool_{os}_{arch}",
					Extract:          "tool",
				},
			},
			want: "",
		},
		{
			name: "returns custom prefix for sub-packages",
			tool: Tool{
				Name:    "database-tool",
				Source:  "org/monorepo",
				Version: "latest",
				Mode:    ModeReleaseChecksums,
				Release: &ReleaseConfig{
					TagPrefix:        strPtr("database/v"),
					DownloadTemplate: "db-tool_{os}_{arch}",
					Extract:          "db-tool",
				},
			},
			want: "database/v",
		},
		{
			name: "returns custom prefix without v",
			tool: Tool{
				Name:    "api-tool",
				Source:  "org/monorepo",
				Version: "latest",
				Mode:    ModeReleaseChecksums,
				Release: &ReleaseConfig{
					TagPrefix:        strPtr("api/"),
					DownloadTemplate: "api_{os}_{arch}",
					Extract:          "api",
				},
			},
			want: "api/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tool.TagPrefix()
			if got != tt.want {
				t.Errorf("TagPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string {
	return &s
}
