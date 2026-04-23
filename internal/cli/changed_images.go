package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/rancher/ci-image/internal/changelog"
)

// runChangedImages implements the `changed-images` command.
//
// It compares two states of images-lock.yaml and prints a space-separated
// list of image names whose base image, tool versions, or tool set changed.
// This output can be fed into a workflow matrix or looped over to invoke
// `make push IMAGE=...` for each changed image.
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

	// Also diff the generated Dockerfiles directly — catches template/script
	// changes that don't surface in images-lock.yaml (and makes platform
	// changes a non-issue since regenerated Dockerfiles will differ).
	dfAffected, err := changedDockerfileImages(from, to)
	if err != nil {
		return fmt.Errorf("checking Dockerfile changes: %w", err)
	}
	affected = unionStrings(affected, dfAffected)

	if len(affected) == 0 {
		// No changes detected; print nothing so callers can detect this and
		// fall back to building all images.
		return nil
	}

	fmt.Println(strings.Join(affected, " "))
	return nil
}

// changedDockerfileImages returns image names whose Dockerfile changed between
// the two refs by running git diff --name-only on dockerfiles/.
// When to is empty the working tree is used as the "after" state.
// Returns nil if the refs are unknown or no Dockerfiles changed.
func changedDockerfileImages(from, to string) ([]string, error) {
	args := []string{"diff", "--name-only", from}
	if to != "" {
		args = append(args, to)
	}
	args = append(args, "--", "dockerfiles/")

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 128 && (strings.Contains(string(exitErr.Stderr), "unknown revision") ||
			strings.Contains(string(exitErr.Stderr), "bad revision")) {
			return nil, nil // unknown ref — treat as no diff
		}
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}

	var images []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		base := path.Base(line)
		name := strings.TrimPrefix(base, "Dockerfile.")
		if name != base { // had the "Dockerfile." prefix
			images = append(images, name)
		}
	}
	return images, nil
}

// unionStrings returns a deduplicated union of a and b preserving order of a
// then any elements of b not already in a.
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, s := range a {
		seen[s] = true
	}
	result := append([]string(nil), a...)
	for _, s := range b {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}
	return result
}
