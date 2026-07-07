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
// Assumes cfg.buildIndices() has already been called (or calls it if needed).
// Returns nil if the config is valid.
func validateConfig(cfg *Config) error {
	// Ensure indices are built (idempotent if already done)
	cfg.ensureIndices()

	var errs []string

	// Validate tool names and check for duplicates
	seen := make(map[string]bool, len(cfg.Tools))
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
		if seen[t.Name] {
			errs = append(errs, fmt.Sprintf("duplicate tool name %q", t.Name))
		}
		seen[t.Name] = true
	}

	// Validate family configuration using precomputed indices
	for _, t := range cfg.Tools {
		if t.Family == "" {
			if t.FamilyDefault {
				errs = append(errs, fmt.Sprintf("tool %q: family_default: true requires family to be set", t.Name))
			}
			continue
		}
		if !toolNameRe.MatchString(t.Family) {
			errs = append(errs, fmt.Sprintf("tool %q: family %q is invalid (must match ^[a-zA-Z0-9][a-zA-Z0-9._-]*$)", t.Name, t.Family))
			continue
		}
	}

	// Validate that each family has at most one default
	for family, fm := range cfg.families {
		if fm.defaultCount > 1 {
			errs = append(errs, fmt.Sprintf("family %q: multiple tools marked as family_default (found %d, expected at most 1)", family, fm.defaultCount))
		}
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
		for _, ref := range img.Tools {
			if seenTools[ref] {
				errs = append(errs, fmt.Sprintf("image %q: duplicate tool reference %q", img.Name, ref))
			}
			seenTools[ref] = true

			// Resolve the reference (could be a tool name or family name)
			resolved := cfg.ResolveToolReference(ref)
			if resolved == nil {
				errs = append(errs, fmt.Sprintf("image %q: tool or family %q is not defined", img.Name, ref))
				continue
			}

			// Mark all resolved tools as referenced
			for _, t := range resolved {
				referencedByImage[t.Name] = true
				if t.Universal {
					errs = append(errs, fmt.Sprintf("image %q: reference %q resolves to universal tool %q which must not be listed in image.tools", img.Name, ref, t.Name))
				}
			}
		}
		for aliasName, targetName := range img.Aliases {
			if !toolNameRe.MatchString(aliasName) {
				errs = append(errs, fmt.Sprintf("image %q: alias name %q is invalid (must match ^[a-zA-Z0-9][a-zA-Z0-9._-]*$)", img.Name, aliasName))
			}
			// Target must be a defined tool. Non-universal targets are auto-included
			// into image.Tools by the loader, so no "not included" check is needed here.
			if cfg.ToolByName(targetName) == nil {
				errs = append(errs, fmt.Sprintf("image %q: alias %q targets tool %q which is not defined", img.Name, aliasName, targetName))
			}
			// Alias name must not conflict with a tool already installed in this image.
			if conflict := cfg.ToolByName(aliasName); conflict != nil && (conflict.Universal || seenTools[aliasName]) {
				errs = append(errs, fmt.Sprintf("image %q: alias name %q conflicts with a tool already installed in this image", img.Name, aliasName))
			}
			// Alias name must not shadow a family selector — /var/ci-tools/active/{family}
			// is on PATH ahead of /usr/local/bin in any image that includes the family's tools.
			if fm := cfg.FamilyMetadata(aliasName); fm != nil {
				familyActiveInImage := false
				for _, toolName := range fm.tools {
					if t := cfg.ToolByName(toolName); t != nil && (t.Universal || seenTools[toolName]) {
						familyActiveInImage = true
						break
					}
				}
				if familyActiveInImage {
					errs = append(errs, fmt.Sprintf("image %q: alias %q shadows the %q family selector; remove the alias and use 'ci-select %s <tool>' instead", img.Name, aliasName, aliasName, aliasName))
				}
			}
		}

		// If any tool from a family is included in this image, the family's default
		// tool must also be included; otherwise selector_setup.tmpl renders an
		// ln -sf with an empty target and the Docker build fails.
		for family, fm := range cfg.families {
			if fm.defaultTool == "" {
				continue // misconfigured family; already reported elsewhere
			}
			dt := cfg.ToolByName(fm.defaultTool)
			defaultInImage := dt != nil && (dt.Universal || seenTools[fm.defaultTool])
			if defaultInImage {
				continue
			}
			for _, toolName := range fm.tools {
				t := cfg.ToolByName(toolName)
				if t == nil {
					continue
				}
				if t.Universal || seenTools[toolName] {
					errs = append(errs, fmt.Sprintf("image %q: includes tool(s) from family %q but not the default tool %q; add it to image.tools or mark a different tool as family_default", img.Name, family, fm.defaultTool))
					break
				}
			}
		}
	}

	// Validate family constraints: ≥2 tools per family, exactly one default,
	// and no family name that collides with a defined tool name.
	for family, fm := range cfg.families {
		if len(fm.tools) < 2 {
			errs = append(errs, fmt.Sprintf("family %q: must have at least 2 tools (found: %s)", family, strings.Join(fm.tools, ", ")))
		}
		// Family default validation already done above via fm.defaultCount
		if fm.defaultCount == 0 {
			errs = append(errs, fmt.Sprintf("family %q: no tool has family_default: true; exactly one is required", family))
		}
		// Check for family name colliding with a tool name
		if cfg.ToolByName(family) != nil {
			errs = append(errs, fmt.Sprintf("family %q: name conflicts with a defined tool name", family))
		}
	}

	// Validate each tool.
	for _, t := range cfg.Tools {
		if t.Name == "" {
			continue // already reported above
		}

		mode := t.EffectiveMode()

		switch mode {
		case ModePinned, ModeStatic, ModeReleaseChecksums:
			// valid modes
		default:
			errs = append(errs, fmt.Sprintf("tool %q: mode %q is not supported (supported: 'pinned', 'static', 'release-checksums')", t.Name, mode))
			continue
		}

		// release-checksums requires an allowlisted source.
		if mode == ModeReleaseChecksums && !inAllowlist(t.Source) {
			errs = append(errs, fmt.Sprintf("tool %q: mode 'release-checksums' requires source to be in the allowlist; %q is not listed", t.Name, t.Source))
		}

		if t.Source == "" {
			errs = append(errs, fmt.Sprintf("tool %q: missing required field 'source'", t.Name))
		}
		if t.Version == "" {
			errs = append(errs, fmt.Sprintf("tool %q: missing required field 'version'", t.Name))
		} else if t.Version == "latest" && mode != ModeReleaseChecksums {
			// version: latest is only valid for release-checksums; pinned and static must pin explicitly.
			errs = append(errs, fmt.Sprintf("tool %q: version 'latest' is not allowed in mode %q", t.Name, mode))
		}
		switch t.Install.EffectiveMethod() {
		case MethodCurl:
			effectiveRelease := t.EffectiveRelease()
			if effectiveRelease == nil {
				errs = append(errs, fmt.Sprintf("tool %q: method 'curl' requires a 'release:' block (or a GitHub source so defaults apply)", t.Name))
			} else {
				if effectiveRelease.DownloadTemplate == "" {
					errs = append(errs, fmt.Sprintf("tool %q: release.download_template is required for method 'curl'", t.Name))
				}
				if effectiveRelease.Extract == "" && mode != ModeReleaseChecksums && isArchiveTemplate(effectiveRelease.DownloadTemplate) {
					errs = append(errs, fmt.Sprintf("tool %q: release.extract is required for method 'curl' when download_template is an archive", t.Name))
				}
			}
			if mode == ModeReleaseChecksums {
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

		case MethodGoInstall:
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
		if t.Install.EffectiveMethod() == MethodCurl && mode != ModeReleaseChecksums && len(t.Checksums) > 0 {
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
