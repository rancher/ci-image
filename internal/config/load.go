package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

// Load reads, parses, and validates deps.yaml at path.
// All validation errors are collected and returned together.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Load(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// Apply per-image defaults.
	for i := range cfg.Images {
		if len(cfg.Images[i].Platforms) == 0 {
			cfg.Images[i].Platforms = []string{"linux/amd64", "linux/arm64"}
		}
	}
	// Prepend universal packages into each image's package list.
	if len(cfg.Packages) > 0 {
		for i := range cfg.Images {
			packages := append([]string(nil), cfg.Packages...)
			cfg.Images[i].Packages = append(packages, cfg.Images[i].Packages...)
		}
	}
	// Mark universal tools and merge them into the flat Tools slice so all
	// internal code can iterate a single list.
	for i := range cfg.Universal {
		cfg.Universal[i].Universal = true
	}
	cfg.Tools = append(cfg.Universal, cfg.Tools...)
	cfg.Universal = nil
	// Auto-include alias targets into image.Tools.
	// If an alias references a non-universal tool that isn't already listed in
	// image.tools, add it implicitly so the user doesn't have to repeat it.
	// Undefined targets are left alone and will be caught by validation.
	toolsByName := make(map[string]*Tool, len(cfg.Tools))
	for i := range cfg.Tools {
		toolsByName[cfg.Tools[i].Name] = &cfg.Tools[i]
	}
	for i := range cfg.Images {
		img := &cfg.Images[i]
		toolSet := make(map[string]bool, len(img.Tools))
		for _, t := range img.Tools {
			toolSet[t] = true
		}
		// Collect the set of alias targets that need auto-including.
		aliasTargets := make(map[string]bool, len(img.Aliases))
		for _, targetName := range img.Aliases {
			aliasTargets[targetName] = true
		}
		// Append missing targets in cfg.Tools order for determinism.
		// Ranging over img.Aliases directly would be non-deterministic.
		for _, t := range cfg.Tools {
			if t.Universal || !aliasTargets[t.Name] || toolSet[t.Name] {
				continue // universal tools are always present; skip already-included or non-targeted
			}
			img.Tools = append(img.Tools, t.Name)
			toolSet[t.Name] = true
		}
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
