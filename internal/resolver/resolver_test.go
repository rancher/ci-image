package resolver

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rancher/ci-image/internal/config"
	gh "github.com/rancher/ci-image/internal/github"
	"github.com/rancher/ci-image/internal/lock"
)

// obChartsV040Lock is the deps.lock state from commit 6aac5b23962a674e836f8deb06111c0ee8bb4761,
// just before ob-charts-tool was bumped from v0.4.0 → v0.4.1.
var obChartsV040Lock = lock.Entry{
	ResolvedVersion: "v0.4.0",
	ResolvedAt:      time.Date(2026, 4, 22, 19, 23, 42, 558744141, time.UTC),
	Checksums: map[string]string{
		"linux/amd64": "db379b6a64045d2017e80cc29b53d8eb5e72c753ca1d9f53a328d52d7a41dc2d",
		"linux/arm64": "8407715fb5d49bc3ba7bd11e4bd09436c3c8b9642e514497ea96eb2d8eea5a1c",
	},
}

// obChartsV041Checksums are the real checksums from the v0.4.1 release.
var obChartsV041Checksums = map[string]string{
	"linux/amd64": "79d9d6f724d9859b144d898e7b82932dbedc148778ab30a7631a19b37d62a9b6",
	"linux/arm64": "ff04bfc01be74466b7f958a507c9ef4fefd36616d2586a0be9b09d7a8da8e8ff",
}

// minimalChartsConfig returns a *config.Config with just ob-charts-tool in
// release-checksums mode, and one image that uses it (so toolPlatforms works).
func minimalChartsConfig() *config.Config {
	return &config.Config{
		Images: []config.Image{
			{
				Name:      "charts",
				Base:      "registry.suse.com/bci/bci-base:15.7",
				Platforms: []string{"linux/amd64", "linux/arm64"},
				Tools:     []string{"ob-charts-tool"},
			},
		},
		Tools: []config.Tool{
			{
				Name:    "ob-charts-tool",
				Source:  "rancher/ob-charts-tool",
				Version: "latest",
				Mode:    "release-checksums",
				Release: &config.ReleaseConfig{
					DownloadTemplate: "ob-charts-tool_{os}_{arch}",
					ChecksumTemplate: "ob-charts-tool_{version|trimprefix:v}_checksums.txt",
				},
			},
		},
	}
}

// obChartsChecksumFileBody returns a sha256sum-format checksum file for ob-charts-tool.
func obChartsChecksumFileBody(checksums map[string]string) string {
	return fmt.Sprintf(
		"%s  ob-charts-tool_linux_amd64\n%s  ob-charts-tool_linux_arm64\n",
		checksums["linux/amd64"],
		checksums["linux/arm64"],
	)
}

