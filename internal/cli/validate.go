package cli

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/lock"
	"github.com/rancher/ci-image/internal/resolver"
)

func runValidate(args []string) error {
	configPath := defaultConfig
	skipLock := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--skip-lock":
			skipLock = true
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	fmt.Printf("%s is valid\n", configPath)

	// Validate deps.lock if it exists and --skip-lock is not set
	if !skipLock {
		lockFilePath := lockPath(configPath)
		if _, err := os.Stat(lockFilePath); err == nil {
			log.Printf("Validating %s...", lockFilePath)
			lk, err := lock.Read(lockFilePath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", lockFilePath, err)
			}

			if err := resolver.ValidateLock(cfg, lk); err != nil {
				return err
			}

			log.Printf("%s is valid (checksums match stated versions)", lockFilePath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", lockFilePath, err)
		}
	}

	return nil
}
