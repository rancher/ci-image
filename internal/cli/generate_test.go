package cli

import (
	"os"
	"slices"
	"testing"

	"go.yaml.in/yaml/v4"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/lockfile"
)

func TestExtractGoVersion(t *testing.T) {
	tests := []struct{ base, want string }{
		{"registry.suse.com/bci/golang:1.26.10@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "1.26.10"},
		{"registry.suse.com/bci/golang:1.26.10", "1.26.10"},
		{"registry.suse.com/bci/python:3.13.15", ""},
		{"registry.suse.com/bci/bci-base:15.7", ""},
	}
	for _, tt := range tests {
		if got := extractGoVersion(tt.base); got != tt.want {
			t.Errorf("extractGoVersion(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestWriteImagesLock_LanguageFields(t *testing.T) {
	t.Chdir(t.TempDir())
	const dg = "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cfg := &config.Config{Images: []config.Image{
		{Name: "go1.26", Base: "registry.suse.com/bci/golang:1.26.10" + dg, LanguageTag: true},
		{Name: "node22", Base: "registry.suse.com/bci/nodejs:22.23.2" + dg},
		{Name: "short", Base: "registry.suse.com/bci/nodejs:24" + dg, LanguageTag: true},
		{Name: "charts", Base: "registry.suse.com/bci/bci-base:15.7" + dg},
	}}
	path := "images-lock.yaml"
	if err := writeImagesLock(cfg, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lk lockfile.ImagesLock
	if err := yaml.Unmarshal(data, &lk); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		image, lang, ver string
		aliases          []string
	}{
		{"go1.26", "go", "1.26.10", []string{"1.26.10"}},
		{"node22", "node", "22.23.2", nil}, // language known, toggle off
		{"short", "node", "", nil},         // toggle on but base has no full version
		{"charts", "", "", nil},            // not a language image
	}
	for _, tt := range tests {
		c := lk.Configs[tt.image]
		if c.Language != tt.lang || c.LanguageVersion != tt.ver || !slices.Equal(c.TagAliases, tt.aliases) {
			t.Errorf("%s: language=%q version=%q aliases=%v, want %q %q %v",
				tt.image, c.Language, c.LanguageVersion, c.TagAliases, tt.lang, tt.ver, tt.aliases)
		}
	}
}
