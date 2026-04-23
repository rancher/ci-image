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
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
