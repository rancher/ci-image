package cli

import (
	"context"
	"io"

	"github.com/rancher/ci-image/internal/config"
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
	return nil, ""
}

// warnIneligibleAliases prints the warning for every image that enables
// language_tag without being able to use it.
func warnIneligibleAliases(w io.Writer, cfg *config.Config) {}

// verifyAliasDigests fails if any image that gets a rolling tag has a base
// whose tag no longer resolves to its pinned digest. Shared bases are looked
// up once. only restricts the check to the named images when non-empty.
func verifyAliasDigests(ctx context.Context, cfg *config.Config, only []string) error {
	return nil
}

// offlineRequested reports whether registry checks should be skipped.
func offlineRequested(flag bool) bool { return false }

// runVerifyAliases implements `verify-aliases [--config path] [image...]`.
func runVerifyAliases(args []string) error { return nil }

// runTags implements `tags [--config path] <image>`: it prints the rolling
// tags recorded in images-lock.yaml for the image, one per line.
func runTags(args []string) error { return nil }
