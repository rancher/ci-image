package language

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const dg = "@sha256:b40c0d9763833ecc6e8bc37a810a147c895b1a1c505243bf9ad3a3f0978b26b6"

func TestDetect(t *testing.T) {
	tests := []struct {
		name, base string
		lang, ver  string
		ok         bool
	}{
		{"go", "registry.suse.com/bci/golang:1.26.10" + dg, "go", "1.26.10", true},
		{"python", "registry.suse.com/bci/python:3.13.15" + dg, "python", "3.13.15", true},
		{"node", "registry.suse.com/bci/nodejs:22.23.2" + dg, "node", "22.23.2", true},
		{"variant suffix", "registry.suse.com/bci/nodejs:24.18.1-base" + dg, "node", "24.18.1", true},
		{"appco go dev", "dp.apps.rancher.io/containers/go:1.27.3-dev-14.2" + dg, "go", "1.27.3", true},
		{"appco python dev", "dp.apps.rancher.io/containers/python:3.13.15-dev-14.2" + dg, "python", "3.13.15", true},
		{"appco node dev", "dp.apps.rancher.io/containers/node:24.18.1-dev-14.2" + dg, "node", "24.18.1", true},
		{"appco short tag", "dp.apps.rancher.io/containers/go:1.27" + dg, "go", "", false},
		{"appco revision", "dp.apps.rancher.io/containers/go:1.26.10-dev-36.3" + dg, "go", "1.26.10", true},
		{"bci variant and revision", "registry.suse.com/bci/nodejs:24.18.1-base-13.2" + dg, "node", "24.18.1", true},
		{"bci revision only", "registry.suse.com/bci/golang:1.27.2-1.1" + dg, "go", "1.27.2", true},
		{"prerelease is not a variant", "registry.suse.com/bci/golang:1.27.3-rc1" + dg, "go", "", false},
		{"trailing junk after revision", "dp.apps.rancher.io/containers/go:1.27.3-dev-14.2-rc1" + dg, "go", "", false},
		{"no digest", "registry.suse.com/bci/golang:1.26.10", "go", "1.26.10", true},
		{"short tag", "registry.suse.com/bci/nodejs:24" + dg, "node", "", false},
		{"minor only", "registry.suse.com/bci/python:3.13" + dg, "python", "", false},
		{"no tag", "registry.suse.com/bci/golang" + dg, "go", "", false},
		{"non-language", "registry.suse.com/bci/bci-base:15.7" + dg, "", "", false},
		{"malformed", "golang:1.26", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.base)
			if got.Language != tt.lang || got.Version != tt.ver || got.AliasOK != tt.ok {
				t.Fatalf("Detect = %+v, want lang=%q ver=%q ok=%v", got, tt.lang, tt.ver, tt.ok)
			}
			if !got.AliasOK && got.Reason == "" {
				t.Error("ineligible base must carry a Reason")
			}
			alias, _ := Alias(tt.base)
			if tt.ok && alias != tt.ver || !tt.ok && alias != "" {
				t.Errorf("Alias = %q", alias)
			}
		})
	}
}

type fakeResolver struct {
	digest string
	err    error
}

func (f fakeResolver) Digest(context.Context, string, string) (string, error) {
	return f.digest, f.err
}

func TestVerifyDigest(t *testing.T) {
	base := "registry.suse.com/bci/golang:1.26.10" + dg
	pinned := strings.TrimPrefix(dg, "@")
	other := "sha256:" + strings.Repeat("a", 64)

	if err := VerifyDigest(context.Background(), fakeResolver{digest: pinned}, base); err != nil {
		t.Fatalf("match: %v", err)
	}
	err := VerifyDigest(context.Background(), fakeResolver{digest: other}, base)
	if err == nil || !strings.Contains(err.Error(), other) || !strings.Contains(err.Error(), pinned) {
		t.Fatalf("mismatch must name both digests, got %v", err)
	}
	if err := VerifyDigest(context.Background(), fakeResolver{err: errors.New("boom")}, base); err == nil {
		t.Fatal("registry error must fail")
	}
	if err := VerifyDigest(context.Background(), fakeResolver{}, "registry.suse.com/bci/golang:1.26.10"); err == nil {
		t.Fatal("base without digest must fail")
	}
}
