// Package renderer implements the {var|modifier} template engine used in
// deps.yaml fields such as download_template, extract, and install.package.
package renderer

import (
	"fmt"
	"regexp"
	"strings"
)

// Vars holds the substitution values available in deps.yaml template strings
// (download_template, extract, install.package).
type Vars struct {
	Name          string // tool name
	Source        string // tool source (org/repo or full URL)
	Version       string // version string, e.g. v2.11.4
	VersionCommit string // full commit SHA (optional; empty string if not set)
	OS            string // OS component of the current platform, e.g. linux
	Arch          string // arch component of the current platform, e.g. amd64
}

func (v Vars) toMap() map[string]string {
	return map[string]string{
		"name":           v.Name,
		"source":         v.Source,
		"version":        v.Version,
		"version_commit": v.VersionCommit,
		"os":             v.OS,
		"arch":           v.Arch,
	}
}

// tokenRe matches {variable} and {variable|modifier1|modifier2|...} template tokens.
// Group 1: variable name. Group 2: modifier string (everything after the first |, may be absent).
var tokenRe = regexp.MustCompile(`\{([^}|]+)(?:\|([^}]*))?\}`)

// Render substitutes all {var} and {var|modifier|...} tokens in tmpl using vars.
// Modifiers are applied left-to-right and chained with additional | separators.
//
// Supported modifiers:
//   - upper           — strings.ToUpper
//   - lower           — strings.ToLower
//   - title           — capitalise first character only (e.g. linux → Linux)
//   - trimprefix:ARG  — strings.TrimPrefix(val, ARG)
//   - trimsuffix:ARG  — strings.TrimSuffix(val, ARG)
//   - replace:FROM=TO — replace exact value (e.g. amd64 → x86_64); no-op if no match
//
// Unknown variables and unknown modifiers are hard errors — unlike dep-fetch which
// silently passes them through. This tool generates CI Dockerfiles where silent
// template typos would produce broken images.
func Render(tmpl string, v Vars) (string, error) {
	vars := v.toMap()
	var renderErr error
	result := tokenRe.ReplaceAllStringFunc(tmpl, func(token string) string {
		if renderErr != nil {
			return ""
		}
		m := tokenRe.FindStringSubmatch(token)
		val, ok := vars[m[1]]
		if !ok {
			renderErr = fmt.Errorf("unknown template variable %q", m[1])
			return ""
		}
		if m[2] == "" {
			return val
		}
		for mod := range strings.SplitSeq(m[2], "|") {
			name, arg, _ := strings.Cut(mod, ":")
			var err error
			val, err = applyModifier(val, name, arg)
			if err != nil {
				renderErr = err
				return ""
			}
		}
		return val
	})
	if renderErr != nil {
		return "", renderErr
	}
	return result, nil
}

// applyModifier applies a single modifier to val. name is the modifier name,
// arg is the part after the colon (empty if no colon was present).
func applyModifier(val, name, arg string) (string, error) {
	switch name {
	case "upper":
		return strings.ToUpper(val), nil
	case "lower":
		return strings.ToLower(val), nil
	case "title":
		if val == "" {
			return val, nil
		}
		return strings.ToUpper(val[:1]) + val[1:], nil
	case "trimprefix":
		return strings.TrimPrefix(val, arg), nil
	case "trimsuffix":
		return strings.TrimSuffix(val, arg), nil
	case "replace":
		from, to, ok := strings.Cut(arg, "=")
		if !ok {
			return "", fmt.Errorf("invalid replace modifier: missing '=' in argument %q", arg)
		}
		if val == from {
			return to, nil
		}
		return val, nil
	default:
		return "", fmt.Errorf("unknown modifier %q", name)
	}
}