// newObChartsTestServer returns an httptest.Server handling the GitHub API
// release lookup and the checksum file download for ob-charts-tool.
func newObChartsTestServer(t *testing.T, latestTag string, checksums map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/rancher/ob-charts-tool/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q}`, latestTag)
		case strings.Contains(r.URL.Path, "ob-charts-tool") &&
			strings.Contains(r.URL.Path, "_checksums.txt"):
			fmt.Fprint(w, obChartsChecksumFileBody(checksums))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// captureLog temporarily redirects the standard logger to a buffer.
// The returned function reads whatever was logged so far.
func captureLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return func() string { return buf.String() }
}

// ── ApplyLock (generate path: lock-only, no network) ────────────────────────

func TestApplyLock_FromLock(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{
		Tools: map[string]lock.Entry{"ob-charts-tool": obChartsV040Lock},
	}

	if err := ApplyLock(cfg, lk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tool := &cfg.Tools[0]
	if tool.Version != "v0.4.0" {
		t.Errorf("version = %q, want %q", tool.Version, "v0.4.0")
	}
	if tool.Checksums["linux/amd64"] != obChartsV040Lock.Checksums["linux/amd64"] {
		t.Errorf("amd64 checksum = %q, want %q", tool.Checksums["linux/amd64"], obChartsV040Lock.Checksums["linux/amd64"])
	}
	if tool.Checksums["linux/arm64"] != obChartsV040Lock.Checksums["linux/arm64"] {
		t.Errorf("arm64 checksum = %q, want %q", tool.Checksums["linux/arm64"], obChartsV040Lock.Checksums["linux/arm64"])
	}
}

func TestApplyLock_MissingFromLock(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{Tools: map[string]lock.Entry{}}

	err := ApplyLock(cfg, lk)
	if err == nil {
		t.Fatal("expected error for tool missing from lock, got nil")
	}
	if !strings.Contains(err.Error(), "run 'update'") {
		t.Errorf("error = %q, want it to mention \"run 'update'\"", err.Error())
	}
}

func TestApplyLock_EmptyChecksums(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{
		Tools: map[string]lock.Entry{
			"ob-charts-tool": {ResolvedVersion: "v0.4.0"}, // version set, checksums absent
		},
	}

	if err := ApplyLock(cfg, lk); err == nil {
		t.Fatal("expected error for empty checksums in lock, got nil")
	}
}

func TestApplyLock_SkipsNonReleaseChecksums(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.Tool{
			{Name: "helm", Source: "https://get.helm.sh", Mode: "static", Version: "v3.20.2"},
		},
	}
	lk := &lock.Lock{Tools: map[string]lock.Entry{}}

	// helm is not release-checksums; an empty lock should not cause an error.
	if err := ApplyLock(cfg, lk); err != nil {
		t.Fatalf("unexpected error for non-release-checksums tool: %v", err)
	}
}

// ── Update (update path: mocked network) ────────────────────────────────────

// TestUpdate_VersionChange is the key real-world scenario: the lock records
// v0.4.0 (from commit 6aac5b23), the upstream has released v0.4.1, so Update
// should log a WARNING about the bump and write the new entry into the lock.
func TestUpdate_VersionChange(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{
		Tools: map[string]lock.Entry{"ob-charts-tool": obChartsV040Lock},
	}

	srv := newObChartsTestServer(t, "v0.4.1", obChartsV041Checksums)
	t.Cleanup(gh.OverrideHTTPForTest(srv.URL))
	logged := captureLog(t)

	if err := Update(cfg, lk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := lk.Tools["ob-charts-tool"]
	if !ok {
		t.Fatal("ob-charts-tool missing from lock after update")
	}
	if entry.ResolvedVersion != "v0.4.1" {
		t.Errorf("resolved_version = %q, want %q", entry.ResolvedVersion, "v0.4.1")
	}
	if entry.Checksums["linux/amd64"] != obChartsV041Checksums["linux/amd64"] {
		t.Errorf("amd64 checksum = %q, want %q", entry.Checksums["linux/amd64"], obChartsV041Checksums["linux/amd64"])
	}
	if entry.Checksums["linux/arm64"] != obChartsV041Checksums["linux/arm64"] {
		t.Errorf("arm64 checksum = %q, want %q", entry.Checksums["linux/arm64"], obChartsV041Checksums["linux/arm64"])
	}

	output := logged()
	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected WARNING log for version bump, got:\n%s", output)
	}
	if !strings.Contains(output, "v0.4.0") || !strings.Contains(output, "v0.4.1") {
		t.Errorf("WARNING log should mention both versions, got:\n%s", output)
	}
}

func TestUpdate_AlreadyUpToDate(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{
		Tools: map[string]lock.Entry{
			"ob-charts-tool": {
				ResolvedVersion: "v0.4.1",
				ResolvedAt:      time.Now().UTC(),
				Checksums:       obChartsV041Checksums,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/rancher/ob-charts-tool/releases/latest" {
			fmt.Fprint(w, `{"tag_name":"v0.4.1"}`)
			return
		}
		// Any checksum fetch means the cache-hit path was skipped — fail loudly.
		t.Errorf("unexpected checksum fetch for already-resolved tool: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(gh.OverrideHTTPForTest(srv.URL))

	if err := Update(cfg, lk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lk.Tools["ob-charts-tool"].ResolvedVersion != "v0.4.1" {
		t.Error("lock version changed unexpectedly")
	}
}

func TestUpdate_FirstTime(t *testing.T) {
	cfg := minimalChartsConfig()
	lk := &lock.Lock{Tools: map[string]lock.Entry{}}

	srv := newObChartsTestServer(t, "v0.4.1", obChartsV041Checksums)
	t.Cleanup(gh.OverrideHTTPForTest(srv.URL))
	logged := captureLog(t)

	if err := Update(cfg, lk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := lk.Tools["ob-charts-tool"]
	if !ok {
		t.Fatal("ob-charts-tool missing from lock after first-time update")
	}
	if entry.ResolvedVersion != "v0.4.1" {
		t.Errorf("resolved_version = %q, want %q", entry.ResolvedVersion, "v0.4.1")
	}
	if len(entry.Checksums) != 2 {
		t.Errorf("expected 2 checksums, got %d", len(entry.Checksums))
	}
	// First-time population has no old version to compare — no WARNING expected.
	if strings.Contains(logged(), "WARNING") {
		t.Errorf("first-time update should not emit WARNING, got:\n%s", logged())
	}
}
