package changelog

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/rancher/ci-image/internal/gitutil"
	"go.yaml.in/yaml/v4"
)

// ReadFromGit reads and parses images-lock.yaml at the given git ref.
// If ref is empty (""), the file is read directly from the filesystem at path.
// Returns nil, nil if the ref does not exist or the file is not present at
// that ref (first-run / new file case).
func ReadFromGit(ref, path string) (*ImagesLock, error) {
	var data []byte

	if ref == "" {
		var err error
		data, err = os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
	} else {
		var err error
		data, err = gitutil.ReadFileAtRef(ref, path)
		if err != nil {
			return nil, fmt.Errorf("reading lock from git: %w", err)
		}
		if data == nil {
			return nil, nil
		}
	}

	var lk ImagesLock
	if err := yaml.Unmarshal(data, &lk); err != nil {
		if ref == "" {
			return nil, fmt.Errorf("parsing images-lock at %s: %w", path, err)
		}
		return nil, fmt.Errorf("parsing images-lock at ref %q: %w", ref, err)
	}
	return &lk, nil
}

// Diff computes what changed between prev (old state) and next (new state).
// If prev is nil (first run), all images are treated as added.
// If next is nil, an empty Changes is returned.
func Diff(prev, next *ImagesLock) *Changes {
	if next == nil {
		return &Changes{}
	}
	if prev == nil {
		return &Changes{
			ImagesAdded: append([]string(nil), next.Images...),
		}
	}

	c := &Changes{
		AllImages: append([]string(nil), next.Images...),
	}

	// Universal package changes.
	prevPkgs := toSet(prev.Packages)
	nextPkgs := toSet(next.Packages)
	for _, p := range prev.Packages {
		if !nextPkgs[p] {
			c.PackagesRemoved = append(c.PackagesRemoved, p)
		}
	}
	for _, p := range next.Packages {
		if !prevPkgs[p] {
			c.PackagesAdded = append(c.PackagesAdded, p)
		}
	}

	// Global family selector changes.
	prevSels := toSet(prev.Selectors)
	nextSels := toSet(next.Selectors)
	for _, s := range prev.Selectors {
		if !nextSels[s] {
			c.SelectorsRemoved = append(c.SelectorsRemoved, SelectorChange{Family: s})
		}
	}
	for _, s := range next.Selectors {
		if !prevSels[s] {
			// Derive the default tool from the first image config that has it.
			defaultTool := ""
			for _, cfg := range next.Configs {
				if d, ok := cfg.FamilySelectors[s]; ok {
					defaultTool = d
					break
				}
			}
			c.SelectorsAdded = append(c.SelectorsAdded, SelectorChange{Family: s, DefaultTool: defaultTool})
		}
	}
	slices.SortFunc(c.SelectorsAdded, func(a, b SelectorChange) int { return strings.Compare(a.Family, b.Family) })
	slices.SortFunc(c.SelectorsRemoved, func(a, b SelectorChange) int { return strings.Compare(a.Family, b.Family) })

	// Global script changes (ci-select, ci-env-init, select-* scripts).
	prevScripts := prev.Scripts
	nextScripts := next.Scripts
	if prevScripts == nil {
		prevScripts = make(map[string]ScriptFile)
	}
	if nextScripts == nil {
		nextScripts = make(map[string]ScriptFile)
	}

	// Find added and modified scripts
	for name, nextScript := range nextScripts {
		prevScript, existed := prevScripts[name]
		if !existed {
			c.ScriptsAdded = append(c.ScriptsAdded, ScriptChange{
				Name:        name,
				NewChecksum: nextScript.Checksum,
			})
		} else if prevScript.Checksum != nextScript.Checksum {
			c.ScriptsModified = append(c.ScriptsModified, ScriptChange{
				Name:        name,
				OldChecksum: prevScript.Checksum,
				NewChecksum: nextScript.Checksum,
			})
		}
	}

	// Find removed scripts
	for name, prevScript := range prevScripts {
		if _, exists := nextScripts[name]; !exists {
			c.ScriptsRemoved = append(c.ScriptsRemoved, ScriptChange{
				Name:        name,
				OldChecksum: prevScript.Checksum,
			})
		}
	}

	slices.SortFunc(c.ScriptsAdded, func(a, b ScriptChange) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.ScriptsRemoved, func(a, b ScriptChange) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(c.ScriptsModified, func(a, b ScriptChange) int { return strings.Compare(a.Name, b.Name) })

	prevImages := toSet(prev.Images)
	nextImages := toSet(next.Images)

	for _, img := range prev.Images {
		if !nextImages[img] {
			c.ImagesRemoved = append(c.ImagesRemoved, img)
		}
	}
	for _, img := range next.Images {
		if !prevImages[img] {
			c.ImagesAdded = append(c.ImagesAdded, img)
		}
	}

	// Per-image diffs for images present in both states.
	for _, imgName := range next.Images {
		if !prevImages[imgName] {
			continue
		}
		prevCfg := prev.Configs[imgName]
		nextCfg := next.Configs[imgName]
		ic := computeImageChanges(imgName, prev.Tools, next.Tools, prev.Hooks, next.Hooks, prevCfg, nextCfg)
		if ic.HasChanges() {
			c.ImageChanges = append(c.ImageChanges, ic)
		}
	}

	return c
}

func computeImageChanges(imgName string, prevTools, nextTools map[string]string, prevHooks, nextHooks map[string]HookFiles, prev, next ImageConfig) ImageChanges {
	ic := ImageChanges{Image: imgName}

	if prev.Base != next.Base {
		ic.BaseImageUpdated = &BaseImageChange{From: prev.Base, To: next.Base}
	}

	prevPlats := append([]string(nil), prev.Platforms...)
	nextPlats := append([]string(nil), next.Platforms...)
	slices.Sort(prevPlats)
	slices.Sort(nextPlats)
	if !slices.Equal(prevPlats, nextPlats) {
		ic.PlatformsChanged = &PlatformsChange{From: prev.Platforms, To: next.Platforms}
	}

	// Image-specific package changes (universal packages are diffed separately).
	prevPkgSet := toSet(prev.Packages)
	nextPkgSet := toSet(next.Packages)
	for _, p := range prev.Packages {
		if !nextPkgSet[p] {
			ic.PackagesRemoved = append(ic.PackagesRemoved, p)
		}
	}
	for _, p := range next.Packages {
		if !prevPkgSet[p] {
			ic.PackagesAdded = append(ic.PackagesAdded, p)
		}
	}
	slices.Sort(ic.PackagesAdded)
	slices.Sort(ic.PackagesRemoved)

	prevToolSet := toSet(prev.Tools)
	nextToolSet := toSet(next.Tools)

	for _, t := range prev.Tools {
		if !nextToolSet[t] {
			ic.ToolsRemoved = append(ic.ToolsRemoved, ToolChange{Tool: t, Version: prevTools[t]})
		}
	}
	for _, t := range next.Tools {
		if !prevToolSet[t] {
			ic.ToolsAdded = append(ic.ToolsAdded, ToolChange{Tool: t, Version: nextTools[t]})
		}
	}
	for _, t := range next.Tools {
		if !prevToolSet[t] {
			continue // already counted as added
		}
		pv, nv := prevTools[t], nextTools[t]
		if pv != nv {
			ic.ToolVersionChanged = append(ic.ToolVersionChanged, ToolVersionChange{Tool: t, From: pv, To: nv})
		}
	}

	slices.SortFunc(ic.ToolsAdded, func(a, b ToolChange) int { return strings.Compare(a.Tool, b.Tool) })
	slices.SortFunc(ic.ToolsRemoved, func(a, b ToolChange) int { return strings.Compare(a.Tool, b.Tool) })
	slices.SortFunc(ic.ToolVersionChanged, func(a, b ToolVersionChange) int { return strings.Compare(a.Tool, b.Tool) })

	// Hook template changes: detect when hooks are added, removed, or modified.
	// Only check hooks for tools that are actually in this image.
	for _, toolName := range next.Tools {
		prevHook := prevHooks[toolName]
		nextHook := nextHooks[toolName]

		// Check pre-hook changes
		if prevHook.Pre == nil && nextHook.Pre != nil {
			// Pre-hook added
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "pre",
				ChangeType:  "added",
				NewChecksum: nextHook.Pre.Checksum,
			})
		} else if prevHook.Pre != nil && nextHook.Pre == nil {
			// Pre-hook removed
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "pre",
				ChangeType:  "removed",
				OldChecksum: prevHook.Pre.Checksum,
			})
		} else if prevHook.Pre != nil && nextHook.Pre != nil && prevHook.Pre.Checksum != nextHook.Pre.Checksum {
			// Pre-hook modified
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "pre",
				ChangeType:  "modified",
				OldChecksum: prevHook.Pre.Checksum,
				NewChecksum: nextHook.Pre.Checksum,
			})
		}

		// Check post-hook changes
		if prevHook.Post == nil && nextHook.Post != nil {
			// Post-hook added
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "post",
				ChangeType:  "added",
				NewChecksum: nextHook.Post.Checksum,
			})
		} else if prevHook.Post != nil && nextHook.Post == nil {
			// Post-hook removed
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "post",
				ChangeType:  "removed",
				OldChecksum: prevHook.Post.Checksum,
			})
		} else if prevHook.Post != nil && nextHook.Post != nil && prevHook.Post.Checksum != nextHook.Post.Checksum {
			// Post-hook modified
			ic.ToolHooksChanged = append(ic.ToolHooksChanged, ToolHookChange{
				Tool:        toolName,
				HookType:    "post",
				ChangeType:  "modified",
				OldChecksum: prevHook.Post.Checksum,
				NewChecksum: nextHook.Post.Checksum,
			})
		}
	}
	slices.SortFunc(ic.ToolHooksChanged, func(a, b ToolHookChange) int {
		if a.Tool != b.Tool {
			return strings.Compare(a.Tool, b.Tool)
		}
		return strings.Compare(a.HookType, b.HookType)
	})

	// Alias diffs: an alias is "removed" if its name disappears or its target changes.
	for name, prevTarget := range prev.Aliases {
		nextTarget, ok := next.Aliases[name]
		if !ok || nextTarget != prevTarget {
			ic.AliasesRemoved = append(ic.AliasesRemoved, AliasChange{Name: name, Target: prevTarget})
		}
	}
	for name, nextTarget := range next.Aliases {
		prevTarget, ok := prev.Aliases[name]
		if !ok || prevTarget != nextTarget {
			ic.AliasesAdded = append(ic.AliasesAdded, AliasChange{Name: name, Target: nextTarget})
		}
	}
	slices.SortFunc(ic.AliasesAdded, func(a, b AliasChange) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(ic.AliasesRemoved, func(a, b AliasChange) int { return strings.Compare(a.Name, b.Name) })

	// Family selector default changes: detect when the active default tool changes.
	// Selector additions/removals are tracked at the global level (Changes.SelectorsAdded/Removed).
	for family, nextDefault := range next.FamilySelectors {
		if prevDefault, ok := prev.FamilySelectors[family]; ok && prevDefault != nextDefault {
			ic.SelectorDefaultChanged = append(ic.SelectorDefaultChanged, SelectorDefaultChange{
				Family: family,
				From:   prevDefault,
				To:     nextDefault,
			})
		}
	}
	slices.SortFunc(ic.SelectorDefaultChanged, func(a, b SelectorDefaultChange) int {
		return strings.Compare(a.Family, b.Family)
	})

	return ic
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
