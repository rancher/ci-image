package github

import "strings"

// ExpandGitHubTemplate expands a release filename template to a full URL for
// GitHub-sourced tools. If tmpl already contains "://", it is returned as-is
// (already a full URL). If source is a GitHub org/repo (shorthand or full
// https://github.com/org/repo) and tmpl is a bare filename, the GitHub release
// base URL is prepended using {version} as a placeholder resolved by Render.
//
// Non-GitHub sources (e.g. https://get.helm.sh) are left untouched so callers
// can supply full URLs and still use the {var|modifier} render engine.
func ExpandGitHubTemplate(tmpl, source string) string {
	if tmpl == "" || strings.Contains(tmpl, "://") {
		return tmpl
	}
	orgRepo := githubOrgRepo(source)
	if orgRepo == "" {
		return tmpl // non-GitHub source; caller must supply a full URL
	}
	return "https://github.com/" + orgRepo + "/releases/download/{version}/" + tmpl
}

// githubOrgRepo extracts the "org/repo" path from a GitHub source.
// Returns "" for non-GitHub sources.
// Trailing slashes and a trailing ".git" suffix are normalised away.
func githubOrgRepo(source string) string {
	var path string
	switch {
	case strings.HasPrefix(source, "https://github.com/"):
		path = strings.TrimPrefix(source, "https://github.com/")
	case strings.HasPrefix(source, "http://github.com/"):
		path = strings.TrimPrefix(source, "http://github.com/")
	case strings.Contains(source, "://"):
		return "" // some other scheme — not GitHub
	default:
		path = source
	}

	path = strings.TrimRight(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")

	if strings.Count(path, "/") != 1 {
		return ""
	}
	parts := strings.SplitN(path, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return ""
	}
	return path
}
