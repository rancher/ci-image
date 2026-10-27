package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const good = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseRef(t *testing.T) {
	r, err := ParseRef("registry.suse.com/bci/golang:1.27.2@" + good)
	if err != nil {
		t.Fatal(err)
	}
	if r.Registry != "registry.suse.com" || r.Repo != "bci/golang" || r.Tag != "1.27.2" || r.Digest != good {
		t.Fatalf("got %+v", r)
	}
	if r.String() != "registry.suse.com/bci/golang:1.27.2@"+good {
		t.Fatalf("round trip: %s", r)
	}
	for _, bad := range []string{"golang:1.27", "registry.suse.com/bci/golang@sha256:xyz", "registry.suse.com"} {
		if _, err := ParseRef(bad); err == nil {
			t.Errorf("ParseRef(%q) should fail", bad)
		}
	}
}

func TestNew_Allowlist(t *testing.T) {
	if _, err := New("evil.example.com"); err == nil {
		t.Fatal("expected allowlist rejection")
	}
}

func TestTagsPaginatesAndDigest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/bci/golang/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/bci/golang/tags/list?n=1000&last=b>; rel="next"`)
			fmt.Fprint(w, `{"tags":["a","b"]}`)
			return
		}
		fmt.Fprint(w, `{"tags":["c"]}`)
	})
	mux.HandleFunc("/v2/bci/golang/manifests/1.27.2", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "image.index") {
			t.Error("index media type not requested")
		}
		w.Header().Set("Docker-Content-Digest", good)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client()}

	tags, err := c.Tags(context.Background(), "bci/golang")
	if err != nil || !slices.Equal(tags, []string{"a", "b", "c"}) {
		t.Fatalf("tags = %v, %v", tags, err)
	}
	d, err := c.Digest(context.Background(), "bci/golang", "1.27.2")
	if err != nil || d != good {
		t.Fatalf("digest = %q, %v", d, err)
	}
}

func TestBearerChallenge(t *testing.T) {
	var ts *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token":"tok"}`)
	})
	mux.HandleFunc("/v2/r/manifests/t", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/auth",service="s"`, ts.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", good)
	})
	ts = httptest.NewServer(mux)
	defer ts.Close()
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client()}
	if d, err := c.Digest(context.Background(), "r", "t"); err != nil || d != good {
		t.Fatalf("digest = %q, %v", d, err)
	}
}

func TestBearerRealmOnOtherHostRefused(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="https://evil.example.com/token"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client()}
	if _, err := c.Digest(context.Background(), "r", "t"); err == nil || !strings.Contains(err.Error(), "refusing token realm") {
		t.Fatalf("err = %v", err)
	}
}

func TestBearerRealmOnAllowedTokenHost(t *testing.T) {
	// The realm lives on a different host than the registry, as with SUSE/SCC.
	auth := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token":"tok"}`)
	}))
	defer auth.Close()
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/api/registry/authorize",service="SUSE Linux Docker Registry"`, auth.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", good)
	}))
	defer reg.Close()

	authHost := strings.TrimPrefix(auth.URL, "https://")
	// httptest TLS servers share one cert pool via any server's client.
	c := &Client{BaseURL: reg.URL, HTTP: reg.Client(), TokenHosts: []string{authHost}}
	if d, err := c.Digest(context.Background(), "r", "t"); err != nil || d != good {
		t.Fatalf("digest = %q, %v", d, err)
	}

	c.TokenHosts = nil
	if _, err := c.Digest(context.Background(), "r", "t"); err == nil || !strings.Contains(err.Error(), "refusing token realm") {
		t.Fatalf("unlisted token host should be refused, err = %v", err)
	}
}

func TestSUSERealmAllowed(t *testing.T) {
	c, err := New("registry.suse.com")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(c.TokenHosts, "scc.suse.com") {
		t.Fatalf("scc.suse.com must be an allowed token host, got %v", c.TokenHosts)
	}
}

func TestDigestMissingHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client()}
	if _, err := c.Digest(context.Background(), "r", "t"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_AppCoUsesEnvCredentials(t *testing.T) {
	t.Setenv("APPCO_USERNAME", "user")
	t.Setenv("APPCO_PASSWORD", "s3cret")
	c, err := New("dp.apps.rancher.io")
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://dp.apps.rancher.io" || c.Username != "user" || c.Password != "s3cret" {
		t.Fatalf("got %+v", c)
	}
}

// basicRegistry answers 401 with a Basic challenge until valid credentials are
// presented, like dp.apps.rancher.io.
func basicRegistry(t *testing.T, user, pass string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Realm"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"tags":["1.27.3-dev-14.2"]}`)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestBasicChallengeWithCredentials(t *testing.T) {
	ts := basicRegistry(t, "user", "s3cret")
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client(), Username: "user", Password: "s3cret"}
	tags, err := c.Tags(context.Background(), "containers/go")
	if err != nil || !slices.Equal(tags, []string{"1.27.3-dev-14.2"}) {
		t.Fatalf("tags = %v, err = %v", tags, err)
	}
}

func TestBasicChallengeWithoutCredentials(t *testing.T) {
	ts := basicRegistry(t, "user", "s3cret")
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client()}
	_, err := c.Tags(context.Background(), "containers/go")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("want an error mentioning credentials, got %v", err)
	}
}

func TestBasicChallengeWrongCredentialsFail(t *testing.T) {
	ts := basicRegistry(t, "user", "s3cret")
	c := &Client{BaseURL: ts.URL, HTTP: ts.Client(), Username: "user", Password: "wrong"}
	if _, err := c.Tags(context.Background(), "containers/go"); err == nil {
		t.Fatal("wrong credentials must fail")
	}
}

// Credentials must never reach a host other than the registry's own, even if
// the registry redirects there.
func TestBasicCredentialsNotSentOffHost(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
	}))
	defer other.Close()
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Realm"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer reg.Close()
	c := &Client{BaseURL: reg.URL, Username: "user", Password: "s3cret"}
	c.HTTP = &http.Client{CheckRedirect: c.checkRedirect}

	if _, err := c.Tags(context.Background(), "containers/go"); err == nil {
		t.Fatal("redirect off the registry host must be refused")
	}
	if leaked {
		t.Fatal("credentials were sent to another host")
	}
}
