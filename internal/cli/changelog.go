package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/ci-image/internal/changelog"
	"github.com/rancher/ci-image/internal/gitutil"
)

// runChangelog implements the `changelog` command.
//
// Subcommands:
//
//	changelog generate --version=X [--from=REF] [--to=REF] [--lock=PATH] [--changelog=PATH]
//
// The generate subcommand diffs two states of images-lock.yaml and prepends a
// versioned entry to CHANGELOG.md.
func runChangelog(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: changelog <subcommand>; valid subcommands: generate")
	}
	sub := args[0]
	args = args[1:]
	switch sub {
	case "generate":
		return runChangelogGenerate(args)
	default:
		return fmt.Errorf("unknown changelog subcommand %q; valid subcommands: generate", sub)
	}
}

// runChangelogGenerate implements `changelog generate`.
//
// Flags:
//
//	--version=X       required: the build version string (e.g. 20260423-42)
//	--from=REF        git ref for the "before" state (default: HEAD~1)
//	--to=REF          git ref for the "after" state (default: HEAD)
//	--lock=PATH       path to images-lock.yaml (default: images-lock.yaml)
//	--changelog=PATH  path to CHANGELOG.md (default: CHANGELOG.md)
func runChangelogGenerate(args []string) error {
	version := ""
	from := "HEAD~1"
	to := "HEAD"
	lockPath := "images-lock.yaml"
	changelogPath := "CHANGELOG.md"

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version" && i+1 < len(args):
			i++
			version = args[i]
		case strings.HasPrefix(args[i], "--version="):
			version = strings.TrimPrefix(args[i], "--version=")
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
		case args[i] == "--changelog" && i+1 < len(args):
			i++
			changelogPath = args[i]
		case strings.HasPrefix(args[i], "--changelog="):
			changelogPath = strings.TrimPrefix(args[i], "--changelog=")
		}
	}

	if version == "" {
		return fmt.Errorf("--version is required")
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

	if changes.IsEmpty() {
		affected, err := dockerfileOnlyChanges(from, to)
		if err != nil {
			return fmt.Errorf("detecting dockerfile changes: %w", err)
		}
		changes.DockerfileChanges = affected
	}

	entry := changelog.Entry{
		Version: version,
		Date:    time.Now().UTC(),
		Changes: changes,
	}

	if err := changelog.Prepend(changelogPath, entry); err != nil {
		return fmt.Errorf("updating %s: %w", changelogPath, err)
	}

	return nil
}

// dockerfileOnlyChanges returns the names of images whose Dockerfiles changed
// between from and to, but only when every changed file is under
// internal/dockerfile/ or dockerfiles/. Returns nil if any other files changed
// or if there are no changes.
func dockerfileOnlyChanges(from, to string) ([]string, error) {
	all, err := gitutil.ChangedFiles(from, to)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	for _, f := range all {
		if !strings.HasPrefix(f, "internal/dockerfile/") && !strings.HasPrefix(f, "dockerfiles/") {
			return nil, nil
		}
	}
	return changedDockerfileImages(from, to)
}
