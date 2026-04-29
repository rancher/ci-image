package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/dockerfile"
	"github.com/rancher/ci-image/internal/fileutil"
	"github.com/rancher/ci-image/internal/lock"
	"github.com/rancher/ci-image/internal/readme"
	"github.com/rancher/ci-image/internal/resolver"
)

// lockPath returns the deps.lock path adjacent to the given config file.
func lockPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "deps.lock")
}

const (
	dockerfilesDir   = "dockerfiles"
	dockerScriptsDir = "dockerfiles/scripts"
	archiveDir       = "archive"
	defaultConfig    = "deps.yaml"
	readmePath       = "README.md"
)

func runGenerate(args []string) error {
	configPath := defaultConfig
	imageRepo := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--image-repo" && i+1 < len(args):
			i++
			imageRepo = args[i]
		case strings.HasPrefix(args[i], "--image-repo="):
			imageRepo = strings.TrimPrefix(args[i], "--image-repo=")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// Load the lock file (empty if it doesn't exist yet).
	lk, err := lock.Read(lockPath(configPath))
	if err != nil {
		return err
	}

	// Apply locked versions/checksums to release-checksums tools.
	// Run 'update' to fetch new versions and refresh deps.lock.
	if err := resolver.ApplyLock(cfg, lk); err != nil {
		return err
	}

	// Generate Dockerfiles (cfg now has resolved versions/checksums).
	files, err := dockerfile.Generate(cfg, defaultSourceURL(imageRepo))
	if err != nil {
		return err
	}

	// Generate selector scripts (one per family + the generic ci-select).
	selectors := dockerfile.GenerateSelectors(cfg)

	if err := os.MkdirAll(dockerfilesDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", dockerfilesDir, err)
	}

	for imageName, content := range files {
		outputPath := filepath.Join(dockerfilesDir, "Dockerfile."+imageName)
		changed, err := fileutil.WriteIfChanged(outputPath, []byte(content), 0o644)
		if err != nil {
			return fmt.Errorf("writing %s: %w", outputPath, err)
		}
		if changed {
			log.Printf("Updated %s", outputPath)
		}
	}

	if err := os.MkdirAll(dockerScriptsDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", dockerScriptsDir, err)
	}

	for name, content := range selectors {
		outputPath := filepath.Join(dockerScriptsDir, name)
		changed, err := fileutil.WriteIfChanged(outputPath, []byte(content), 0o755)
		if err != nil {
			return fmt.Errorf("writing %s: %w", outputPath, err)
		}
		if changed {
			log.Printf("Updated %s", outputPath)
		}
	}

	// Archive any Dockerfiles for images no longer in config.
	if err := archiveRemovedDockerfiles(files); err != nil {
		return fmt.Errorf("archiving removed dockerfiles: %w", err)
	}

	// Remove any stale selector scripts for families no longer in config.
	if err := cleanupRemovedSelectors(selectors); err != nil {
		return fmt.Errorf("cleaning up removed selectors: %w", err)
	}

	// Write the compiled images lock.
	imagesLockPath := filepath.Join(filepath.Dir(configPath), "images-lock.yaml")
	if err := writeImagesLock(cfg, imagesLockPath); err != nil {
		return fmt.Errorf("writing %s: %w", imagesLockPath, err)
	}
	log.Printf("Generated %s", imagesLockPath)

	// Update the Available Images table in README.md.
	rows := make([]readme.ImageRow, 0, len(cfg.Images))
	for _, img := range cfg.Images {
		rows = append(rows, readme.ImageRow{
			Name:        img.Name,
			GoVersion:   extractGoVersion(img.Base),
			Description: img.Description,
		})
	}
	readmeFile := filepath.Join(filepath.Dir(configPath), readmePath)
	if err := readme.UpdateTable(readmeFile, rows); err != nil {
		return fmt.Errorf("updating %s: %w", readmeFile, err)
	}

	return nil
}

// cleanupRemovedSelectors deletes select-*.sh and ci-select.sh files from
// dockerfilesDir that are no longer produced by the current config.
func cleanupRemovedSelectors(generated map[string]string) error {
	entries, err := os.ReadDir(dockerScriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		isSelectorScript := name == "ci-select.sh" ||
			(strings.HasPrefix(name, "select-") && strings.HasSuffix(name, ".sh"))
		if !isSelectorScript {
			continue
		}
		if _, active := generated[name]; active {
			continue
		}
		path := filepath.Join(dockerScriptsDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		log.Printf("Removed stale selector %s", path)
	}
	return nil
}

// archiveRemovedDockerfiles moves any Dockerfile.<name> in dockerfilesDir that
// is not present in the generated set to archiveDir/Dockerfile.<name>.<YYYYMMDD-HHMMSS> (UTC).
// This preserves history in git when images are removed from config.
func archiveRemovedDockerfiles(generated map[string]string) error {
	entries, err := os.ReadDir(dockerfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dateStr := time.Now().UTC().Format("20060102-150405")

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "Dockerfile.") {
			continue
		}
		imageName := strings.TrimPrefix(name, "Dockerfile.")
		if _, active := generated[imageName]; active {
			continue
		}
		// Image no longer in config — move to archive.
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return err
		}
		src := filepath.Join(dockerfilesDir, name)
		dst := filepath.Join(archiveDir, name+"."+dateStr)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		log.Printf("Archived removed image %q → %s", imageName, dst)
	}
	return nil
}

