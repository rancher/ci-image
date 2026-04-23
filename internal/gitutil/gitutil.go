package gitutil

import (
	"errors"
	"fmt"
	"os/exec"
)

// ErrGitNotFound is returned when git is not in the system PATH.
var ErrGitNotFound = errors.New("git executable not found in PATH")

// ReadFileAtRef returns the contents of path at the given git ref.
// Returns (nil, nil) if the ref does not exist or the file is not present at
// that ref.
// Returns ErrGitNotFound if git is not available on this system.
func ReadFileAtRef(ref, path string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrGitNotFound
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
