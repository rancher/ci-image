package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
var toolNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var ociImageNameRe = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var platformRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)

// validateConfig runs all validation checks on cfg, collecting all errors.
// Returns nil if the config is valid.
func validateConfig(cfg *Config) error {
	var errs []string

	// Build a name→tool map; catch duplicates.
	toolsByName := make(map[string]*Tool, len(cfg.Tools))
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if t.Name == "" {
			errs = append(errs, fmt.Sprintf("tool at index %d: missing required field 'name'", i))
			continue
		}
		if !toolNameRe.MatchString(t.Name) {
			errs = append(errs, fmt.Sprintf("tool at index %d: name %q is invalid (must match ^[a-zA-Z0-9][a-zA-Z0-9._-]*$)", i, t.Name))
			continue
		}
		if _, dup := toolsByName[t.Name]; dup {
			errs = append(errs, fmt.Sprintf("duplicate tool name %q", t.Name))
		}
		toolsByName[t.Name] = t
	}

	// Validate images; track which tool names are explicitly listed.
	referencedByImage := make(map[string]bool)
	for i, img := range cfg.Images {
		if img.Name == "" {
			errs = append(errs, fmt.Sprintf("image at index %d: missing required field 'name'", i))
			continue
		}
		if !ociImageNameRe.MatchString(img.Name) {
			errs = append(errs, fmt.Sprintf("image at index %d: name %q is invalid (must match ^[a-z0-9]+(?:[._-][a-z0-9]+)*$)", i, img.Name))
			continue
		}
		if img.Base == "" {
			errs = append(errs, fmt.Sprintf("image %q: missing required field 'base'", img.Name))
		}
		seenPlatforms := make(map[string]bool)
		for _, p := range img.Platforms {
			if !platformRe.MatchString(p) {
				errs = append(errs, fmt.Sprintf("image %q: invalid platform format %q (must be \"os/arch\")", img.Name, p))
			}
			if seenPlatforms[p] {
				errs = append(errs, fmt.Sprintf("image %q: duplicate platform %q", img.Name, p))
			}
			seenPlatforms[p] = true
		}
		if len(img.Packages) == 0 {
			errs = append(errs, fmt.Sprintf("image %q: 'packages' must have at least one entry", img.Name))
		}
		seenTools := make(map[string]bool)
		for _, toolName := range img.Tools {
			if seenTools[toolName] {
				errs = append(errs, fmt.Sprintf("image %q: duplicate tool %q", img.Name, toolName))
			}
			seenTools[toolName] = true
			referencedByImage[toolName] = true
			t, ok := toolsByName[toolName]
			if !ok {
				errs = append(errs, fmt.Sprintf("image %q: tool %q is not defined in tools:", img.Name, toolName))
				continue
			}
			if t.Universal {
				errs = append(errs, fmt.Sprintf("image %q: tool %q is in the universal: section and must not be listed in image.tools", img.Name, toolName))
			}
		}
	}

	// Validate each tool.
	for _, t := range cfg.Tools {
		if t.Name == "" {
			continue // already reported above
		}

		mode := t.EffectiveMode()

		switch mode {
		case "pinned", "static", "release-checksums":
			// valid modes
		default:
			errs = append(errs, fmt.Sprintf("tool %q: mode %q is not supported (supported: 'pinned', 'static', 'release-checksums')", t.Name, mode))
			continue
		}

		// release-checksums requires an allowlisted source.
		if mode == "release-checksums" && !inAllowlist(t.Source) {
			errs = append(errs, fmt.Sprintf("tool %q: mode 'release-checksums' requires source to be in the allowlist; %q is not listed", t.Name, t.Source))
		}

		if t.Source == "" {
			errs = append(errs, fmt.Sprintf("tool %q: missing required field 'source'", t.Name))
		}
		if t.Version == "" {
			errs = append(errs, fmt.Sprintf("tool %q: missing required field 'version'", t.Name))
		} else if t.Version == "latest" && mode != "release-checksums" {
			// version: latest is only valid for release-checksums; pinned and static must pin explicitly.
			errs = append(errs, fmt.Sprintf("tool %q: version 'latest' is not allowed in mode %q", t.Name, mode))
		}
		switch t.Install.EffectiveMethod() {
		case "curl":
			effectiveRelease := t.EffectiveRelease()
			if effectiveRelease == nil {
				errs = append(errs, fmt.Sprintf("tool %q: method 'curl' requires a 'release:' block (or a GitHub source so defaults apply)", t.Name))
			} else {
				if effectiveRelease.DownloadTemplate == "" {
					errs = append(errs, fmt.Sprintf("tool %q: release.download_template is required for method 'curl'", t.Name))
				}
				if effectiveRelease.Extract == "" && mode != "release-checksums" && isArchiveTemplate(effectiveRelease.DownloadTemplate) {
					errs = append(errs, fmt.Sprintf("tool %q: release.extract is required for method 'curl' when download_template is an archive", t.Name))
				}
			}
			if mode == "release-checksums" {
				// Checksums are fetched at generate time; they must not be declared statically.
				if len(t.Checksums) > 0 {
					errs = append(errs, fmt.Sprintf("tool %q: checksums must be absent for mode 'release-checksums' (they are fetched at generate time)", t.Name))
				}
			} else {
				// pinned / static: checksums must be declared explicitly.
				if len(t.Checksums) == 0 {
					errs = append(errs, fmt.Sprintf("tool %q: method 'curl' requires checksums", t.Name))
				}
				for platform, checksum := range t.Checksums {
					if !platformRe.MatchString(platform) {
						errs = append(errs, fmt.Sprintf("tool %q: invalid platform format %q in checksums (must be \"os/arch\")", t.Name, platform))
					}
					if !sha256Re.MatchString(checksum) {
						errs = append(errs, fmt.Sprintf("tool %q: invalid SHA-256 checksum for platform %s (must be 64 lowercase hex chars)", t.Name, platform))
					}
				}
			}
			if t.Install.Package != "" {
				errs = append(errs, fmt.Sprintf("tool %q: install.package is forbidden for method 'curl'", t.Name))
			}

		case "go-install":
			if t.Install.Package == "" {
				errs = append(errs, fmt.Sprintf("tool %q: install.package is required for method 'go-install'", t.Name))
			}
			if t.Release != nil {
				errs = append(errs, fmt.Sprintf("tool %q: release: block is forbidden for method 'go-install'", t.Name))
			}
			if len(t.Checksums) > 0 {
				errs = append(errs, fmt.Sprintf("tool %q: checksums are forbidden for method 'go-install'", t.Name))
			}

		default:
			errs = append(errs, fmt.Sprintf("tool %q: unknown install method %q", t.Name, t.Install.Method))
		}

		// Every tool must be reachable: either universal or listed in at least one image.
		if !t.Universal && !referencedByImage[t.Name] {
			errs = append(errs, fmt.Sprintf("tool %q: not universal and not listed in any image.tools (would never be used)", t.Name))
		}

		// Pinned/static curl tools: verify checksum coverage for every image that includes the tool.
		// release-checksums tools have checksums resolved at generate time.
		if t.Install.EffectiveMethod() == "curl" && mode != "release-checksums" && len(t.Checksums) > 0 {
			for _, img := range cfg.Images {
				if !imageIncludesTool(img, &t) {
					continue
				}
				for _, platform := range img.Platforms {
					if _, ok := t.Checksums[platform]; !ok {
						errs = append(errs, fmt.Sprintf("tool %q: missing checksum for platform %s required by image %q", t.Name, platform, img.Name))
					}
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

// isArchiveTemplate reports whether a download_template string ends in a known
// archive extension. Used to determine whether extract: is required.
// Templates always use literal extensions (the extension never varies by variable),
// so a simple suffix check on the raw template is reliable.
func isArchiveTemplate(tmpl string) bool {
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".zip"} {
		if strings.HasSuffix(tmpl, ext) {
			return true
		}
	}
	return false
}

// imageIncludesTool reports whether img will include t at generate time.
func imageIncludesTool(img Image, t *Tool) bool {
	return t.Universal || slices.Contains(img.Tools, t.Name)
}

// ImageIncludesTool is the exported form, for use outside the config package.
func ImageIncludesTool(img Image, t *Tool) bool {
	return imageIncludesTool(img, t)
}
