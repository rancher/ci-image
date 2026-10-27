// Package registry is a minimal, read-only OCI registry client: it lists tags
// and resolves a tag to its manifest digest. It exists so deps.yaml base image
// references can be refreshed without pulling in a container library.
//
// Only registries on the compile-time allowlist are contacted.
package registry

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// registryConfig describes how to talk to an allowlisted registry.
type registryConfig struct {
	// tokenHosts are the extra hosts the registry may delegate token issuance
	// to (its own host is always accepted). A challenge can name any realm, so
	// tokens are only requested from hosts listed here.
	tokenHosts []string
	// usernameEnv and passwordEnv name the environment variables holding
	// basic-auth credentials, for registries that require them.
	usernameEnv, passwordEnv string
}

// allowedRegistries is the compile-time allowlist of registries this client
// will talk to. Adding an entry requires a code change visible in PR review.
var allowedRegistries = map[string]registryConfig{
	// registry.suse.com issues anonymous pull tokens via SCC.
	"registry.suse.com": {tokenHosts: []string{"scc.suse.com"}},
	// dp.apps.rancher.io (Application Collection) answers with a Basic challenge.
	// The password is a user access token or a service account secret.
	"dp.apps.rancher.io": {usernameEnv: "APPCO_USERNAME", passwordEnv: "APPCO_PASSWORD"},
}

const (
	userAgent = "ci-image-builder/1 (+https://github.com/rancher/ci-image)"
	maxPages  = 50

	acceptManifests = "application/vnd.oci.image.index.v1+json, " +
		"application/vnd.docker.distribution.manifest.list.v2+json, " +
		"application/vnd.oci.image.manifest.v1+json, " +
		"application/vnd.docker.distribution.manifest.v2+json"
)

var digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Ref is a parsed base image reference: registry/repo[:tag][@digest].
type Ref struct {
	Registry string
	Repo     string
	Tag      string
	Digest   string
}

// ParseRef parses "registry.suse.com/bci/golang:1.27.2@sha256:...".
func ParseRef(s string) (Ref, error) {
	var r Ref
	rest, digest, hasDigest := strings.Cut(s, "@")
	if hasDigest {
		if !digestRe.MatchString(digest) {
			return r, fmt.Errorf("%q: malformed digest", s)
		}
		r.Digest = digest
	}
	host, path, ok := strings.Cut(rest, "/")
	if !ok || !strings.ContainsAny(host, ".:") {
		return r, fmt.Errorf("%q: expected registry/repo[:tag][@digest]", s)
	}
	r.Registry = host
	// A tag colon can only follow the last path segment.
	if i := strings.LastIndex(path, ":"); i > strings.LastIndex(path, "/") {
		r.Repo, r.Tag = path[:i], path[i+1:]
	} else {
		r.Repo = path
	}
	if r.Repo == "" {
		return r, fmt.Errorf("%q: missing repository", s)
	}
	return r, nil
}

// String renders the reference in the form used by deps.yaml.
func (r Ref) String() string {
	s := r.Registry + "/" + r.Repo
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// Client talks to a single registry.
type Client struct {
	// BaseURL is the registry root, e.g. https://registry.suse.com.
	BaseURL string
	HTTP    *http.Client
	// TokenHosts are additional hosts, besides BaseURL's, that may issue tokens.
	TokenHosts []string
	// Username and Password are basic-auth credentials, for registries that
	// require them. Only ever sent to BaseURL's host.
	Username, Password string

	// credsHint tells the user where to supply credentials when a registry
	// asks for them and none are set.
	credsHint string
}

// New returns a client for an allowlisted registry host.
func New(host string) (*Client, error) {
	cfg, ok := allowedRegistries[host]
	if !ok {
		return nil, fmt.Errorf("registry %q is not on the allowlist", host)
	}
	c := &Client{
		BaseURL:    "https://" + host,
		TokenHosts: cfg.tokenHosts,
	}
	if cfg.usernameEnv != "" {
		c.Username = os.Getenv(cfg.usernameEnv)
		c.Password = os.Getenv(cfg.passwordEnv)
		c.credsHint = fmt.Sprintf("set %s and %s", cfg.usernameEnv, cfg.passwordEnv)
	}
	c.HTTP = &http.Client{Timeout: 60 * time.Second, CheckRedirect: c.checkRedirect}
	return c, nil
}

// maxRedirects bounds redirect chains, matching net/http's default.
const maxRedirects = 10

// checkRedirect revalidates every redirect destination against the allowlist.
// Requests that began on the registry host may only stay on that host; token
// requests (which began elsewhere) may also reach the registry or another
// allowed token host. The scheme may never be downgraded from the base's.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	dest := req.URL
	if dest.Scheme != base.Scheme {
		return fmt.Errorf("refusing redirect to %s: scheme differs from %s", dest.Redacted(), base.Scheme)
	}
	allowed := dest.Host == base.Host
	if via[0].URL.Host != base.Host {
		allowed = allowed || slices.Contains(c.TokenHosts, dest.Host)
	}
	if !allowed {
		return fmt.Errorf("refusing redirect from %s to %s: host not allowed", via[len(via)-1].URL.Host, dest.Host)
	}
	return nil
}

