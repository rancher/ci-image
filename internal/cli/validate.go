package cli

import (
	"fmt"

	"github.com/rancher/ci-image/internal/config"
)

func runValidate(args []string) error {
	configPath := defaultConfig
	if len(args) > 0 {
		configPath = args[0]
	}

	if _, err := config.Load(configPath); err != nil {
		return err
	}

	fmt.Printf("%s is valid\n", configPath)
	return nil
}
