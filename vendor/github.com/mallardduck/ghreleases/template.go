package ghrelease

import (
	"fmt"
	"regexp"
	"strings"
)

// tokenRe matches {variable} and {variable|modifier1|modifier2|...} template tokens.
var tokenRe = regexp.MustCompile(`\{([^}|]+)(?:\|([^}]*))?\}`)

// TemplateVars holds variables for template rendering.
type TemplateVars struct {
	Name    string // Binary name
	Version string // Release version/tag
	OS      string // Operating system (linux, darwin, windows)
	Arch    string // Architecture (amd64, arm64, etc)
	Ext     string // File extension (tar.gz, zip, etc)

	// Extra holds additional variables addressable as {key}. Built-in variables
	// always win over Extra. A key present with an empty value counts as found
	// (it renders as an empty string rather than ErrUnknownVariable).
	Extra map[string]string
}

// TemplateMode controls error handling for unknown variables.
type TemplateMode int

const (
	// TemplateStrict returns error on unknown variables/modifiers
	TemplateStrict TemplateMode = iota

	// TemplatePermissive passes through unknown vars/modifiers unchanged
	TemplatePermissive

	// TemplateFailsafe ignores unknown modifiers (returns unmodified value)
	// but preserves unknown variables as-is
	TemplateFailsafe
)

// Render applies template variables to a pattern string.
// Supports variables: {name}, {version}, {os}, {arch}, {ext}, plus any keys in TemplateVars.Extra
// Supports modifiers: upper, lower, title, trimprefix:PREFIX, trimsuffix:SUFFIX, replace:FROM=TO
//
// Modifiers are chained with pipe separators and applied left-to-right:
//
//	{variable|modifier1|modifier2}
//
// Examples:
//
//	{name}_{version}_{os}_{arch}.{ext}
//	{name|upper}-{version}.tar.gz
//	{os|replace:darwin=macos}-binary
//	{version|trimprefix:v}
//
// In TemplateStrict mode the partially rendered string is returned together with
// the error; callers must check the error before using the result.
//
// Mode behavior:
//   - TemplateStrict: returns error on unknown variables/modifiers
//   - TemplatePermissive: preserves token unchanged (e.g., {os|bogusmod})
//   - TemplateFailsafe: ignores unknown modifiers, substitutes variable value (e.g., linux)
func Render(pattern string, vars TemplateVars, mode TemplateMode) (string, error) {
	var renderErr error
	result := tokenRe.ReplaceAllStringFunc(pattern, func(token string) string {
		m := tokenRe.FindStringSubmatch(token)
		if len(m) < 2 {
			if mode == TemplateStrict {
				renderErr = fmt.Errorf("%w: malformed template", ErrInvalidModifier)
			}
			return token
		}

		varName := m[1]
		value, found := lookupVar(vars, varName)
		if !found {
			if mode == TemplateStrict {
				renderErr = fmt.Errorf("%w: %s", ErrUnknownVariable, varName)
			}
			return token
		}

		// Apply modifiers if present
		if len(m) > 2 && m[2] != "" {
			for _, mod := range strings.Split(m[2], "|") {
				var err error
				newValue, err := applyModifier(value, mod)
				if err != nil {
					if mode == TemplateStrict {
						renderErr = err
						return token
					}
					if mode == TemplatePermissive {
						return token
					}
					// TemplateFailsafe: ignore error, keep current value
				} else {
					value = newValue
				}
			}
		}

		return value
	})

	return result, renderErr
}

func lookupVar(vars TemplateVars, name string) (string, bool) {
	switch name {
	case "name":
		return vars.Name, true
	case "version":
		return vars.Version, true
	case "os":
		return vars.OS, true
	case "arch":
		return vars.Arch, true
	case "ext":
		return vars.Ext, true
	default:
		v, ok := vars.Extra[name]
		return v, ok
	}
}

func toTitle(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func applyModifier(value, modifier string) (string, error) {
	// Handle modifiers with arguments
	name, arg, hasArg := strings.Cut(modifier, ":")

	switch name {
	case "upper":
		return strings.ToUpper(value), nil
	case "lower":
		return strings.ToLower(value), nil
	case "title":
		return toTitle(value), nil
	case "trimprefix":
		if !hasArg {
			return "", fmt.Errorf("%w: trimprefix requires argument", ErrInvalidModifier)
		}
		return strings.TrimPrefix(value, arg), nil
	case "trimsuffix":
		if !hasArg {
			return "", fmt.Errorf("%w: trimsuffix requires argument", ErrInvalidModifier)
		}
		return strings.TrimSuffix(value, arg), nil
	case "replace":
		if !hasArg {
			return "", fmt.Errorf("%w: replace requires argument", ErrInvalidModifier)
		}
		from, to, ok := strings.Cut(arg, "=")
		if !ok {
			return "", fmt.Errorf("%w: replace requires FROM=TO", ErrInvalidModifier)
		}
		// Only replace if exact match (not ReplaceAll like in the other version)
		if value == from {
			return to, nil
		}
		return value, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownModifier, name)
	}
}
