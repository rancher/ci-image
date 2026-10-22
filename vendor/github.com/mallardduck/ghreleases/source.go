package ghrelease

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	httpsSourceRe = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)
	sshSourceRe   = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// ParseSource extracts owner and repo from various GitHub identifiers.
// Supports:
//   - owner/repo
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
func ParseSource(source string) (owner, repo string, err error) {
	source = strings.TrimSpace(source)

	if source == "" {
		return "", "", fmt.Errorf("%w: empty source", ErrInvalidSource)
	}

	// Case 1: HTTPS URLs
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return parseHTTPSSource(source)
	}

	// Case 2: SSH URLs (git@github.com:owner/repo.git)
	if strings.HasPrefix(source, "git@") {
		return parseSSHSource(source)
	}

	// Case 3: Simple owner/repo
	if !strings.Contains(source, "/") {
		return "", "", fmt.Errorf("%w: missing separator", ErrInvalidSource)
	}

	parts := strings.Split(source, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: expected owner/repo", ErrInvalidSource)
	}

	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("%w: empty owner or repo", ErrInvalidSource)
	}

	return owner, repo, nil
}

func parseHTTPSSource(source string) (string, string, error) {
	// Extract path from URL
	// https://github.com/owner/repo -> owner/repo
	// https://github.com/owner/repo.git -> owner/repo
	matches := httpsSourceRe.FindStringSubmatch(source)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("%w: invalid GitHub URL", ErrInvalidSource)
	}

	return matches[1], matches[2], nil
}

func parseSSHSource(source string) (string, string, error) {
	// git@github.com:owner/repo.git -> owner/repo
	matches := sshSourceRe.FindStringSubmatch(source)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("%w: invalid SSH URL", ErrInvalidSource)
	}

	return matches[1], matches[2], nil
}
