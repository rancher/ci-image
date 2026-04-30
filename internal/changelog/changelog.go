package changelog

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/ci-image/internal/fileutil"
)

const (
	fileHeader = `# Changelog

All notable changes to ci-image are documented here.
Versions follow the ` + "`YYYYMMDD-<run_number>`" + ` format used by CI builds.

`
	beginMarker = "<!-- BEGIN ENTRIES -->\n"
	endMarker   = "\n<!-- END ENTRIES -->"
)

// Entry represents one versioned changelog entry to prepend.
type Entry struct {
	Version string
	Date    time.Time
	Changes *Changes
}

// Prepend inserts a new versioned entry at the top of the changelog at path.
// If the file does not exist, it is created with the standard header and markers.
// Existing entries are preserved below the new one.
func Prepend(path string, entry Entry) error {
	rendered := renderEntry(entry)

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Idempotency: if this version is already recorded, do nothing.
	// This prevents duplicate entries if the same build is re-run.
	versionHeader := "## Revision: " + entry.Version
	if strings.Contains(string(raw), versionHeader) {
		return nil
	}

	var newContent string
	if os.IsNotExist(err) || len(raw) == 0 {
		newContent = fileHeader + beginMarker + rendered + endMarker + "\n"
	} else {
		content := string(raw)
		before, rest, ok := strings.Cut(content, beginMarker)
		if !ok {
			// No markers yet — append markers and entry to existing content.
			newContent = content + "\n" + beginMarker + rendered + endMarker + "\n"
		} else {
			// Prepend new entry inside existing markers.
			newContent = before + beginMarker + rendered + rest
		}
	}

	_, err = fileutil.WriteIfChanged(path, []byte(newContent), 0o644)
	return err
}

// renderEntry produces the Markdown text for one changelog entry.
func renderEntry(entry Entry) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "## Revision: %s (%s)\n\n", entry.Version, entry.Date.UTC().Format("2006-01-02"))

	if entry.Changes.IsEmpty() {
		sb.WriteString("_No notable changes._\n")
		return sb.String()
	}

	// Universal package changes affect all images.
	if len(entry.Changes.PackagesAdded) > 0 {
		sb.WriteString("### Universal Packages Added\n\n")
		for _, p := range entry.Changes.PackagesAdded {
			fmt.Fprintf(&sb, "- `%s`\n", p)
		}
		sb.WriteString("\n")
	}
	if len(entry.Changes.PackagesRemoved) > 0 {
		sb.WriteString("### Universal Packages Removed\n\n")
		for _, p := range entry.Changes.PackagesRemoved {
			fmt.Fprintf(&sb, "- `%s`\n", p)
		}
		sb.WriteString("\n")
	}

	// Family selector additions/removals affect all images that carry those tools.
	if len(entry.Changes.SelectorsAdded) > 0 {
		sb.WriteString("### Family Selectors Added\n\n")
		for _, s := range entry.Changes.SelectorsAdded {
			fmt.Fprintf(&sb, "- `%s` (default: `%s`) — use `ci-select %s <tool>` or `select-%s <tool>`\n",
				s.Family, s.DefaultTool, s.Family, s.Family)
		}
		sb.WriteString("\n")
	}
	if len(entry.Changes.SelectorsRemoved) > 0 {
		sb.WriteString("### Family Selectors Removed\n\n")
		for _, s := range entry.Changes.SelectorsRemoved {
			fmt.Fprintf(&sb, "- `%s`\n", s.Family)
		}
		sb.WriteString("\n")
	}

	for _, ic := range entry.Changes.ImageChanges {
		fmt.Fprintf(&sb, "### Image: %s:%s\n\n", ic.Image, entry.Version)
		if ic.BaseImageUpdated != nil {
			fmt.Fprintf(&sb, "- Base image: `%s` → `%s`\n",
				trimDigest(ic.BaseImageUpdated.From),
				trimDigest(ic.BaseImageUpdated.To))
		}
		if ic.PlatformsChanged != nil {
			fmt.Fprintf(&sb, "- Platforms: `%s` → `%s`\n",
				strings.Join(ic.PlatformsChanged.From, ", "),
				strings.Join(ic.PlatformsChanged.To, ", "))
		}
		for _, p := range ic.PackagesAdded {
			fmt.Fprintf(&sb, "- Added package: `%s`\n", p)
		}
		for _, p := range ic.PackagesRemoved {
			fmt.Fprintf(&sb, "- Removed package: `%s`\n", p)
		}
		for _, tv := range ic.ToolVersionChanged {
			fmt.Fprintf(&sb, "- `%s`: `%s` → `%s`\n", tv.Tool, tv.From, tv.To)
		}
		for _, ta := range ic.ToolsAdded {
			fmt.Fprintf(&sb, "- Added: `%s` `%s`\n", ta.Tool, ta.Version)
		}
		for _, tr := range ic.ToolsRemoved {
			fmt.Fprintf(&sb, "- Removed: `%s`\n", tr.Tool)
		}
		for _, aa := range ic.AliasesAdded {
			fmt.Fprintf(&sb, "- Added alias: `%s` → `%s`\n", aa.Name, aa.Target)
		}
		for _, ar := range ic.AliasesRemoved {
			fmt.Fprintf(&sb, "- Removed alias: `%s`\n", ar.Name)
		}
		for _, sc := range ic.SelectorDefaultChanged {
			fmt.Fprintf(&sb, "- `%s` selector default: `%s` → `%s`\n", sc.Family, sc.From, sc.To)
		}
		if len(entry.Changes.PackagesAdded) > 0 || len(entry.Changes.PackagesRemoved) > 0 {
			sb.WriteString("- Universal package changes\n")
		}
		sb.WriteString("\n")
	}

	// Images rebuilt solely due to universal package changes have no per-image
	// diff entry. Render a minimal section for each so the changelog shows
	// every image that was actually rebuilt.
	if len(entry.Changes.PackagesAdded) > 0 || len(entry.Changes.PackagesRemoved) > 0 {
		shown := make(map[string]bool, len(entry.Changes.ImageChanges))
		for _, ic := range entry.Changes.ImageChanges {
			shown[ic.Image] = true
		}
		for _, img := range entry.Changes.AllImages {
			if !shown[img] {
				fmt.Fprintf(&sb, "### Image: %s:%s\n\n- Universal package changes\n\n", img, entry.Version)
			}
		}
	}

	for _, img := range entry.Changes.DockerfileChanges {
		fmt.Fprintf(&sb, "### Image: %s:%s\n\n- Dockerfile template changes\n\n", img, entry.Version)
	}

	if len(entry.Changes.ImagesAdded) > 0 {
		sb.WriteString("### Images Added\n\n")
		for _, img := range entry.Changes.ImagesAdded {
			fmt.Fprintf(&sb, "- `%s`\n", img)
		}
		sb.WriteString("\n")
	}

	if len(entry.Changes.ImagesRemoved) > 0 {
		sb.WriteString("### Images Removed\n\n")
		for _, img := range entry.Changes.ImagesRemoved {
			fmt.Fprintf(&sb, "- `%s`\n", img)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// trimDigest removes the @sha256:... digest suffix from an image reference,
// leaving just the registry/image:tag portion for readability.
func trimDigest(ref string) string {
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx]
	}
	return ref
}