// Tags returns every tag of repo, following pagination.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	next := fmt.Sprintf("/v2/%s/tags/list?n=1000", repo)
	var all []string
	for range maxPages {
		resp, err := c.do(ctx, http.MethodGet, next, "")
		if err != nil {
			return nil, err
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding tag list for %s: %w", repo, err)
		}
		all = append(all, body.Tags...)
		next = nextLink(resp.Header.Get("Link"))
		if next == "" {
			return all, nil
		}
	}
	return nil, fmt.Errorf("tag list for %s exceeds %d pages", repo, maxPages)
}

// Digest resolves repo:tag to its manifest (or index) digest.
func (c *Client) Digest(ctx context.Context, repo, tag string) (string, error) {
	resp, err := c.do(ctx, http.MethodHead, fmt.Sprintf("/v2/%s/manifests/%s", repo, tag), acceptManifests)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	d := resp.Header.Get("Docker-Content-Digest")
	if !digestRe.MatchString(d) {
		return "", fmt.Errorf("%s:%s: registry returned no usable Docker-Content-Digest (%q)", repo, tag, d)
	}
	return d, nil
}

// nextLink extracts the target of a rel="next" Link header, or "".
func nextLink(h string) string {
	for part := range strings.SplitSeq(h, ",") {
		target, params, ok := strings.Cut(part, ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(target), "<>")
	}
	return ""
}

// do issues a request, answering a single Bearer challenge if the registry
// issues one (anonymous pull tokens).
func (c *Client) do(ctx context.Context, method, path, accept string) (*http.Response, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	target, err := base.Parse(path)
	if err != nil {
		return nil, err
	}
	if target.Host != base.Host || target.Scheme != base.Scheme {
		return nil, fmt.Errorf("registry pagination left %s://%s", base.Scheme, base.Host)
	}

	token := ""
	basic := false
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else if basic {
			// Only ever sent to base's host: the host check above covers the
			// request, and checkRedirect refuses any redirect off it.
			req.SetBasicAuth(c.Username, c.Password)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, target, err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || attempt == 1 {
			return nil, fmt.Errorf("%s %s: unexpected status %d", method, target, resp.StatusCode)
		}
		if strings.HasPrefix(strings.ToLower(challenge), "basic ") {
			if c.Username == "" || c.Password == "" {
				return nil, fmt.Errorf("%s requires credentials: %s", base.Host, cmp.Or(c.credsHint, "none configured"))
			}
			basic = true
			continue
		}
		if token, err = c.fetchToken(ctx, base, challenge); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s %s: authentication failed", method, target)
}

var challengeParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func (c *Client) fetchToken(ctx context.Context, base *url.URL, challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("unsupported auth challenge %q", challenge)
	}
	params := map[string]string{}
	for _, m := range challengeParamRe.FindAllStringSubmatch(challenge, -1) {
		params[m[1]] = m[2]
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || (realm.Host != base.Host && !slices.Contains(c.TokenHosts, realm.Host)) ||
		(realm.Scheme != "https" && base.Scheme == "https") {
		return "", fmt.Errorf("refusing token realm %q: not an allowed token host for %s", params["realm"], base.Host)
	}
	q := realm.Query()
	for _, k := range []string{"service", "scope"} {
		if v := params[k]; v != "" {
			q.Set(k, v)
		}
	}
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("token response contained no token")
}
