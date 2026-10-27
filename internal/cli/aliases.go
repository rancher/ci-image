package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/rancher/ci-image/internal/config"
	"github.com/rancher/ci-image/internal/language"
	"github.com/rancher/ci-image/internal/lockfile"
	"github.com/rancher/ci-image/internal/registry"
)

// newRegistry builds the registry client for a host; a variable so tests can
// point it at a fake.
var newRegistry = registry.New

const offlineEnv = "CI_IMAGE_OFFLINE"

// imageAliases returns the rolling tags for an image. They are derived from
// the base reference only; an image that asks for one but has no full X.Y.Z
// base tag gets none, plus a warning describing why.
func imageAliases(img config.Image) (aliases []string, warning string) {
	if !img.LanguageTag {
		return nil, ""
	}
	alias, info := language.Alias(img.Base)
	if alias == "" {
		return nil, fmt.Sprintf("image %s: language_tag enabled but no alias produced: %s", img.Name, info.Reason)
	}
	return []string{alias}, ""
}

// warnIneligibleAliases prints the warning for every image that enables
// language_tag without being able to use it.
func warnIneligibleAliases(w io.Writer, cfg *config.Config) {
	for _, img := range cfg.Images {
		if _, warning := imageAliases(img); warning != "" {
			fmt.Fprintln(w, "warning: "+warning)
		}
	}
}

// verifyAliasDigests fails if any image that gets a rolling tag has a base
// whose tag no longer resolves to its pinned digest. Shared bases are looked
// up once. only restricts the check to the named images when non-empty.
func verifyAliasDigests(ctx context.Context, cfg *config.Config, only []string) error {
	clients := map[string]*registry.Client{}
	checked := map[string]error{}
	var errs []error
	for _, img := range cfg.Images {
		if len(only) > 0 && !slices.Contains(only, img.Name) {
			continue
		}
		if aliases, _ := imageAliases(img); len(aliases) == 0 {
			continue
		}
		if _, done := checked[img.Base]; !done {
			checked[img.Base] = verifyBase(ctx, clients, img.Base)
		}
		if err := checked[img.Base]; err != nil {
			errs = append(errs, fmt.Errorf("image %s: %w", img.Name, err))
		}
	}
	return errors.Join(errs...)
}

func verifyBase(ctx context.Context, clients map[string]*registry.Client, base string) error {
	ref, err := registry.ParseRef(base)
	if err != nil {
		return err
	}
	cl, ok := clients[ref.Registry]
	if !ok {
		if cl, err = newRegistry(ref.Registry); err != nil {
			return err
		}
		clients[ref.Registry] = cl
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return language.VerifyDigest(ctx, cl, base)
}

// offlineRequested reports whether registry checks should be skipped.
func offlineRequested(flag bool) bool {
	if flag {
		return true
	}
	v := os.Getenv(offlineEnv)
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// runVerifyAliases implements `verify-aliases [--config path] [image...]`.
func runVerifyAliases(args []string) error {
	configPath := defaultConfig
	var images []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		default:
			images = append(images, args[i])
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := verifyAliasDigests(context.Background(), cfg, images); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "alias digests verified")
	return nil
}

// runTags implements `tags [--config path] <image>`: it prints the rolling
// tags recorded in images-lock.yaml for the image, one per line (nothing when
// there are none). Build and push read the lock so they agree with generate.
func runTags(args []string) error {
	configPath := defaultConfig
	var image string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config" && i+1 < len(args):
			i++
			configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		default:
			image = args[i]
		}
	}
	if image == "" {
		return errors.New("usage: tags [--config path] <image>")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "images-lock.yaml"))
	if err != nil {
		return err
	}
	var lk lockfile.ImagesLock
	if err := yaml.Unmarshal(data, &lk); err != nil {
		return fmt.Errorf("parsing images-lock.yaml: %w", err)
	}
	c, ok := lk.Configs[image]
	if !ok {
		return fmt.Errorf("image %q not in images-lock.yaml", image)
	}
	for _, t := range c.TagAliases {
		fmt.Println(t)
	}
	return nil
}
