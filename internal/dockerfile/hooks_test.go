package dockerfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHookTemplates_NoHooksDirectory(t *testing.T) {
	// Change to a temp directory where hooks/ doesn't exist
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}
	if tmpl == nil {
		t.Fatal("loadHookTemplates() returned nil template")
	}

	// Should return base templates without hooks
	if tmpl.Lookup("dockerfile.tmpl") == nil {
		t.Error("loadHookTemplates() should include base templates")
	}
}

func TestLoadHookTemplates_EmptyHooksDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create empty hooks/ directory
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}
	if tmpl == nil {
		t.Fatal("loadHookTemplates() returned nil template")
	}
}

func TestLoadHookTemplates_ValidPreHook(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory with pre-hook
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	preHook := `RUN echo "pre-install setup"
RUN mkdir -p /some/dir`

	if err := os.WriteFile("hooks/test-pre.tmpl", []byte(preHook), 0644); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}

	// Verify the template was loaded
	if tmpl.Lookup("test-pre.tmpl") == nil {
		t.Error("loadHookTemplates() should include test-pre.tmpl")
	}
}

func TestLoadHookTemplates_ValidPostHook(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory with post-hook
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	postHook := `RUN echo "post-install cleanup"
RUN rm -rf /tmp/install`

	if err := os.WriteFile("hooks/test-post.tmpl", []byte(postHook), 0644); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}

	// Verify the template was loaded
	if tmpl.Lookup("test-post.tmpl") == nil {
		t.Error("loadHookTemplates() should include test-post.tmpl")
	}
}

func TestLoadHookTemplates_BothPreAndPostHooks(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	preHook := `RUN echo "pre"`
	postHook := `RUN echo "post"`

	if err := os.WriteFile("hooks/tool-pre.tmpl", []byte(preHook), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("hooks/tool-post.tmpl", []byte(postHook), 0644); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}

	// Verify both templates were loaded
	if tmpl.Lookup("tool-pre.tmpl") == nil {
		t.Error("loadHookTemplates() should include tool-pre.tmpl")
	}
	if tmpl.Lookup("tool-post.tmpl") == nil {
		t.Error("loadHookTemplates() should include tool-post.tmpl")
	}
}

func TestLoadHookTemplates_MultipleTools(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	// Create hooks for multiple tools
	hooks := map[string]string{
		"nix-pre.tmpl":   "RUN echo nix-pre",
		"nix-post.tmpl":  "RUN echo nix-post",
		"helm-pre.tmpl":  "RUN echo helm-pre",
		"helm-post.tmpl": "RUN echo helm-post",
	}

	for name, content := range hooks {
		path := filepath.Join("hooks", name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}

	// Verify all templates were loaded
	for name := range hooks {
		if tmpl.Lookup(name) == nil {
			t.Errorf("loadHookTemplates() should include %s", name)
		}
	}
}

func TestLoadHookTemplates_IgnoresNonHookFiles(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	// Create valid hook
	if err := os.WriteFile("hooks/tool-pre.tmpl", []byte("RUN echo pre"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create files that should be ignored
	if err := os.WriteFile("hooks/README.md", []byte("# Hooks"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("hooks/example.txt", []byte("example"), 0644); err != nil {
		t.Fatal(err)
	}

	tmpl, err := loadHookTemplates()
	if err != nil {
		t.Fatalf("loadHookTemplates() error = %v, want nil", err)
	}

	// Verify only the hook template was loaded
	if tmpl.Lookup("tool-pre.tmpl") == nil {
		t.Error("loadHookTemplates() should include tool-pre.tmpl")
	}
	if tmpl.Lookup("README.md") != nil {
		t.Error("loadHookTemplates() should not include README.md")
	}
	if tmpl.Lookup("example.txt") != nil {
		t.Error("loadHookTemplates() should not include example.txt")
	}
}

func TestLoadHookTemplates_MalformedTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create hooks/ directory
	if err := os.Mkdir("hooks", 0755); err != nil {
		t.Fatal(err)
	}

	// Create malformed template (invalid Go template syntax)
	malformed := `RUN echo "unclosed {{.Variable`

	if err := os.WriteFile("hooks/bad-pre.tmpl", []byte(malformed), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = loadHookTemplates()
	if err == nil {
		t.Error("loadHookTemplates() should return error for malformed template")
	}
}
