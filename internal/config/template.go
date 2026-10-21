package config

import (
	"strings"

	ghrelease "github.com/mallardduck/ghreleases"
)

// TemplateVars builds the variable set for deps.yaml templates
// (download_template, checksum_template, extract, install.package).
//
// source and version_commit are always present, even when empty, so a template
// referencing {version_commit} on a tool without one renders as "" rather than
// failing as an unknown variable.
func TemplateVars(name, source, version, versionCommit, os, arch string) ghrelease.TemplateVars {
	return ghrelease.TemplateVars{
		Name:    name,
		Version: version,
		OS:      os,
		Arch:    arch,
		Extra: map[string]string{
			"source":         source,
			"version_commit": versionCommit,
		},
	}
}

// RenderTemplate renders a deps.yaml template in strict mode: unknown variables
// and modifiers are hard errors, because a silent typo here would produce a
// broken CI Dockerfile. On error the partially rendered string is discarded.
func RenderTemplate(tmpl string, vars ghrelease.TemplateVars) (string, error) {
	out, err := ghrelease.Render(tmpl, vars, ghrelease.TemplateStrict)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ExpandGitHubTemplate expands a release filename template to a full URL for
// GitHub-sourced tools. If tmpl already contains "://", it is returned as-is
// (already a full URL). If source is a GitHub org/repo (shorthand or full
// https://github.com/org/repo) and tmpl is a bare filename, the GitHub release
// base URL is prepended using {version} as a placeholder resolved by
// RenderTemplate.
//
// Non-GitHub sources (e.g. https://get.helm.sh) are left untouched so callers
// can supply full URLs and still use the {var|modifier} render engine.
func ExpandGitHubTemplate(tmpl, source string) string {
	if tmpl == "" || strings.Contains(tmpl, "://") {
		return tmpl
	}
	owner, repo, err := ghrelease.ParseSource(source)
	if err != nil {
		return tmpl // non-GitHub source; caller must supply a full URL
	}
	return "https://github.com/" + owner + "/" + repo + "/releases/download/{version}/" + tmpl
}
