package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/registry"
)

func TestDiscover_BaseImagesOnlyWhenRequested(t *testing.T) {
	calls := 0
	orig := newRegistry
	newRegistry = func(host string) (*registry.Client, error) {
		calls++
		return nil, context.Canceled // fail fast: we only count attempts
	}
	t.Cleanup(func() { newRegistry = orig })

	cfg := &config.Config{Images: []config.Image{{
		Name: "x",
		Base: "registry.suse.com/bci/golang:1.27.2@sha256:" + hex64("a"),
	}}}

	rep := Discover(context.Background(), cfg, Options{})
	if calls != 0 || len(rep.Skipped)+len(rep.Candidates)+len(rep.UpToDate) != 0 {
		t.Fatalf("default run touched base images: calls=%d report=%+v", calls, rep)
	}

	rep = Discover(context.Background(), cfg, Options{BaseImages: true})
	if calls != 1 || len(rep.Skipped) != 1 {
		t.Fatalf("--base-images should check the base: calls=%d report=%+v", calls, rep)
	}
}

func TestDiscover_ToolsFilterChecksOnlyNamed(t *testing.T) {
	cfg := &config.Config{Tools: []config.Tool{
		// release-checksums tools are skipped without any network call,
		// which makes them a cheap way to observe which tools were visited.
		{Name: "a", Source: "o/a", Version: "latest", Mode: config.ModeReleaseChecksums},
		{Name: "b", Source: "o/b", Version: "latest", Mode: config.ModeReleaseChecksums},
		{Name: "c", Source: "o/c", Version: "latest", Mode: config.ModeReleaseChecksums},
	}}
	rep := Discover(context.Background(), cfg, Options{Tools: []string{"a", "c"}})
	var got []string
	for _, s := range rep.Skipped {
		got = append(got, s.Name)
	}
	if strings.Join(got, ",") != "a,c" {
		t.Fatalf("visited %v, want a,c", got)
	}
	if rep := Discover(context.Background(), cfg, Options{}); len(rep.Skipped) != 3 {
		t.Fatalf("nil filter should visit all, visited %d", len(rep.Skipped))
	}
}

func TestChooseTag(t *testing.T) {
	tags := []string{"1.27.1", "1.27.2", "1.27.10", "1.27.2-1.1", "1.28.0", "1.26.9", "latest", "24", "25", "15.7", "15.8"}
	tests := []struct{ cur, chosen, newer string }{
		{"1.27.2", "1.27.10", "1.28.0"}, // numeric patch order, newer line reported
		{"1.28.0", "1.28.0", ""},
		{"24", "24", "25"}, // floating major: tag kept, line reported
		{"15.7", "15.7", "15.8"},
		{"latest", "latest", ""},
	}
	for _, tt := range tests {
		c, n := chooseTag(tt.cur, slices.Clone(tags))
		if c != tt.chosen || n != tt.newer {
			t.Errorf("chooseTag(%q) = %q,%q; want %q,%q", tt.cur, c, n, tt.chosen, tt.newer)
		}
	}
}

func TestChooseTag_VariantAndRevision(t *testing.T) {
	tags := []string{
		// Application Collection: no rolling tag, every tag carries a revision.
		"1.26.9-dev-34.1", "1.26.9-dev-34.2", "1.26.10-dev-36.2", "1.26.10-dev-36.3", "1.26.10-dev-36.10",
		"1.27.0-dev-37.1",              // next line: reported, never proposed
		"1.26.11-slim-40.1", "1.26.11", // other variants never cross over
		"1.26.11-dev", "1.26.12-dev-40.1-rc1", // other shapes / unparseable
		// BCI: variant plus build revision, and a revisionless rolling tag.
		"24.18.1-base", "24.18.1-base-13.1", "24.18.1-base-13.2", "24.18.2-base-14.1", "24.18.2-base", "24.19.0-base-1.1",
		"1.27.2", "1.27.2-1.1", "1.27.3-2.1",
	}
	tests := []struct{ cur, chosen, newer string }{
		{"1.26.9-dev-34.1", "1.26.10-dev-36.10", "1.27.0-dev-37.1"}, // version and revision both advance, numerically
		{"1.26.10-dev-36.3", "1.26.10-dev-36.10", "1.27.0-dev-37.1"},
		{"1.26.9-dev-34.2", "1.26.10-dev-36.10", "1.27.0-dev-37.1"},
		{"24.18.1-base-13.1", "24.18.2-base-14.1", "24.19.0-base-1.1"}, // BCI variant + revision
		{"24.18.1-base", "24.18.2-base", ""},                           // revisionless stays revisionless
		{"1.27.2-1.1", "1.27.3-2.1", ""},                               // BCI revision without a variant
		{"1.27.2", "1.27.2", ""},                                       // plain tags ignore revisioned ones
	}
	for _, tt := range tests {
		c, n := chooseTag(tt.cur, slices.Clone(tags))
		if c != tt.chosen || n != tt.newer {
			t.Errorf("chooseTag(%q) = %q,%q; want %q,%q", tt.cur, c, n, tt.chosen, tt.newer)
		}
	}
}

func TestDiscoverBase_VariantAndRevisionBump(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			fmt.Fprint(w, `{"tags":["1.26.9-dev-34.1","1.26.10-dev-36.2","1.26.10-dev-36.3","1.27.0-dev-37.1"]}`)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:"+hex64("b"))
	}))
	defer srv.Close()
	orig := newRegistry
	newRegistry = func(string) (*registry.Client, error) {
		return &registry.Client{BaseURL: srv.URL, HTTP: srv.Client()}, nil
	}
	t.Cleanup(func() { newRegistry = orig })

	base := "dp.apps.rancher.io/containers/go:1.26.9-dev-34.1@sha256:" + hex64("a")
	cache := &tagLists{lists: map[string]*tagList{}}
	c, _, sk, _ := discoverBase(context.Background(), cache, base, []string{"go1.27"})
	if sk != nil || c == nil {
		t.Fatalf("expected a candidate, got skipped=%+v", sk)
	}
	if c.Latest != "1.26.10-dev-36.3" || c.Refresh || c.Bump != BumpPatch {
		t.Errorf("candidate = %+v, want a patch bump to 1.26.10-dev-36.3", c)
	}
	if want := "dp.apps.rancher.io/containers/go:1.26.10-dev-36.3@sha256:" + hex64("b"); c.newBase != want {
		t.Errorf("newBase = %q, want %q", c.newBase, want)
	}
}