// imagesLock is the structure written to images-lock.yaml.
type imagesLock struct {
	Images    []string                   `yaml:"images"`
	Packages  []string                   `yaml:"packages,omitempty"`  // universal packages installed in every image
	Tools     map[string]string          `yaml:"tools,omitempty"`     // name → version, all tools across all images
	Selectors []string                   `yaml:"selectors,omitempty"` // active family selector names, e.g. ["helm"]
	Configs   map[string]imageLockConfig `yaml:"configs"`
}

type imageLockConfig struct {
	Base            string            `yaml:"base"`
	Platforms       []string          `yaml:"platforms"`
	Packages        []string          `yaml:"packages,omitempty"`         // image-specific packages only (excludes universal)
	Tools           []string          `yaml:"tools,omitempty"`            // tool names only; versions in top-level tools map
	Aliases         map[string]string `yaml:"aliases,omitempty"`          // symlink_name: tool_name
	FamilySelectors map[string]string `yaml:"family_selectors,omitempty"` // family → default tool
	GoVersion       string            `yaml:"go_version,omitempty"`
	Description     string            `yaml:"description,omitempty"`
}

// extractGoVersion returns the Go version from a SUSE BCI golang base image
// reference (e.g. "registry.suse.com/bci/golang:1.25.9@sha256:…" → "1.25.9").
// Only matches the known BCI registry prefix; returns "" for any other base.
func extractGoVersion(base string) string {
	const prefix = "registry.suse.com/bci/golang:"
	if !strings.HasPrefix(base, prefix) {
		return ""
	}
	tag := base[len(prefix):]
	if at := strings.IndexByte(tag, '@'); at != -1 {
		tag = tag[:at]
	}
	return tag
}

const imagesLockHeader = "# images-lock.yaml — compiled image index generated by 'generate'.\n" +
	"# Records the active image names, universal packages, tool versions, and\n" +
	"# per-image configuration (base, platforms, image-specific packages, tools, aliases).\n" +
	"# Do not edit manually.\n"

// writeImagesLock writes images-lock.yaml: the active image names, top-level
// universal packages and tool versions, plus a per-image configs map with the
// resolved base, platforms, image-specific packages, tool memberships, and
// optional metadata such as Go version and description.
func writeImagesLock(cfg *config.Config, path string) error {
	lk := imagesLock{
		Packages:  cfg.Packages,
		Tools:     make(map[string]string),
		Selectors: dockerfile.FamilySelectorNames(cfg),
		Configs:   make(map[string]imageLockConfig, len(cfg.Images)),
	}

	// Build a set of universal packages so we can store only image-specific
	// additions in each config entry (mirrors how tools are split: top-level
	// map holds versions, per-image list holds membership).
	universalPkgs := make(map[string]bool, len(cfg.Packages))
	for _, p := range cfg.Packages {
		universalPkgs[p] = true
	}

	for _, img := range cfg.Images {
		lk.Images = append(lk.Images, img.Name)

		var toolNames []string
		for i := range cfg.Tools {
			t := &cfg.Tools[i]
			if config.ImageIncludesTool(img, t) {
				toolNames = append(toolNames, t.Name)
				lk.Tools[t.Name] = t.Version
			}
		}
		slices.Sort(toolNames)

		// img.Packages has universal packages prepended by load.go; strip them
		// so the per-image entry only records image-specific additions.
		var specificPkgs []string
		for _, p := range img.Packages {
			if !universalPkgs[p] {
				specificPkgs = append(specificPkgs, p)
			}
		}

		var aliases map[string]string
		if len(img.Aliases) > 0 {
			aliases = img.Aliases
		}

		// Record which family selectors are active for this image and their defaults.
		var familySelectors map[string]string
		for i := range cfg.Tools {
			t := &cfg.Tools[i]
			if t.Family == "" || !t.FamilyDefault {
				continue
			}
			// Only include families where at least one family tool is in this image.
			if !config.ImageIncludesTool(img, t) {
				continue
			}
			if familySelectors == nil {
				familySelectors = make(map[string]string)
			}
			familySelectors[t.Family] = t.Name
		}

		lk.Configs[img.Name] = imageLockConfig{
			Base:            img.Base,
			Platforms:       img.Platforms,
			Packages:        specificPkgs,
			Tools:           toolNames,
			Aliases:         aliases,
			FamilySelectors: familySelectors,
			GoVersion:       extractGoVersion(img.Base),
			Description:     img.Description,
		}
	}

	body, err := yaml.Marshal(lk)
	if err != nil {
		return fmt.Errorf("marshalling images lock: %w", err)
	}
	_, err = fileutil.WriteIfChanged(path, append([]byte(imagesLockHeader), body...), 0o644)
	return err
}

const defaultSourceRepo = "rancher/ci-image"

func defaultSourceURL(override string) string {
	repo := defaultSourceRepo
	if override != "" {
		repo = override
	}
	return fmt.Sprintf("https://github.com/%s", repo)
}
