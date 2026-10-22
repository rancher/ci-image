package resolver

import (
	"net/http"
	"net/url"

	ghrelease "github.com/mallardduck/ghreleases"
)

// rewriteTransport sends every request to target, preserving path and query, so
// both API calls (api.github.com) and asset URLs (github.com) reach the fake
// server while the request URLs still carry real GitHub hosts.
type rewriteTransport struct {
	target *url.URL
	inner  http.RoundTripper
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	clone.Host = r.target.Host
	return r.inner.RoundTrip(clone)
}

// overrideGitHub points the resolver's GitHub client at baseURL (an httptest
// server). The production host allowlist stays enabled so tests exercise the
// same policy as real runs. Returns a restore function for t.Cleanup.
func overrideGitHub(baseURL string) func() {
	target, err := url.Parse(baseURL)
	if err != nil {
		panic("overrideGitHub: invalid URL: " + err.Error())
	}
	hc := &http.Client{Transport: &rewriteTransport{target: target, inner: http.DefaultTransport}}
	c := ghrelease.NewClientWithHTTP(hc, "", ghrelease.WithAllowedHosts(githubHosts...))
	old := ghClient
	ghClient = func() *ghrelease.Client { return c }
	return func() { ghClient = old }
}
