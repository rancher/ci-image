package dockerfile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rancher/ci-image/internal/config"
)

// Generate builds a Dockerfile for each image in cfg.
// sourceURL is embedded as org.opencontainers.image.source; pass DefaultSourceURL if not overriding.
// Returns a map of image name → Dockerfile content. No I/O is performed.
func Generate(cfg *config.Config, sourceURL string) (map[string]string, error) {
	result := make(map[string]string, len(cfg.Images))
	for _, img := range cfg.Images {
		vars, err := NewDockerfileVars(cfg, img, sourceURL)
		if err != nil {
			return nil, fmt.Errorf("image %q: %w", img.Name, err)
		}
		result[img.Name] = vars.Render()
	}
	return result, nil
}

// GenerateSelectors returns the generated selector script files that must be
// written alongside the Dockerfiles. The map key is the filename
// (e.g. "ci-select.sh") and the value is the script content.
// Returns nil if no families are defined in cfg.
func GenerateSelectors(cfg *config.Config) map[string]string {
	// Collect unique families across all tools.
	type familyInfo struct {
		defaultTool string
		validTools  []string
	}
	families := make(map[string]*familyInfo)
	for _, t := range cfg.Tools {
		if t.Family == "" {
			continue
		}
		if _, ok := families[t.Family]; !ok {
			families[t.Family] = &familyInfo{}
		}
		fi := families[t.Family]
		fi.validTools = append(fi.validTools, t.Name)
		if t.FamilyDefault {
			fi.defaultTool = t.Name
		}
	}
	if len(families) == 0 {
		return nil
	}

	result := make(map[string]string, len(families)+1)

	// One thin per-family wrapper that just calls ci-select.
	for family := range families {
		result["select-"+family+".sh"] = selectFamilyScript(family)
	}

	// One generic ci-select script that handles all families.
	result["ci-select.sh"] = ciSelectScript()

	return result
}

// selectFamilyScript returns the content of the per-family selector script.
// It is a minimal wrapper around ci-select so all logic lives in one place.
func selectFamilyScript(family string) string {
	return fmt.Sprintf(`#!/bin/sh
# select-%s — activate a %s version for this CI job.
# Equivalent to: ci-select %s [TOOL]
#
# Usage:
#   select-%s              show available tools and current selection
#   select-%s TOOL         activate TOOL as the default '%s' command
exec ci-select %s "$@"
`, family, family, family, family, family, family, family)
}

// ciSelectScript returns the content of the generic ci-select script.
// It discovers available families and tools from the manifest written at image
// build time under /usr/local/share/ci-tools/families/.
func ciSelectScript() string {
	// Sort the lines so the output is deterministic.
	lines := []string{
		`#!/bin/sh`,
		`# ci-select — activate a tool from a CI tool family for this job.`,
		`#`,
		`# The manifest at /usr/local/share/ci-tools/families/ records which tools`,
		`# are available per family. The active selection is a symlink in`,
		`# /var/ci-tools/active/ which is on PATH ahead of /usr/local/bin.`,
		`#`,
		`# Usage:`,
		`#   ci-select                   list available families`,
		`#   ci-select FAMILY            show tools in FAMILY and the current selection`,
		`#   ci-select FAMILY TOOL       activate TOOL as the default FAMILY command`,
		``,
		`set -e`,
		``,
		`FAMILIES_DIR=/usr/local/share/ci-tools/families`,
		`ACTIVE_DIR=/var/ci-tools/active`,
		``,
		`_list_families() {`,
		`    for _d in "${FAMILIES_DIR}"/*/; do`,
		`        [ -d "$_d" ] && basename "$_d"`,
		`    done`,
		`}`,
		``,
		`_list_tools() {`,
		`    _fam="$1"`,
		`    for _f in "${FAMILIES_DIR}/${_fam}"/*; do`,
		`        _name=$(basename "$_f")`,
		`        [ "$_name" = "default" ] && continue`,
		`        printf '  %s\n' "$_name"`,
		`    done`,
		`}`,
		``,
		`_current() {`,
		`    _link="${ACTIVE_DIR}/$1"`,
		`    if [ -L "$_link" ]; then`,
		`        basename "$(readlink "$_link")"`,
		`    else`,
		`        printf '(none)\n'`,
		`    fi`,
		`}`,
		``,
		`FAMILY="${1:-}"`,
		`TOOL="${2:-}"`,
		``,
		`if [ -z "$FAMILY" ]; then`,
		`    printf 'Available CI tool families:\n'`,
		`    _list_families`,
		`    printf '\nUsage: ci-select FAMILY [TOOL]\n'`,
		`    exit 0`,
		`fi`,
		``,
		`if [ ! -d "${FAMILIES_DIR}/${FAMILY}" ]; then`,
		`    printf 'ci-select: unknown family "%s"\n' "$FAMILY" >&2`,
		`    printf 'Available families:\n' >&2`,
		`    _list_families >&2`,
		`    exit 1`,
		`fi`,
		``,
		`if [ -z "$TOOL" ]; then`,
		`    printf 'Available %s tools:\n' "$FAMILY"`,
		`    _list_tools "$FAMILY"`,
		`    printf 'Current: %s\n' "$(_current "$FAMILY")"`,
		`    exit 0`,
		`fi`,
		``,
		`if [ ! -f "${FAMILIES_DIR}/${FAMILY}/${TOOL}" ]; then`,
		`    printf 'ci-select: "%s" is not a valid %s tool\n' "$TOOL" "$FAMILY" >&2`,
		`    printf 'Available:\n' >&2`,
		`    _list_tools "$FAMILY" >&2`,
		`    exit 1`,
		`fi`,
		``,
		`ln -sf "/usr/local/bin/${TOOL}" "${ACTIVE_DIR}/${FAMILY}"`,
		`printf '%s is now: %s\n' "$FAMILY" "$TOOL"`,
	}
	return strings.Join(lines, "\n") + "\n"
}

// FamilySelectorNames returns the sorted list of family names that have
// selector scripts, for use in cleanup logic.
func FamilySelectorNames(cfg *config.Config) []string {
	seen := make(map[string]bool)
	for _, t := range cfg.Tools {
		if t.Family != "" {
			seen[t.Family] = true
		}
	}
	names := make([]string, 0, len(seen))
	for f := range seen {
		names = append(names, f)
	}
	slices.Sort(names)
	return names
}
