package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	ghrelease "github.com/mallardduck/ghreleases"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/registry"
)

type rewriteTransport struct{ target *url.URL }

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme, clone.URL.Host, clone.Host = r.target.Scheme, r.target.Host, r.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// useGitHub points the updater's GitHub client at handler, keeping the
// production host allowlist on so tests run under the real policy.
func useGitHub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	c := ghrelease.NewClientWithHTTP(&http.Client{Transport: &rewriteTransport{target: target}}, "",
		ghrelease.WithAllowedHosts("github.com", "githubusercontent.com"))
	orig := githubClient
	githubClient = func() *ghrelease.Client { return c }
	t.Cleanup(func() { githubClient = orig })
}

func tool(name, source, version string) *config.Tool {
	return &config.Tool{Name: name, Source: source, Version: version}
}

func TestDiscoverTool_GitHubBumps(t *testing.T) {
	latest := map[string]string{"patch": "v1.2.4", "minor": "v1.3.0", "major": "v2.0.0"}
	want := map[string]Bump{"patch": BumpPatch, "minor": BumpMinor, "major": BumpMajor}
	for name, tag := range latest {
		t.Run(name, func(t *testing.T) {
			useGitHub(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/o/tool/releases" { // default "v" prefix lists releases
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				fmt.Fprintf(w, `[{"tag_name":%q,"prerelease":false,"draft":false}]`, tag)
			})
			r := discoverTool(context.Background(), tool("tool", "o/tool", "v1.2.3"))
			if r.cand == nil {
				t.Fatalf("no candidate: %+v", r)
			}
			if r.cand.Latest != tag || r.cand.Bump != want[name] || !strings.HasSuffix(r.cand.ReleaseURL, "/o/tool/releases/tag/"+tag) {
				t.Errorf("candidate = %+v", *r.cand)
			}
		})
	}
}

func TestDiscoverTool_UpToDateAndPinnedAhead(t *testing.T) {
	for _, current := range []string{"v1.2.3", "v1.9.0"} { // equal, and pinned ahead of the feed
		useGitHub(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"tag_name":"v1.2.3"}]`)
		})
		r := discoverTool(context.Background(), tool("tool", "o/tool", current))
		if r.cand != nil || r.upToDate != "tool" {
			t.Errorf("current %s: %+v", current, r)
		}
	}
}

func TestDiscoverTool_SkipsWhenLookupFails(t *testing.T) {
	useGitHub(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
	r := discoverTool(context.Background(), tool("tool", "o/tool", "v1.0.0"))
	if r.skip == nil || !strings.Contains(r.skip.Reason, "looking up latest release") {
		t.Fatalf("got %+v", r)
	}
}

func TestDiscoverTool_SkipsFormatMismatch(t *testing.T) {
	useGitHub(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[{"tag_name":"v2.0.0"}]`) })
	if r := discoverTool(context.Background(), tool("t", "o/t", "1.0.0")); r.skip == nil || !strings.Contains(r.skip.Reason, "different version format") {
		t.Errorf("v-prefix mismatch: %+v", r)
	}
}

func TestDiscoverTool_NonGitHubSourcesAreOutOfScope(t *testing.T) {
	useGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a non-GitHub tool must not trigger any request, got %s", r.URL)
	})
	for name, source := range map[string]string{
		"kubectl1.33": "https://dl.k8s.io/release",
		"helmv3":      "https://get.helm.sh",
		"nix":         "https://releases.nixos.org/nix",
		"other":       "https://example.com/dl",
		"lookalike":   "https://notgithub.com/o/r",
	} {
		r := discoverTool(context.Background(), tool(name, source, "v1.0.0"))
		if r.notGitHub != name || r.cand != nil || r.skip != nil || r.upToDate != "" {
			t.Errorf("%s (%s): got %+v, want only notGitHub set", name, source, r)
		}
	}
}

func TestDiscoverTool_ReleaseChecksumsToolsAreLeftToUpdate(t *testing.T) {
	tl := tool("t", "rancher/ob-charts-tool", "latest")
	tl.Mode = config.ModeReleaseChecksums
	if r := discoverTool(context.Background(), tl); r.skip == nil || !strings.Contains(r.skip.Reason, "deps.lock") {
		t.Fatalf("got %+v", r)
	}
}

func TestDiscoverTool_GoInstallResolvesCommit(t *testing.T) {
	sha := strings.Repeat("c", 40)
	useGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/golang/vuln/releases":
			fmt.Fprint(w, `[{"tag_name":"v1.7.0"}]`)
		case "/repos/golang/vuln/commits/v1.7.0":
			fmt.Fprintf(w, `{"sha":%q}`, sha)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	tl := tool("govulncheck", "golang/vuln", "v1.6.0")
	tl.VersionCommit = strings.Repeat("a", 40)
	r := discoverTool(context.Background(), tl)
	if r.cand == nil || r.cand.commit != sha || r.cand.Latest != "v1.7.0" {
		t.Fatalf("got %+v", r)
	}
}

func TestDiscover_ReportsGitHubToolsThenBasesAndCollectsNonGitHub(t *testing.T) {
	useGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v1.0.1"}]`)
	})
	orig := newRegistry
	newRegistry = func(host string) (*registry.Client, error) { return nil, context.Canceled }
	t.Cleanup(func() { newRegistry = orig })

	cfg := &config.Config{
		Images: []config.Image{{Name: "i", Base: "registry.suse.com/bci/golang:1.27.2@sha256:" + hex64("a")}},
		Tools: []config.Tool{
			*tool("kubectl1.33", "https://dl.k8s.io/release", "v1.33.13"),
			*tool("gh-a", "o/a", "v1.0.0"),
			*tool("helmv3", "https://get.helm.sh", "v3.21.3"),
			*tool("gh-b", "o/b", "v1.0.0"),
		},
	}
	rep := Discover(context.Background(), cfg, Options{BaseImages: true})

	var got []string
	for _, c := range rep.Candidates {
		got = append(got, c.Name)
	}
	if strings.Join(got, ",") != "gh-a,gh-b" {
		t.Errorf("candidates = %v, want GitHub tools in declaration order", got)
	}
	if strings.Join(rep.NotGitHub, ",") != "kubectl1.33,helmv3" {
		t.Errorf("NotGitHub = %v", rep.NotGitHub)
	}
	// The base image was attempted after the tools (its lookup fails by design here).
	if len(rep.Skipped) != 1 || !strings.HasPrefix(rep.Skipped[0].Name, "base ") {
		t.Errorf("skipped = %+v, want only the base image", rep.Skipped)
	}
}
