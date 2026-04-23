package github

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rancher/ci-image/internal/config/renderer"
)

// withTestServer temporarily redirects apiBase and allowedHostSuffix to the given
// httptest server and restores them after the test.
func withTestServer(t *testing.T, ts *httptest.Server) {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	origBase, origSuffix := apiBase, allowedHostSuffix
	apiBase = ts.URL
	allowedHostSuffix = u.Host
	t.Cleanup(func() {
		apiBase = origBase
		allowedHostSuffix = origSuffix
	})
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
			name:   "empty template",
			tmpl:   "",
			source: "org/repo",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandGitHubTemplate(tt.tmpl, tt.source)
			if got != tt.want {
				t.Errorf("ExpandGitHubTemplate(%q, %q)\n  got  %q\n  want %q", tt.tmpl, tt.source, got, tt.want)
			}
		})
	}
}

func TestExpandGitHubTemplate_PreservesModifiers(t *testing.T) {
	got := ExpandGitHubTemplate("ob-charts-tool_{version|trimprefix:v}_checksums.txt", "rancher/ob-charts-tool")
	want := "https://github.com/rancher/ob-charts-tool/releases/download/{version}/ob-charts-tool_{version|trimprefix:v}_checksums.txt"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// Confirm the expanded template is renderable.
	rendered, err := renderer.Render(got, renderer.Vars{Source: "rancher/ob-charts-tool", Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}
	if !strings.Contains(rendered, "1.2.3") {
		t.Errorf("rendered URL should contain trimmed version: %q", rendered)
	}
}

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/golangci/golangci-lint/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, `{"tag_name":"v2.12.0","name":"v2.12.0"}`)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	tag, err := LatestReleaseTag("golangci", "golangci-lint")
	if err != nil {
		t.Fatalf("LatestRelease() unexpected error: %v", err)
	}
	if tag != "v2.12.0" {
		t.Errorf("LatestRelease() = %q, want %q", tag, "v2.12.0")
	}
}

func TestLatestRelease_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	_, err := LatestReleaseTag("nobody", "norepo")
	if err == nil {
		t.Fatal("LatestRelease() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("LatestRelease() error = %q, want it to mention 404", err.Error())
	}
}

func TestLatestRelease_EmptyTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":""}`)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	_, err := LatestReleaseTag("org", "repo")
	if err == nil {
		t.Fatal("LatestRelease() expected error for empty tag_name, got nil")
	}
}

func TestDownloadAndHash(t *testing.T) {
	content := []byte("hello world\n")
	want := sha256.Sum256(content)
	wantHex := hex.EncodeToString(want[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	got, err := DownloadAndHash(srv.URL + "/asset.tar.gz")
	if err != nil {
		t.Fatalf("DownloadAndHash() unexpected error: %v", err)
	}
	if got != wantHex {
		t.Errorf("DownloadAndHash() = %q, want %q", got, wantHex)
	}
}

func TestFetchChecksumFile(t *testing.T) {
	body := `# checksums
abc123def0abc123def0abc123def0abc123def0abc123def0abc123def0abcd  tool-v1.0.0-linux-amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  tool-v1.0.0-linux-arm64.tar.gz
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	got, err := FetchChecksumFile(srv.URL + "/checksums.txt")
	if err != nil {
		t.Fatalf("FetchChecksumFile() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FetchChecksumFile() got %d entries, want 2", len(got))
	}
	wantAmd64 := "abc123def0abc123def0abc123def0abc123def0abc123def0abc123def0abcd"
	if got["tool-v1.0.0-linux-amd64.tar.gz"] != wantAmd64 {
		t.Errorf("amd64 checksum = %q, want %q", got["tool-v1.0.0-linux-amd64.tar.gz"], wantAmd64)
	}
}

func TestFetchChecksumFile_BareHash(t *testing.T) {
	hash := "abc123def0abc123def0abc123def0abc123def0abc123def0abc123def0abcd"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, hash)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	got, err := FetchChecksumFile(srv.URL + "/checksum")
	if err != nil {
		t.Fatalf("FetchChecksumFile() unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("FetchChecksumFile() got %d entries, want 1", len(got))
	}
	if got[""] != hash {
		t.Errorf("FetchChecksumFile() got %q for empty filename, want %q", got[""], hash)
	}
}

func TestFetchChecksumFile_SkipsComments(t *testing.T) {
	body := "# this is a comment\n\nabc123def0abc123def0abc123def0abc123def0abc123def0abc123def0abcd  file.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	got, err := FetchChecksumFile(srv.URL + "/checksums.txt")
	if err != nil {
		t.Fatalf("FetchChecksumFile() unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("FetchChecksumFile() got %d entries, want 1", len(got))
	}
}

func TestFetchChecksumFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	withTestServer(t, srv)

	_, err := FetchChecksumFile(srv.URL + "/missing.txt")
	if err == nil {
		t.Fatal("FetchChecksumFile() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("FetchChecksumFile() error = %q, want it to mention 404", err.Error())
	}
}
