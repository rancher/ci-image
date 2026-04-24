package gitutil

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrGitNotFound is returned when git is not in the system PATH.
var ErrGitNotFound = errors.New("git executable not found in PATH")

// ErrShallowClone is returned when the repository is a shallow clone.
// Shallow clones have incomplete history, which causes changelog generation
// to silently produce wrong output (missing or truncated diffs).
var ErrShallowClone = errors.New("shallow clone detected: changelog generation requires full git history (re-clone without --depth, or run: git fetch --unshallow)")

// shallowCheck is called by ReadFileAtRef to detect shallow clones.
// It is a variable so tests can replace it without needing a real shallow repo.
var shallowCheck = defaultShallowCheck

func defaultShallowCheck() (bool, error) {
	out, err := exec.Command("git", "rev-parse", "--is-shallow-repository").Output()
	if err != nil {
		// Older git, not a git repo, etc. — don't block.
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// ReadFileAtRef returns the contents of path at the given git ref.
// Returns (nil, nil) if the ref does not exist or the file is not present at
// that ref.
// Returns ErrGitNotFound if git is not available on this system.
// Returns ErrShallowClone if the repository is a shallow clone.
func ReadFileAtRef(ref, path string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrGitNotFound
	}

	shallow, err := shallowCheck()
	if err != nil {
		return nil, fmt.Errorf("checking shallow clone: %w", err)
	}
	if shallow {
		return nil, ErrShallowClone
	}

	// Confirm the ref resolves before attempting git show.
	// git cat-file -e exits 0 when the object exists, non-zero otherwise,
	// with no string-matching required.
	if err := exec.Command("git", "cat-file", "-e", ref).Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
			return nil, nil // ref does not exist — first-run case
		}
		return nil, fmt.Errorf("git cat-file -e %s: %w", ref, err)
	}

	out, err := exec.Command("git", "show", ref+":"+path).Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 128 {
			return nil, nil // file not present at this ref
		}
		return nil, fmt.Errorf("git show %s:%s: %w", ref, path, err)
	}
	return out, nil
}
