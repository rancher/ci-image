package dockerfile

import (
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/rancher/ci-image/internal/config"
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

// ToolSetup tests

func TestToolSetup_RenderPre_NilSetup(t *testing.T) {
	var setup *ToolSetup
	result := setup.RenderPre()
	if result != "" {
		t.Errorf("RenderPre() on nil ToolSetup = %q, want empty string", result)
	}
}

func TestToolSetup_RenderPost_NilSetup(t *testing.T) {
	var setup *ToolSetup
	result := setup.RenderPost()
	if result != "" {
		t.Errorf("RenderPost() on nil ToolSetup = %q, want empty string", result)
	}
}

func TestToolSetup_RenderPre_EmptyTemplate(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(""))
	setup := &ToolSetup{
		PreTemplate: "",
		templates:   tmpl,
	}
	result := setup.RenderPre()
	if result != "" {
		t.Errorf("RenderPre() with empty template = %q, want empty string", result)
	}
}

func TestToolSetup_RenderPost_EmptyTemplate(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(""))
	setup := &ToolSetup{
		PostTemplate: "",
		templates:    tmpl,
	}
	result := setup.RenderPost()
	if result != "" {
		t.Errorf("RenderPost() with empty template = %q, want empty string", result)
	}
}

func TestToolSetup_RenderPre_ValidTemplate(t *testing.T) {
	tmpl := template.Must(template.New("test-pre.tmpl").Parse("RUN echo 'pre-install setup'"))
	setup := &ToolSetup{
		Name:        "testtool",
		PreTemplate: "test-pre.tmpl",
		templates:   tmpl,
	}

	result := setup.RenderPre()
	expected := "\n# Pre-install setup for testtool\nRUN echo 'pre-install setup'\n\n"
	if result != expected {
		t.Errorf("RenderPre() = %q, want %q", result, expected)
	}
}

func TestToolSetup_RenderPost_ValidTemplate(t *testing.T) {
	tmpl := template.Must(template.New("test-post.tmpl").Parse("RUN echo 'post-install cleanup'"))
	setup := &ToolSetup{
		Name:         "testtool",
		PostTemplate: "test-post.tmpl",
		templates:    tmpl,
	}

	result := setup.RenderPost()
	expected := "\n\n# Post-install setup for testtool\nRUN echo 'post-install cleanup'"
	if result != expected {
		t.Errorf("RenderPost() = %q, want %q", result, expected)
	}
}

func TestToolSetup_RenderPre_MultilineTemplate(t *testing.T) {
	content := `RUN useradd -m myuser && \
    mkdir -p /app && \
    chown myuser:myuser /app`

	tmpl := template.Must(template.New("multi-pre.tmpl").Parse(content))
	setup := &ToolSetup{
		Name:        "testtool",
		PreTemplate: "multi-pre.tmpl",
		templates:   tmpl,
	}

	result := setup.RenderPre()
	expected := "\n# Pre-install setup for testtool\n" + content + "\n\n"
	if result != expected {
		t.Errorf("RenderPre() = %q, want %q", result, expected)
	}
}

func TestToolSetup_RenderPost_TrimsTrailingNewline(t *testing.T) {
	content := "RUN echo 'test'\n\n"

	tmpl := template.Must(template.New("test-post.tmpl").Parse(content))
	setup := &ToolSetup{
		Name:         "testtool",
		PostTemplate: "test-post.tmpl",
		templates:    tmpl,
	}

	result := setup.RenderPost()
	expected := "\n\n# Post-install setup for testtool\nRUN echo 'test'"
	if result != expected {
		t.Errorf("RenderPost() = %q, want %q (should trim trailing newlines)", result, expected)
	}
}

func TestToolSetup_RenderPre_TemplateNotFound(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(""))
	setup := &ToolSetup{
		PreTemplate: "nonexistent.tmpl",
		templates:   tmpl,
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("RenderPre() should panic when template not found")
		}
	}()

	setup.RenderPre()
}

func TestToolSetup_RenderPost_TemplateNotFound(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(""))
	setup := &ToolSetup{
		PostTemplate: "nonexistent.tmpl",
		templates:    tmpl,
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("RenderPost() should panic when template not found")
		}
	}()

	setup.RenderPost()
}

func TestToolSetup_BothPreAndPost(t *testing.T) {
	tmpl := template.Must(template.New("tool-pre.tmpl").Parse("RUN echo pre"))
	tmpl = template.Must(tmpl.New("tool-post.tmpl").Parse("RUN echo post"))

	setup := &ToolSetup{
		Name:         "testtool",
		PreTemplate:  "tool-pre.tmpl",
		PostTemplate: "tool-post.tmpl",
		templates:    tmpl,
	}

	pre := setup.RenderPre()
	post := setup.RenderPost()

	expectedPre := "\n# Pre-install setup for testtool\nRUN echo pre\n\n"
	expectedPost := "\n\n# Post-install setup for testtool\nRUN echo post"
	if pre != expectedPre {
		t.Errorf("RenderPre() = %q, want %q", pre, expectedPre)
	}
	if post != expectedPost {
		t.Errorf("RenderPost() = %q, want %q", post, expectedPost)
	}
}

