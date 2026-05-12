package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	data := []byte("hello world\n")

	if err := AtomicWrite(path, data, 0o644); err != nil {
		t.Fatalf("AtomicWrite() unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() unexpected error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("AtomicWrite() wrote %q, want %q", got, data)
	}

	// Confirm no temp files were left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("AtomicWrite() left %d entries in dir, want 1", len(entries))
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	if err := AtomicWrite(path, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("first AtomicWrite() unexpected error: %v", err)
	}
	if err := AtomicWrite(path, []byte("second\n"), 0o644); err != nil {
		t.Fatalf("second AtomicWrite() unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() unexpected error: %v", err)
	}
	if string(got) != "second\n" {
		t.Errorf("AtomicWrite() overwrite got %q, want %q", got, "second\n")
	}
}

func TestMD5File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Test with known content
	content := []byte("hello world\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}

	// Expected MD5 of "hello world\n" is 6f5902ac237024bdd0c176cb93063dc4
	expected := "6f5902ac237024bdd0c176cb93063dc4"
	got, err := MD5File(path)
	if err != nil {
		t.Fatalf("MD5File() unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("MD5File() = %s, want %s", got, expected)
	}
}

func TestMD5File_NonExistent(t *testing.T) {
	_, err := MD5File("/nonexistent/file")
	if err == nil {
		t.Error("MD5File() on nonexistent file should return error")
	}
}
