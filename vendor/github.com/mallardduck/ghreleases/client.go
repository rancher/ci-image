package ghrelease

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Client handles GitHub API interactions.
type Client struct {
	httpClient *http.Client
	token      string // optional GITHUB_TOKEN
	baseURL    string // defaults to "https://api.github.com"

	// allowedHosts, when non-empty, restricts every request (and redirect) to
	// these hosts and their subdomains, plus the base URL host.
	allowedHosts []string

	manifestVerifier SignatureVerifier
	// manifestVerifierErr is set when WithManifestVerifier was given a nil
	// verifier; every manifest fetch then fails closed.
	manifestVerifierErr error

	artifactVerifier SignatureVerifier
	// artifactVerifierErr is set when WithArtifactVerifier was given a nil
	// verifier; every download then fails closed.
	artifactVerifierErr error
}

// Option configures a Client. Pass options to NewClient or NewClientWithHTTP.
type Option func(*Client)

// WithBaseURL overrides the GitHub API base URL (default "https://api.github.com").
// It is mainly useful for GitHub Enterprise and for tests. The base URL host is
// always allowed by WithAllowedHosts and always receives the token.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithAllowedHosts restricts the client to the given hosts. Requests, and
// redirects, to any other host fail with ErrHostNotAllowed. An entry matches
// the exact host or any of its subdomains ("github.com" allows
// "api.github.com"). The base URL host is always allowed. By default the
// client is unrestricted.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *Client) {
		for _, h := range hosts {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				c.allowedHosts = append(c.allowedHosts, h)
			}
		}
	}
}

// SignatureVerifier authenticates content against a detached signature, for
// example a Sigstore bundle or an OpenPGP .asc file. A checksum file published
// next to a binary proves integrity but not authenticity; verifying a signature
// closes that gap, whether the content is the checksum manifest
// (WithManifestVerifier) or the downloaded asset itself (WithArtifactVerifier).
// One implementation can serve both roles.
type SignatureVerifier interface {
	// SignatureName returns the asset name of the detached signature for the
	// asset named assetName (e.g. "checksums.txt" -> "checksums.txt.sigstore.json",
	// "tool.tar.gz" -> "tool.tar.gz.asc").
	SignatureName(assetName string) string

	// Verify reports whether signature authenticates exactly the bytes read from
	// content. It must read content to EOF; returning nil having read less is
	// treated as a failure. Any non-nil error rejects the content.
	Verify(ctx context.Context, content io.Reader, signature []byte) error
}

// WithManifestVerifier makes FetchChecksums, FetchChecksumsLenient and
// FetchMultiHashChecksums authenticate each checksum manifest before parsing
// it. The signature is located via v.SignatureName (or WithSignatureURL), and a
// missing signature fails with ErrSignatureMissing, a rejected one with
// ErrSignatureInvalid, so enabling a verifier can never silently fall back to
// unauthenticated checksums. Without this option behavior is unchanged.
func WithManifestVerifier(v SignatureVerifier) Option {
	return func(c *Client) {
		c.manifestVerifier, c.manifestVerifierErr = v, nil
		if isNilInterface(v) {
			c.manifestVerifierErr = &wrapped{sentinel: ErrInvalidVerifier, cause: errors.New("WithManifestVerifier was given a nil verifier")}
		}
	}
}

// isNilInterface reports whether v is nil or an interface holding a nil
// pointer, map, slice, func or chan - for example the result of ignoring the
// error from a verifier constructor.
func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// WithArtifactVerifier makes Download and DownloadToBytes
// authenticate each asset against its detached signature. The signature is
// fetched before the asset, from DownloadOptions.SignatureURL or, failing that,
// next to the asset via v.SignatureName. A missing signature fails with
// ErrSignatureMissing and a rejected one with ErrSignatureInvalid; verification
// never silently falls back to an unauthenticated download. Without this option
// behavior is unchanged. Independent of WithManifestVerifier: use either or both.
//
// While a download is being verified the destination receives no bytes (see
// DownloadOptions.StreamUnverified).
func WithArtifactVerifier(v SignatureVerifier) Option {
	return func(c *Client) {
		c.artifactVerifier, c.artifactVerifierErr = v, nil
		if isNilInterface(v) {
			c.artifactVerifierErr = &wrapped{sentinel: ErrInvalidVerifier, cause: errors.New("WithArtifactVerifier was given a nil verifier")}
		}
	}
}

