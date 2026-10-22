package updater

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/rancher/ci-image/internal/config"
)

// ToolEdit is a reviewed change to one tool block in deps.yaml.
type ToolEdit struct {
	Name          string
	Version       string
	VersionCommit string            // empty: leave version_commit alone
	Checksums     map[string]string // empty: leave checksums alone (go-install)
}

// BaseEdit is a reviewed change to an image base reference. Every image whose
// base equals Old is rewritten.
type BaseEdit struct {
	Old, New string
}

var (
	topKeyRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):`)
	nameRe   = regexp.MustCompile(`^(\s*)-\s+name:\s*"?([^"\s#]+)"?`)
)

// Apply returns data with edits applied. It patches individual lines rather
// than re-serialising the YAML, so comments, ordering and quoting survive.
// Callers must re-parse the result (see Verify) before trusting it.
func Apply(data []byte, tools []ToolEdit, bases []BaseEdit) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	for _, e := range tools {
		if err := applyTool(lines, e); err != nil {
			return nil, fmt.Errorf("tool %q: %w", e.Name, err)
		}
	}
	for _, e := range bases {
		if err := applyBase(lines, e); err != nil {
			return nil, fmt.Errorf("base %q: %w", e.Old, err)
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// toolBlock locates the list item for the named tool inside the universal: or
// tools: sections (image entries share the "- name:" shape, and an image may
// share a tool's name, so the section matters). It returns the line range
// (start, end) and the item's indent.
func toolBlock(lines []string, name string) (start, end, indent int, err error) {
	section := ""
	start = -1
	for i, l := range lines {
		if m := topKeyRe.FindStringSubmatch(l); m != nil {
			if start >= 0 {
				return start, i, indent, nil
			}
			section = m[1]
			continue
		}
		m := nameRe.FindStringSubmatch(l)
		if m == nil || (section != "tools" && section != "universal") {
			continue
		}
		if start >= 0 && len(m[1]) <= indent {
			return start, i, indent, nil
		}
		if start < 0 && m[2] == name {
			start, indent = i, len(m[1])
		}
	}
	if start >= 0 {
		return start, len(lines), indent, nil
	}
	return 0, 0, 0, fmt.Errorf("not found in tools/universal")
}

func setScalar(line, key, val string) (string, bool) {
	re := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(key) + `:\s*)("?)[^"#\s]*("?)(.*)$`)
	if !re.MatchString(line) {
		return line, false
	}
	return re.ReplaceAllString(line, "${1}${2}"+val+"${3}${4}"), true
}

func applyTool(lines []string, e ToolEdit) error {
	start, end, indent, err := toolBlock(lines, e.Name)
	if err != nil {
		return err
	}
	field := strings.Repeat(" ", indent+2)
	done := map[string]bool{}
	remaining := maps.Clone(e.Checksums)
	inChecksums := false

	for i := start + 1; i < end; i++ {
		l := lines[i]
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if inChecksums {
			if indentOf(l) > indent+2 {
				key, _, _ := strings.Cut(trimmed, ":")
				if sum, ok := remaining[strings.Trim(key, `"`)]; ok {
					lines[i], _ = setScalar(l, key, sum)
					delete(remaining, strings.Trim(key, `"`))
				}
				continue
			}
			inChecksums = false
		}
		switch {
		case strings.HasPrefix(l, field+"version:") && e.Version != "":
			lines[i], done["version"] = setScalar(l, "version", e.Version)
		case strings.HasPrefix(l, field+"version_commit:") && e.VersionCommit != "":
			lines[i], done["version_commit"] = setScalar(l, "version_commit", e.VersionCommit)
		case strings.HasPrefix(l, field+"checksums:") && len(e.Checksums) > 0:
			inChecksums = true
			done["checksums"] = true
		}
	}
	if e.Version != "" && !done["version"] {
		return fmt.Errorf("no version: line found")
	}
	if e.VersionCommit != "" && !done["version_commit"] {
		return fmt.Errorf("no version_commit: line found")
	}
	if len(e.Checksums) > 0 && !done["checksums"] {
		return fmt.Errorf("no checksums: block found")
	}
	if len(remaining) > 0 {
		return fmt.Errorf("checksum platforms not present in deps.yaml: %v", slices.Sorted(maps.Keys(remaining)))
	}
	return nil
}

func applyBase(lines []string, e BaseEdit) error {
	section := ""
	n := 0
	for i, l := range lines {
		if m := topKeyRe.FindStringSubmatch(l); m != nil {
			section = m[1]
			continue
		}
		if section != "images" {
			continue
		}
		trimmed := strings.TrimSpace(l)
		val, ok := strings.CutPrefix(trimmed, "base:")
		if !ok {
			continue
		}
		val, _, _ = strings.Cut(strings.TrimSpace(val), " #")
		if strings.Trim(strings.TrimSpace(val), `"`) != e.Old {
			continue
		}
		lines[i] = strings.Replace(l, e.Old, e.New, 1)
		n++
	}
	if n == 0 {
		return fmt.Errorf("no image uses this base")
	}
	return nil
}

// Verify re-reads the edited config and checks every edit landed exactly as
// reviewed, so a line-patching mistake can never be written to disk silently.
func Verify(newData []byte, tools []ToolEdit, bases []BaseEdit) error {
	cfg, err := config.Parse(newData, "edited deps.yaml")
	if err != nil {
		return fmt.Errorf("edited deps.yaml no longer validates: %w", err)
	}
	for _, e := range tools {
		t := cfg.ToolByName(e.Name)
		if t == nil {
			return fmt.Errorf("tool %q missing after edit", e.Name)
		}
		if e.Version != "" && t.Version != e.Version {
			return fmt.Errorf("tool %q: version is %q after edit, want %q", e.Name, t.Version, e.Version)
		}
		if e.VersionCommit != "" && t.VersionCommit != e.VersionCommit {
			return fmt.Errorf("tool %q: version_commit is %q after edit, want %q", e.Name, t.VersionCommit, e.VersionCommit)
		}
		if len(e.Checksums) > 0 && !maps.Equal(t.Checksums, e.Checksums) {
			return fmt.Errorf("tool %q: checksums differ from those reviewed after edit", e.Name)
		}
	}
	for _, e := range bases {
		for _, img := range cfg.Images {
			if img.Base == e.Old {
				return fmt.Errorf("image %q still uses base %q after edit", img.Name, e.Old)
			}
		}
	}
	return nil
}
