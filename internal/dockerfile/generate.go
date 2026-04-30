package dockerfile

import (
	"fmt"
	"slices"

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
	return executeTemplate("select-family.tmpl", map[string]string{"Family": family})
}

// ciSelectScript returns the content of the generic ci-select script.
// It discovers available families and tools from the manifest written at image
// build time under /usr/local/share/ci-tools/families/.
func ciSelectScript() string {
	return executeTemplate("ci-select.tmpl", nil)
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
