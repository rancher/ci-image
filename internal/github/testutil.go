package github

import (
	"net/http"
	"net/url"
)

// OverrideHTTPForTest replaces the package-level HTTP client with one whose
// transport rewrites every outgoing request's host to the given baseURL (e.g.
// an httptest.Server URL), while leaving the allowedHostSuffix check intact so
// github.com URLs continue to pass validation.
//
// Returns a restore function; call it (or pass it to t.Cleanup) when done.
// Intended for use in tests only.
func OverrideHTTPForTest(baseURL string) func() {
	target, err := url.Parse(baseURL)
	if err != nil {
		panic("github.OverrideHTTPForTest: invalid URL: " + err.Error())
	}
	old := httpClient
	httpClient = &http.Client{
		Transport: &redirectTransport{target: target, inner: http.DefaultTransport},
	}
	return func() { httpClient = old }
}

// redirectTransport rewrites every request's scheme and host to target,
// preserving the original path and query. This lets tests intercept all
// outbound HTTP calls — both API calls that use apiBase and asset download
// URLs that contain the real github.com host — without changing allowedHostSuffix.
type redirectTransport struct {
	target *url.URL
	inner  http.RoundTripper
}

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	clone.Host = r.target.Host
	return r.inner.RoundTrip(clone)
}
