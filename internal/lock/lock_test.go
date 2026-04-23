package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRead_MissingFile(t *testing.T) {
	l, err := Read(filepath.Join(t.TempDir(), "deps.lock"))
	if err != nil {
		t.Fatalf("Read() missing file should return empty lock, got error: %v", err)
	}
	if l == nil || l.Tools == nil {
		t.Fatal("Read() returned nil lock or nil Tools map")
	}
	if len(l.Tools) != 0 {
		t.Errorf("Read() empty lock should have no tools, got %d", len(l.Tools))
	}
}

func TestWriteRead_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deps.lock")
	ts := time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC)

	original := &Lock{
		Tools: map[string]Entry{
			"charts-build-scripts": {ResolvedVersion: "v0.18.0", ResolvedAt: ts},
			"ob-charts-tool":       {ResolvedVersion: "v1.2.3", ResolvedAt: ts},
		},
	}

	if err := Write(path, original); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read() unexpected error: %v", err)
	}

	if len(got.Tools) != len(original.Tools) {
		t.Fatalf("round-trip: got %d tools, want %d", len(got.Tools), len(original.Tools))
	}
	for name, want := range original.Tools {
		entry, ok := got.Tools[name]
		if !ok {
			t.Errorf("round-trip: missing tool %q", name)
			continue
		}
		if entry.ResolvedVersion != want.ResolvedVersion {
			t.Errorf("tool %q: version = %q, want %q", name, entry.ResolvedVersion, want.ResolvedVersion)
		}
		if !entry.ResolvedAt.Equal(want.ResolvedAt) {
			t.Errorf("tool %q: resolved_at = %v, want %v", name, entry.ResolvedAt, want.ResolvedAt)
		}
	}
}

func TestWrite_HasGeneratedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deps.lock")
	l := &Lock{Tools: map[string]Entry{
		"mytool": {ResolvedVersion: "v1.0.0", ResolvedAt: time.Now().UTC()},
	}}
	if err := Write(path, l); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(data), "# Auto-generated") {
		t.Errorf("Write() output missing generated-file header:\n%s", data)
	}
}

func TestWrite_Overwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deps.lock")
	ts := time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC)

	first := &Lock{Tools: map[string]Entry{"tool-a": {ResolvedVersion: "v1.0.0", ResolvedAt: ts}}}
	second := &Lock{Tools: map[string]Entry{"tool-a": {ResolvedVersion: "v2.0.0", ResolvedAt: ts}}}

	if err := Write(path, first); err != nil {
		t.Fatalf("first Write() error: %v", err)
	}
	if err := Write(path, second); err != nil {
		t.Fatalf("second Write() error: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got.Tools["tool-a"].ResolvedVersion != "v2.0.0" {
		t.Errorf("overwrite: got version %q, want v2.0.0", got.Tools["tool-a"].ResolvedVersion)
	}
}
