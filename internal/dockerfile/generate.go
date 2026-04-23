package dockerfile

import (
	"fmt"

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