type signatureURLKey struct{}

// WithSignatureURL returns a context telling the FetchChecksums* methods
// exactly where the manifest's detached signature lives. Use it when the
// manifest URL is a GitHub API asset URL (".../releases/assets/<id>"), where
// the signature cannot be found by name: look it up in Release.Assets using
// SignatureVerifier.SignatureName and pass its URL here. Without it the
// signature is expected next to the manifest, at the same URL with the last
// path segment replaced by SignatureName.
func WithSignatureURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, signatureURLKey{}, url)
}

// siblingURL replaces the last path segment of rawURL with name.
func siblingURL(rawURL, name string) string {
	q := ""
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL, q = rawURL[:i], rawURL[i:]
	}
	if i := strings.LastIndex(rawURL, "/"); i >= 0 {
		rawURL = rawURL[:i+1]
	}
	return rawURL + name + q
}

// Release represents a GitHub release.
type Release struct {
	ID          int64
	TagName     string
	Name        string
	Draft       bool
	Prerelease  bool
	PublishedAt time.Time
	// Body is the release notes text. It is populated by the single-release
	// and latest-release lookups as well as by listings.
	Body   string
	Assets []Asset
}

// Asset represents a downloadable release asset.
type Asset struct {
	Name string
	URL  string // API URL for downloading
	Size int64
}

// ListOptions configures pagination and fetch-time filtering for release listing functions.
//
// Fetch-time filters (TagPrefix, ExcludePrereleases, ExcludeDrafts, Since) are applied
// per-release during iteration so callers receive a filtered slice without a separate
// post-processing step. Since additionally enables early-exit pagination in
// ListAllReleases (see field doc below).
type ListOptions struct {
	// Page is the 1-based page number for ListReleases. Values of 0 and 1 both
	// select the first page. Ignored by ListAllReleases, which always starts at
	// page 1 and paginates automatically.
	Page int

	// PerPage is the number of releases to request per page (max 100). A value of
	// 0 uses the GitHub default of 30. Capped to 100 if a larger value is supplied.
	// For ListAllReleases, 100 is always used regardless of this field.
	PerPage int

	// TagPrefix filters releases to those whose TagName starts with this string.
	// An empty string disables prefix filtering and matches all releases.
	// Useful for monorepos where sub-package releases share a common prefix
	// (e.g. "database/" for tags like "database/v1.2.3").
	TagPrefix string

	// ExcludePrereleases drops releases marked as prerelease by GitHub.
	// Equivalent to calling ExcludePrereleases() on the result, but applied
	// during fetch so excluded entries never enter the returned slice.
	ExcludePrereleases bool

	// ExcludeDrafts drops releases marked as draft by GitHub.
	// Equivalent to calling ExcludeDrafts() on the result, but applied
	// during fetch so excluded entries never enter the returned slice.
	ExcludeDrafts bool

	// Since filters out releases published before this time and, in
	// ListAllReleases, stops pagination entirely once a release older than Since
	// is encountered. Because GitHub returns releases newest-first, any release
	// older than Since guarantees that all subsequent pages are also older, making
	// early exit safe and efficient.
	//
	// This is the recommended way to bound a ListAllReleases call to a time window
	// (e.g. "all releases since the start of last month") without fetching the
	// entire release history. Use FilterByDateRange for an upper-bound filter after
	// the fetch, since an upper bound cannot trigger early exit.
	//
	// A zero value disables this filter.
	Since time.Time
}

// ErrRateLimited is the sentinel matched by errors.Is when the GitHub API rate
// limit is exceeded. Type-assert to *RateLimitError to access RetryAfter.
var ErrRateLimited = errors.New("GitHub API rate limit exceeded")

// RateLimitError is returned when the GitHub API responds with a rate-limit status
// (HTTP 429, or HTTP 403 with X-RateLimit-Remaining: 0).
// RetryAfter is the time at which the limit resets; it may be zero if the API did
// not supply a reset timestamp.
//
// Use errors.Is(err, ErrRateLimited) to test for rate limiting, or type-assert to
// *RateLimitError to read RetryAfter:
//
//	var rl *ghrelease.RateLimitError
//	if errors.As(err, &rl) && !rl.RetryAfter.IsZero() {
//	    time.Sleep(time.Until(rl.RetryAfter))
//	}
type RateLimitError struct {
	RetryAfter time.Time
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter.IsZero() {
		return ErrRateLimited.Error()
	}
	return ErrRateLimited.Error() + "; retry after " + e.RetryAfter.Format(time.RFC3339)
}

