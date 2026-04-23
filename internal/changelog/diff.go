package changelog

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

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
		out, err := exec.Command("git", "show", ref+":"+path).Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
				return nil, nil // unknown ref or file not present
			}
			return nil, fmt.Errorf("git show %s:%s: %w", ref, path, err)
		}
		data = out
	}

	var lk ImagesLock
	if err := yaml.Unmarshal(data, &lk); err != nil {
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

	c := &Changes{}

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
		ic := computeImageChanges(imgName, prev.Tools, next.Tools, prevCfg, nextCfg)
		if ic.HasChanges() {
			c.ImageChanges = append(c.ImageChanges, ic)
		}
	}

	return c
}

func computeImageChanges(imgName string, prevTools, nextTools map[string]string, prev, next ImageConfig) ImageChanges {
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

	return ic
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
