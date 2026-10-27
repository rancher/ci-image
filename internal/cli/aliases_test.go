package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/registry"
)

const (
	digA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// fakeRegistry serves every manifest HEAD with digest and counts requests.
func fakeRegistry(t *testing.T, digest string) *int {
	t.Helper()
	hits := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Header().Set("Docker-Content-Digest", digest)
	}))
	t.Cleanup(srv.Close)
	orig := newRegistry
	newRegistry = func(string) (*registry.Client, error) {
		return &registry.Client{BaseURL: srv.URL, HTTP: srv.Client()}, nil
	}
	t.Cleanup(func() { newRegistry = orig })
	return hits
}

func testCfg(images ...config.Image) *config.Config { return &config.Config{Images: images} }

func goImage(name, tag, digest string, langTag bool) config.Image {
	return config.Image{
		Name:        name,
		Base:        "registry.suse.com/bci/golang:" + tag + "@" + digest,
		LanguageTag: langTag,
	}
}

func TestImageAliases(t *testing.T) {
	if a, w := imageAliases(goImage("go1.26", "1.26.10", digA, true)); len(a) != 1 || a[0] != "1.26.10" || w != "" {
		t.Errorf("eligible: %v %q", a, w)
	}
	if a, w := imageAliases(goImage("go1.26", "1.26.10", digA, false)); len(a) != 0 || w != "" {
		t.Errorf("toggle off must be silent: %v %q", a, w)
	}
	a, w := imageAliases(goImage("go1.26", "1.26", digA, true))
	if len(a) != 0 || !strings.Contains(w, "go1.26") || !strings.Contains(w, "not a full X.Y.Z") {
		t.Errorf("short tag: %v %q", a, w)
	}
}

func TestWarnIneligibleAliases(t *testing.T) {
	var buf bytes.Buffer
	warnIneligibleAliases(&buf, testCfg(
		goImage("ok", "1.26.10", digA, true),
		goImage("short", "1.26", digA, true),
		goImage("off", "1.26", digA, false),
	))
	got := buf.String()
	if !strings.Contains(got, "warning: image short:") || strings.Contains(got, "image ok") || strings.Contains(got, "image off") {
		t.Errorf("unexpected warnings:\n%s", got)
	}
}

func TestVerifyAliasDigests(t *testing.T) {
	ctx := context.Background()

	hits := fakeRegistry(t, digA)
	cfg := testCfg(
		goImage("a", "1.26.10", digA, true),
		goImage("b", "1.26.10", digA, true),  // shared base: one lookup
		goImage("c", "1.25.14", digB, false), // no alias: never checked
	)
	if err := verifyAliasDigests(ctx, cfg, nil); err != nil {
		t.Fatalf("match: %v", err)
	}
	if *hits != 1 {
		t.Errorf("expected 1 registry lookup, got %d", *hits)
	}

	fakeRegistry(t, digB)
	err := verifyAliasDigests(ctx, testCfg(goImage("a", "1.26.10", digA, true)), nil)
	if err == nil || !strings.Contains(err.Error(), digA) || !strings.Contains(err.Error(), digB) || !strings.Contains(err.Error(), "image a") {
		t.Fatalf("mismatch must name image and both digests, got %v", err)
	}

	// only filters to the named images.
	if err := verifyAliasDigests(ctx, testCfg(goImage("a", "1.26.10", digA, true)), []string{"other"}); err != nil {
		t.Fatalf("filtered-out image must not be checked: %v", err)
	}
}

func TestOfflineRequested(t *testing.T) {
	cases := []struct {
		env  string
		flag bool
		want bool
	}{{"", false, false}, {"", true, true}, {"1", false, true}, {"0", false, false}, {"false", false, false}}
	for _, c := range cases {
		t.Setenv(offlineEnv, c.env)
		if got := offlineRequested(c.flag); got != c.want {
			t.Errorf("env=%q flag=%v: got %v", c.env, c.flag, got)
		}
	}
}

func TestRunTags(t *testing.T) {
	dir := t.TempDir()
	lock := "images:\n- go1.26\n- charts\nconfigs:\n  go1.26:\n    base: b\n    platforms: [linux/amd64]\n    tag_aliases:\n    - 1.26.10\n  charts:\n    base: b\n    platforms: [linux/amd64]\n"
	if err := os.WriteFile(filepath.Join(dir, "images-lock.yaml"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "deps.yaml")

	out := captureStdout(t, func() {
		if err := runTags([]string{"--config", cfgPath, "go1.26"}); err != nil {
			t.Fatal(err)
		}
	})
	if out != "1.26.10\n" {
		t.Errorf("go1.26 tags = %q", out)
	}
	out = captureStdout(t, func() {
		if err := runTags([]string{"--config", cfgPath, "charts"}); err != nil {
			t.Fatal(err)
		}
	})
	if out != "" {
		t.Errorf("charts tags = %q", out)
	}
	if err := runTags([]string{"--config", cfgPath, "nope"}); err == nil {
		t.Error("unknown image must error")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}