// Is reports whether target matches ErrRateLimited.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// NewClient creates a GitHub API client.
// Token is optional but recommended to avoid rate limits.
// If token is empty, tries GITHUB_TOKEN environment variable.
//
// The default HTTP client enforces dial, TLS and response-header timeouts but no
// total timeout, so large downloads are bounded only by the caller's context.
func NewClient(token string, opts ...Option) *Client {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second, // time to establish TCP connection
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second, // time to receive first response byte
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	return newClient(httpClient, token, opts)
}

// NewClientWithHTTP creates a client with a custom HTTP client.
// Allows callers to configure timeouts, proxies, etc.
func NewClientWithHTTP(httpClient *http.Client, token string, opts ...Option) *Client {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	return newClient(httpClient, token, opts)
}

func newClient(httpClient *http.Client, token string, opts []Option) *Client {
	c := &Client{
		httpClient: httpClient,
		token:      token,
		baseURL:    "https://api.github.com",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// LatestRelease fetches the latest release tag for a repository.
//
// By default it calls GitHub's /releases/latest endpoint, which returns the
// most recently published non-prerelease, non-draft release regardless of tag
// format. For repositories that maintain multiple release lines under distinct
// tag prefixes (e.g. "v*" for a library and "cli/v*" for a CLI), attach a
// ReleaseFilter to the context with WithTagPrefix so that only releases
// matching the desired prefix are considered.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (string, error) {
	rel, err := c.LatestReleaseInfo(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return rel.TagName, nil
}

// LatestReleaseInfo is LatestRelease that returns the whole release, not just
// its tag, so callers can read Prerelease, PublishedAt, Body and Assets without
// a second request. It honours the same context filters as LatestRelease
// (WithTagPrefix, WithPrereleases).
func (c *Client) LatestReleaseInfo(ctx context.Context, owner, repo string) (*Release, error) {
	filter, _ := releaseFilterFromContext(ctx)
	if filter.IncludePrereleases {
		return c.highestRelease(ctx, owner, repo, filter)
	}
	if filter.TagPrefix != "" {
		return c.latestReleaseWithFilter(ctx, owner, repo, filter)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo)

	var raw apiRelease
	if err := c.doGet(ctx, url, &raw); err != nil {
		return nil, wrapFetchErr(err)
	}

	return raw.toRelease(), nil
}

// latestReleaseWithFilter lists releases in descending publish order and
// returns the first non-draft, non-prerelease one whose tag starts with
// filter.TagPrefix. It paginates until a match is found or an empty page
// signals exhaustion.
func (c *Client) latestReleaseWithFilter(ctx context.Context, owner, repo string, filter ReleaseFilter) (*Release, error) {
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/releases?page=%d&per_page=100", c.baseURL, owner, repo, page)
		var raw []apiRelease
		if err := c.doGet(ctx, url, &raw); err != nil {
			return nil, wrapFetchErr(err)
		}
		if len(raw) == 0 {
			break
		}
		for _, r := range raw {
			rel := r.toRelease()
			if rel.Draft || rel.Prerelease {
				continue
			}
			if strings.HasPrefix(rel.TagName, filter.TagPrefix) {
				return rel, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: no release with tag prefix %q in %s/%s", ErrReleaseNotFound, filter.TagPrefix, owner, repo)
}

// highestRelease scans every page of non-draft releases matching the filter
// prefix and returns the tag with the highest semver precedence. Prereleases
// are included. Non-semver tags are ignored.
func (c *Client) highestRelease(ctx context.Context, owner, repo string, filter ReleaseFilter) (*Release, error) {
	var best *Release
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/releases?page=%d&per_page=100", c.baseURL, owner, repo, page)
		var raw []apiRelease
		if err := c.doGet(ctx, url, &raw); err != nil {
			return nil, wrapFetchErr(err)
		}
		if len(raw) == 0 {
			break
		}
		for _, r := range raw {
			if r.Draft || !strings.HasPrefix(r.TagName, filter.TagPrefix) {
				continue
			}
			if _, ok := parseVersion(r.TagName); !ok {
				continue
			}
			if best == nil {
				best = r.toRelease()
				continue
			}
			if cmp, _ := CompareVersions(r.TagName, best.TagName); cmp > 0 {
				best = r.toRelease()
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: no semver release in %s/%s", ErrReleaseNotFound, owner, repo)
	}
	return best, nil
}

// GetRelease fetches a specific release by tag.
func (c *Client) GetRelease(ctx context.Context, owner, repo, tag string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.baseURL, owner, repo, tag)

	var raw apiRelease
	if err := c.doGet(ctx, url, &raw); err != nil {
		return nil, wrapFetchErr(err)
	}

	return raw.toRelease(), nil
}

// ResolveTagCommit resolves a tag (or any other Git ref, such as a branch name)
// to the full SHA of the commit it currently points to.
//
// This is the correct way to pin a dependency (e.g. a GitHub Action) to an
// immutable commit: Release.TagName only gives you the tag, and the GitHub API's
// target_commitish field on a release is not a reliable source for this — GitHub
// documents it as "unused if the Git tag already exists," and in practice it is
// often just the branch the release was cut from (e.g. "main") rather than the
// commit the tag resolves to today.
//
// ResolveTagCommit works for both lightweight and annotated tags, since GitHub's
// commits API dereferences annotated tag objects to the underlying commit
// automatically.
func (c *Client) ResolveTagCommit(ctx context.Context, owner, repo, ref string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.baseURL, owner, repo, ref)

	var commit struct {
		SHA string `json:"sha"`
	}

	if err := c.doGet(ctx, url, &commit); err != nil {
		var rl *RateLimitError
		if errors.As(err, &rl) {
			return "", rl
		}
		return "", &wrapped{sentinel: ErrRefNotFound, cause: err}
	}

	return commit.SHA, nil
}

// ListReleases fetches one page of releases for a repository, newest first.
// Any filter fields set in opts are applied to the returned slice.
func (c *Client) ListReleases(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Release, error) {
	page, perPage := 1, 30
	if opts != nil {
		if opts.Page > 1 {
			page = opts.Page
		}
		if opts.PerPage > 0 {
			perPage = opts.PerPage
			if perPage > 100 {
				perPage = 100
			}
		}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases?page=%d&per_page=%d", c.baseURL, owner, repo, page, perPage)

	var raw []apiRelease
	if err := c.doGet(ctx, url, &raw); err != nil {
		return nil, wrapFetchErr(err)
	}

	releases := make([]*Release, 0, len(raw))
	for _, r := range raw {
		rel := r.toRelease()
		if matchesOpts(rel, opts) {
			releases = append(releases, rel)
		}
	}
	return releases, nil
}

// ListAllReleases fetches every release for a repository by paginating until exhausted,
// applying any filter fields in opts during iteration. Releases are returned newest first.
//
// When opts.Since is set, pagination stops as soon as a release's PublishedAt is before
// that time — since the API returns newest-first, all remaining pages would be older too.
//
// If the GitHub API rate limit is exceeded mid-pagination, a *RateLimitError is returned
// containing the time at which the limit resets. Callers can use errors.As to retrieve
// it and schedule a retry after that time.
func (c *Client) ListAllReleases(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Release, error) {
	var all []*Release
	for pageNum := 1; ; pageNum++ {
		var raw []apiRelease
		url := fmt.Sprintf("%s/repos/%s/%s/releases?page=%d&per_page=%d", c.baseURL, owner, repo, pageNum, 100)
		if err := c.doGet(ctx, url, &raw); err != nil {
			return nil, wrapFetchErr(err)
		}

		done := false
		for _, r := range raw {
			rel := r.toRelease()
			if opts != nil && !opts.Since.IsZero() && rel.PublishedAt.Before(opts.Since) {
				done = true
				break
			}
			if matchesOpts(rel, opts) {
				all = append(all, rel)
			}
		}
		if done || len(raw) < 100 {
			break
		}
	}
	return all, nil
}

// matchesOpts returns true if r passes all filter criteria in opts.
// A nil opts matches everything.
func matchesOpts(r *Release, opts *ListOptions) bool {
	if opts == nil {
		return true
	}
	if opts.TagPrefix != "" && !strings.HasPrefix(r.TagName, opts.TagPrefix) {
		return false
	}
	if opts.ExcludePrereleases && r.Prerelease {
		return false
	}
	if opts.ExcludeDrafts && r.Draft {
		return false
	}
	if !opts.Since.IsZero() && r.PublishedAt.Before(opts.Since) {
		return false
	}
	return true
}

// apiRelease is the raw GitHub API shape shared by single-release and list endpoints.
type apiRelease struct {
	ID          int64  `json:"id"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (r apiRelease) toRelease() *Release {
	published, _ := time.Parse(time.RFC3339, r.PublishedAt)
	rel := &Release{
		ID:          r.ID,
		TagName:     r.TagName,
		Name:        r.Name,
		Body:        r.Body,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		PublishedAt: published,
		Assets:      make([]Asset, len(r.Assets)),
	}
	for i, a := range r.Assets {
		rel.Assets[i] = Asset{Name: a.Name, URL: a.URL, Size: a.Size}
	}
	return rel
}

// FetchChecksums downloads and parses a checksum file strictly (see ParseChecksumFile).
func (c *Client) FetchChecksums(ctx context.Context, url string) (map[string]string, error) {
	body, err := c.fetchManifest(ctx, url)
	if err != nil {
		return nil, err
	}
	return ParseChecksumFile(bytes.NewReader(body))
}

// FetchChecksumsLenient downloads and parses a checksum file leniently
// (see ParseChecksumFileLenient).
func (c *Client) FetchChecksumsLenient(ctx context.Context, url string) (map[string]string, error) {
	body, err := c.fetchManifest(ctx, url)
	if err != nil {
		return nil, err
	}
	return ParseChecksumFileLenient(bytes.NewReader(body))
}

// FetchMultiHashChecksums downloads a multi-hash checksum file and its sibling
// hash-order file, then returns the SHA-256 column (see ParseMultiHashChecksumFile).
//
// Only the checksum file is authenticated by WithManifestVerifier; the hash-order
// file is fetched unverified, since such files are not normally signed. It only
// picks which column is read, and every column lives in the authenticated
// checksum file, so tampering with it selects a different hash for the same
// asset: a download checked against it fails with ErrChecksumMismatch, it cannot
// make a forged asset pass.
func (c *Client) FetchMultiHashChecksums(ctx context.Context, checksumURL, orderURL string) (map[string]string, error) {
	body, err := c.fetchManifest(ctx, checksumURL)
	if err != nil {
		return nil, err
	}
	orderBody, err := c.fetchBody(ctx, orderURL)
	if err != nil {
		return nil, err
	}
	order, err := ParseHashOrder(bytes.NewReader(orderBody))
	if err != nil {
		return nil, err
	}
	return ParseMultiHashChecksumFile(bytes.NewReader(body), order)
}

// fetchManifest fetches a checksum manifest and, when a SignatureVerifier is
// configured, authenticates it against its detached signature.
func (c *Client) fetchManifest(ctx context.Context, url string) ([]byte, error) {
	if c.manifestVerifierErr != nil {
		return nil, c.manifestVerifierErr
	}
	body, err := c.fetchBody(ctx, url)
	if err != nil {
		return nil, err
	}
	if c.manifestVerifier == nil {
		return body, nil
	}
	sigURL, _ := ctx.Value(signatureURLKey{}).(string)
	if sigURL == "" {
		sigURL = siblingURL(url, c.manifestVerifier.SignatureName(lastPathSegment(url)))
	}
	sig, err := c.fetchSignature(ctx, sigURL)
	if err != nil {
		return nil, err
	}
	// The manifest is in memory, so count what the verifier reads: accepting
	// after reading less than all of it authenticates nothing.
	cr := &countingReader{r: bytes.NewReader(body)}
	if err := safeVerify(ctx, c.manifestVerifier, cr, sig); err != nil {
		return nil, &wrapped{sentinel: ErrSignatureInvalid, cause: err}
	}
	if cr.n != int64(len(body)) {
		return nil, &wrapped{sentinel: ErrSignatureInvalid, cause: errVerifierFinished}
	}
	return body, nil
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// safeVerify calls v.Verify, turning a panic into an error: a panicking
// verifier must fail verification, not crash the caller or let content through.
func safeVerify(ctx context.Context, v SignatureVerifier, content io.Reader, sig []byte) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("verifier panicked: %v", p)
		}
	}()
	return v.Verify(ctx, content, sig)
}

// fetchSignature fetches a detached signature. A 404 means the release has no
// signature, which is reported as ErrSignatureMissing so that enabling a
// verifier can never degrade into an unauthenticated fetch.
func (c *Client) fetchSignature(ctx context.Context, sigURL string) ([]byte, error) {
	sig, err := c.fetchBody(ctx, sigURL)
	if err != nil {
		var st *HTTPStatusError
		if errors.As(err, &st) && st.StatusCode == http.StatusNotFound {
			return nil, &wrapped{sentinel: ErrSignatureMissing, cause: err}
		}
		return nil, err
	}
	return sig, nil
}

// lastPathSegment returns the final path segment of rawURL, ignoring any query
// string or fragment.
func lastPathSegment(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	return rawURL[strings.LastIndex(rawURL, "/")+1:]
}

// fetchBody GETs url and returns the body, applying the default API timeout when
// ctx has no deadline. Non-200 responses return *HTTPStatusError.
func (c *Client) fetchBody(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()

	req, err := c.newRequest(ctx, url, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) doGet(ctx context.Context, url string, v interface{}) error {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()

	req, err := c.newRequest(ctx, url, "application/vnd.github.v3+json")
	if err != nil {
		return err
	}
	resp, err := c.send(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(v)
	case http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: rateLimitReset(resp)}
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return &RateLimitError{RetryAfter: rateLimitReset(resp)}
		}
	}
	return &HTTPStatusError{URL: url, StatusCode: resp.StatusCode}
}

// apiTimeout bounds API and checksum calls whose context has no deadline.
const apiTimeout = 30 * time.Second

func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, apiTimeout)
}

// newRequest builds a GET request, enforcing the host allowlist and attaching
// the token only for GitHub-owned hosts (or the API base host).
func (c *Client) newRequest(ctx context.Context, rawURL, accept string) (*http.Request, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if !c.hostAllowed(u.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrHostNotAllowed, u.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" && c.tokenHost(u.Hostname()) {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", accept)
	return req, nil
}

// send performs req using a per-request copy of the HTTP client whose redirect
// policy enforces the host allowlist and strips the token on redirects to
// non-GitHub hosts. The caller's http.Client is never mutated.
func (c *Client) send(req *http.Request) (*http.Response, error) {
	hc := *c.httpClient
	prev := c.httpClient.CheckRedirect
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		host := r.URL.Hostname()
		if !c.hostAllowed(host) {
			return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
		}
		if !c.tokenHost(host) {
			r.Header.Del("Authorization")
		}
		if prev != nil {
			return prev(r, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return hc.Do(req)
}

func (c *Client) baseHost() string {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// hostAllowed reports whether host may be contacted. Unrestricted by default.
func (c *Client) hostAllowed(host string) bool {
	if len(c.allowedHosts) == 0 {
		return true
	}
	host = strings.ToLower(host)
	if host == c.baseHost() {
		return true
	}
	for _, allowed := range c.allowedHosts {
		if hostMatches(host, allowed) {
			return true
		}
	}
	return false
}

// tokenHost reports whether the token may be sent to host.
func (c *Client) tokenHost(host string) bool {
	host = strings.ToLower(host)
	return host == c.baseHost() ||
		hostMatches(host, "github.com") ||
		hostMatches(host, "githubusercontent.com")
}

// hostMatches reports whether host equals domain or is a subdomain of it,
// matching on a dot boundary ("evilgithub.com" does not match "github.com").
func hostMatches(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// wrapFetchErr wraps a doGet error as ErrReleaseNotFound, unless it is already a
// *RateLimitError — in which case it is returned directly so errors.Is/As callers
// can detect and inspect it without unwrapping through ErrReleaseNotFound.
func wrapFetchErr(err error) error {
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return rl
	}
	return &wrapped{sentinel: ErrReleaseNotFound, cause: err}
}

// rateLimitReset parses the earliest reset time from GitHub rate-limit response headers.
// Checks Retry-After (seconds) then X-RateLimit-Reset (Unix timestamp).
// Returns zero time if neither header is present or parseable.
func rateLimitReset(resp *http.Response) time.Time {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.ParseInt(ra, 10, 64); err == nil {
			return time.Now().Add(time.Duration(secs) * time.Second)
		}
	}
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			return time.Unix(ts, 0)
		}
	}
	return time.Time{}
}
