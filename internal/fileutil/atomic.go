package fileutil

import (
	"bytes"
	"os"
	"path/filepath"
)

// WriteIfChanged writes data to path only when the existing file content
// differs. Returns true if the file was written, false if it was unchanged.
// This keeps git status clean on repeated generate runs with no config changes.
func WriteIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, AtomicWrite(path, data, perm)
}

// AtomicWrite writes data to path via a temp file + rename so readers never
// see a partially-written file.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