func TestToolSetup_OnlyPre(t *testing.T) {
	tmpl := template.Must(template.New("tool-pre.tmpl").Parse("RUN echo pre"))
	setup := &ToolSetup{
		Name:         "testtool",
		PreTemplate:  "tool-pre.tmpl",
		PostTemplate: "", // No post template
		templates:    tmpl,
	}

	pre := setup.RenderPre()
	post := setup.RenderPost()

	expectedPre := "\n# Pre-install setup for testtool\nRUN echo pre\n\n"
	if pre != expectedPre {
		t.Errorf("RenderPre() = %q, want %q", pre, expectedPre)
	}
	if post != "" {
		t.Errorf("RenderPost() = %q, want empty string", post)
	}
}

func TestToolSetup_OnlyPost(t *testing.T) {
	tmpl := template.Must(template.New("tool-post.tmpl").Parse("RUN echo post"))
	setup := &ToolSetup{
		Name:         "testtool",
		PreTemplate:  "", // No pre template
		PostTemplate: "tool-post.tmpl",
		templates:    tmpl,
	}

	pre := setup.RenderPre()
	post := setup.RenderPost()

	if pre != "" {
		t.Errorf("RenderPre() = %q, want empty string", pre)
	}
	expectedPost := "\n\n# Post-install setup for testtool\nRUN echo post"
	if post != expectedPost {
		t.Errorf("RenderPost() = %q, want %q", post, expectedPost)
	}
}

// buildToolSetup tests

func TestBuildToolSetup_NoHooks(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(""))
	tool := config.Tool{Name: "kubectl"}

	result := buildToolSetup(tool, tmpl)

	if result != nil {
		t.Errorf("buildToolSetup() = %+v, want nil when no hooks exist", result)
	}
}

func TestBuildToolSetup_OnlyPre(t *testing.T) {
	tmpl := template.Must(template.New("helm-pre.tmpl").Parse("RUN echo pre"))
	tool := config.Tool{Name: "helm"}

	result := buildToolSetup(tool, tmpl)

	if result == nil {
		t.Fatal("buildToolSetup() returned nil, want ToolSetup")
	}
	if result.Name != "helm" {
		t.Errorf("buildToolSetup() Name = %q, want %q", result.Name, "helm")
	}
	if result.PreTemplate != "helm-pre.tmpl" {
		t.Errorf("buildToolSetup() PreTemplate = %q, want %q", result.PreTemplate, "helm-pre.tmpl")
	}
	if result.PostTemplate != "" {
		t.Errorf("buildToolSetup() PostTemplate = %q, want empty string", result.PostTemplate)
	}
	if result.templates != tmpl {
		t.Error("buildToolSetup() should set templates field")
	}
}

func TestBuildToolSetup_OnlyPost(t *testing.T) {
	tmpl := template.Must(template.New("nix-post.tmpl").Parse("RUN echo post"))
	tool := config.Tool{Name: "nix"}

	result := buildToolSetup(tool, tmpl)

	if result == nil {
		t.Fatal("buildToolSetup() returned nil, want ToolSetup")
	}
	if result.Name != "nix" {
		t.Errorf("buildToolSetup() Name = %q, want %q", result.Name, "nix")
	}
	if result.PreTemplate != "" {
		t.Errorf("buildToolSetup() PreTemplate = %q, want empty string", result.PreTemplate)
	}
	if result.PostTemplate != "nix-post.tmpl" {
		t.Errorf("buildToolSetup() PostTemplate = %q, want %q", result.PostTemplate, "nix-post.tmpl")
	}
	if result.templates != tmpl {
		t.Error("buildToolSetup() should set templates field")
	}
}

func TestBuildToolSetup_BothHooks(t *testing.T) {
	tmpl := template.Must(template.New("terraform-pre.tmpl").Parse("RUN echo pre"))
	tmpl = template.Must(tmpl.New("terraform-post.tmpl").Parse("RUN echo post"))
	tool := config.Tool{Name: "terraform"}

	result := buildToolSetup(tool, tmpl)

	if result == nil {
		t.Fatal("buildToolSetup() returned nil, want ToolSetup")
	}
	if result.Name != "terraform" {
		t.Errorf("buildToolSetup() Name = %q, want %q", result.Name, "terraform")
	}
	if result.PreTemplate != "terraform-pre.tmpl" {
		t.Errorf("buildToolSetup() PreTemplate = %q, want %q", result.PreTemplate, "terraform-pre.tmpl")
	}
	if result.PostTemplate != "terraform-post.tmpl" {
		t.Errorf("buildToolSetup() PostTemplate = %q, want %q", result.PostTemplate, "terraform-post.tmpl")
	}
	if result.templates != tmpl {
		t.Error("buildToolSetup() should set templates field")
	}
}
