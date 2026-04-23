package gitutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTempGitRepo creates a temporary git repository with a single commit
// containing the given files (filename → content). Returns the repo directory
// and the commit SHA.
func newTempGitRepo(t *testing.T, files map[string]string) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", name)
	}
	run("commit", "-m", "init")

	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha = string(out[:len(out)-1]) // trim newline
	return dir, sha
}

// chdir changes to dir for the duration of the test, restoring the original
// working directory on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
}

func TestReadFileAtRef_RefPresent(t *testing.T) {
	dir, sha := newTempGitRepo(t, map[string]string{"lock.yaml": "hello"})
	chdir(t, dir)

	data, err := ReadFileAtRef(sha, "lock.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected %q, got %q", "hello", data)
	}
}

func TestReadFileAtRef_RefUnknown(t *testing.T) {
	dir, _ := newTempGitRepo(t, map[string]string{"lock.yaml": "hello"})
	chdir(t, dir)

	data, err := ReadFileAtRef("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "lock.yaml")
	if err != nil {
		t.Fatalf("expected nil error for unknown ref, got: %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data for unknown ref, got: %q", data)
	}
}

func TestReadFileAtRef_FileNotAtRef(t *testing.T) {
	dir, sha := newTempGitRepo(t, map[string]string{"lock.yaml": "hello"})
	chdir(t, dir)

	data, err := ReadFileAtRef(sha, "no-such-file.yaml")
	if err != nil {
		t.Fatalf("expected nil error when file absent at ref, got: %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data, got: %q", data)
	}
}

func TestReadFileAtRef_GitNotFound(t *testing.T) {
	// Temporarily shadow PATH so git cannot be found.
	orig := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer os.Setenv("PATH", orig)

	_, err := ReadFileAtRef("HEAD", "lock.yaml")
	if !errors.Is(err, ErrGitNotFound) {
		t.Errorf("expected ErrGitNotFound, got: %v", err)
	}
}
