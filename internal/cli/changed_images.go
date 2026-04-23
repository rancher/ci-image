package cli

import (
	"fmt"
	"strings"

	"github.com/rancher/ci-image/internal/changelog"
)

// runChangedImages implements the `changed-images` command.
//
// It compares two states of images-lock.yaml and prints a space-separated
// list of image names whose base image, tool versions, or tool set changed.
// This output can be passed directly to `make push IMAGES="..."`.
//
// Usage:
//
//	go run main.go changed-images [--from=REF] [--to=REF] [--lock=PATH]
//
// Flags:
//
//	--from=REF   git ref for the "before" state (default: HEAD)
//	--to=REF     git ref for the "after" state; empty means read from the filesystem (default: "")
//	--lock=PATH  path to images-lock.yaml (default: images-lock.yaml)
func runChangedImages(args []string) error {
	from := "HEAD"
	to := ""
	lockPath := "images-lock.yaml"

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--from" && i+1 < len(args):
			i++
			from = args[i]
		case strings.HasPrefix(args[i], "--from="):
			from = strings.TrimPrefix(args[i], "--from=")
		case args[i] == "--to" && i+1 < len(args):
			i++
			to = args[i]
		case strings.HasPrefix(args[i], "--to="):
			to = strings.TrimPrefix(args[i], "--to=")
		case args[i] == "--lock" && i+1 < len(args):
			i++
			lockPath = args[i]
		case strings.HasPrefix(args[i], "--lock="):
			lockPath = strings.TrimPrefix(args[i], "--lock=")
		}
	}

	prev, err := changelog.ReadFromGit(from, lockPath)
	if err != nil {
		return fmt.Errorf("reading images-lock at %q: %w", from, err)
	}

	next, err := changelog.ReadFromGit(to, lockPath)
	if err != nil {
		return fmt.Errorf("reading images-lock at %q: %w", to, err)
	}

	changes := changelog.Diff(prev, next)

	// A universal package change affects every image — return all image names
	// so the caller builds everything. Using next.Images (not changes.AffectedImages)
	// ensures newly added images are included too.
	if len(changes.PackagesAdded) > 0 || len(changes.PackagesRemoved) > 0 {
		if next != nil && len(next.Images) > 0 {
			fmt.Println(strings.Join(next.Images, " "))
		}
		return nil
	}

	affected := changes.AffectedImages()
	affected = append(affected, changes.ImagesAdded...)

	if len(affected) == 0 {
		// No changes detected; print nothing so callers can detect this and
		// fall back to building all images.
		return nil
	}

	fmt.Println(strings.Join(affected, " "))
	return nil
}
