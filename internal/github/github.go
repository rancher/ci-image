package github

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	// apiBase is the GitHub API base, overridable in tests.
	apiBase           = "https://api.github.com"
	allowedHostSuffix = "github.com"
)

type release struct {
	TagName string `json:"tag_name"`
}

// httpClient is a package-level client with a 60-second timeout so hung
// connections do not stall generate/update indefinitely.
var httpClient = &http.Client{Timeout: 60 * time.Second}

const userAgent = "ci-image-builder/1 (+https://github.com/rancher/ci-image)"

// ParseSourceRepo extracts owner and repo from an org/repo shorthand or a full
// https://github.com/org/repo URL. Returns an error for non-GitHub sources so
// callers can gracefully skip auto-version-resolution.
func ParseSourceRepo(source string) (owner, repo string, err error) {
	var path string
	switch {
	case strings.HasPrefix(source, "https://github.com/"):
		path = strings.TrimPrefix(source, "https://github.com/")
	case strings.HasPrefix(source, "http://github.com/"):
		path = strings.TrimPrefix(source, "http://github.com/")
	case strings.Contains(source, "://"):
		return "", "", fmt.Errorf("%q is not a GitHub URL", source)
	default:
		path = source
	}
	if strings.Count(path, "/") != 1 {
		return "", "", fmt.Errorf("cannot parse %q as org/repo or https://github.com/org/repo", source)
	}
	parts := strings.SplitN(path, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse %q as org/repo or https://github.com/org/repo", source)
	}
	return parts[0], parts[1], nil
}

// LatestReleaseTag returns the tag name of the latest release for the given
// org/repo. Respects the GITHUB_TOKEN environment variable for auth.
func LatestReleaseTag(owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, owner, repo)
	resp, err := doGet(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API %s: unexpected status %d", url, resp.StatusCode)
	}

	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decoding GitHub API response: %w", err)
	}
	if r.TagName == "" {
		return "", fmt.Errorf("GitHub API returned empty tag_name for %s/%s", owner, repo)
	}
	return r.TagName, nil
}

// DownloadAndHash downloads the asset at url, streams it through a SHA-256
// hasher, and returns the lowercase hex digest. The asset body is discarded.
func DownloadAndHash(url string) (string, error) {
	resp, err := doGet(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: unexpected status %d", url, resp.StatusCode)
	}

	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return "", fmt.Errorf("hashing %s: %w", url, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FetchChecksumFile downloads a SHA-256 checksum file and returns a map of
// filename → lowercase hex SHA-256. Lines are expected in the standard
// `sha256sum` format: "<hash>  <filename>" (one or two spaces). Blank lines
// and lines beginning with '#' are ignored.
func FetchChecksumFile(url string) (map[string]string, error) {
	resp, err := doGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching checksum file %s: unexpected status %d", url, resp.StatusCode)
	}

	result := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		checksum := strings.ToLower(fields[0])
		if len(checksum) != 64 || !isHex(checksum) {
			continue // skip malformed lines silently
		}
		var filename string
		if len(fields) >= 2 {
			filename = fields[len(fields)-1]
		}
		result[filename] = checksum
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading checksum file %s: %w", url, err)
	}
	return result, nil
}

// isHex reports whether s consists entirely of lowercase hexadecimal digits.
func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// doGet performs an authenticated GET request, adding Authorization header
// if GITHUB_TOKEN is set.
func doGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if hostErr := validateHost(req.URL.Host); hostErr != nil {
		return nil, hostErr
	}

	// #nosec G107 - Host is validated against github.com domain suffix or test apiBase
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return resp, nil
}

func validateHost(host string) error {
	if strings.HasSuffix(host, allowedHostSuffix) {
		return nil
	}
	return fmt.Errorf("unauthorized host: %s", host)
}
