package cli

import (
	"fmt"
	"log"
	"strings"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/lock"
	"github.com/rancher/ci-image/internal/resolver"
)

func runUpdate(args []string) error {
	configPath := defaultConfig
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	lk, err := lock.Read(lockPath(configPath))
	if err != nil {
		return err
	}

	if err := resolver.Update(cfg, lk); err != nil {
		return err
	}

	changed, err := lock.WriteIfChanged(lockPath(configPath), lk)
	if err != nil {
		return fmt.Errorf("writing deps.lock: %w", err)
	}
	if changed {
		log.Printf("Updated deps.lock")
	} else {
		log.Printf("deps.lock is already up to date")
	}
	return nil
}
