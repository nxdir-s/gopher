package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file and the directories leading to it
func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %s", err.Error())
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %s", err.Error())
	}
}

// newProject creates a module rooted at a temp dir with an isolated user config
func newProject(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	userDir := t.TempDir()

	t.Setenv(XdgConfigEnv, userDir)

	writeFile(t, filepath.Join(root, GoModFile), "module github.com/nxdir-s/demo\n\ngo 1.26.4\n")

	return root, filepath.Join(userDir, "gopher")
}

func TestLoadDetectsModule(t *testing.T) {
	root, _ := newProject(t)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if cfg.Module != "github.com/nxdir-s/demo" {
		t.Errorf("module = %q, want github.com/nxdir-s/demo", cfg.Module)
	}

	if cfg.OutDir != DefaultOutDir {
		t.Errorf("out dir = %q, want %q", cfg.OutDir, DefaultOutDir)
	}

	if cfg.Root() != root {
		t.Errorf("root = %q, want %q", cfg.Root(), root)
	}
}

func TestLoadDetectsModuleFromSubdirectory(t *testing.T) {
	root, _ := newProject(t)

	sub := filepath.Join(root, "internal", "adapters")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("failed to create dir: %s", err.Error())
	}

	cfg, err := Load(sub)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if cfg.Module != "github.com/nxdir-s/demo" {
		t.Errorf("module = %q, want github.com/nxdir-s/demo", cfg.Module)
	}
}

func TestLoadProjectConfigOverridesUserConfig(t *testing.T) {
	root, userDir := newProject(t)

	writeFile(t, filepath.Join(userDir, UserFileName), `{
		"out_dir": "user-out",
		"template_dir": "/tmp/user-templates",
		"defaults": {"pkg": "user", "tracer": "false"}
	}`)

	writeFile(t, filepath.Join(root, ConfigDirName, ConfigFileName), `{
		"out_dir": "project-out",
		"defaults": {"pkg": "project"}
	}`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if cfg.OutDir != "project-out" {
		t.Errorf("out dir = %q, want project-out", cfg.OutDir)
	}

	if cfg.TemplateDir != "/tmp/user-templates" {
		t.Errorf("template dir = %q, want the user value to survive", cfg.TemplateDir)
	}

	pkg, ok := cfg.Default("pkg")
	if !ok || pkg != "project" {
		t.Errorf("pkg default = %q, want project", pkg)
	}

	tracer, ok := cfg.Default("tracer")
	if !ok || tracer != "false" {
		t.Errorf("tracer default = %q, want the user value to survive", tracer)
	}
}

func TestLoadReportsBrokenConfig(t *testing.T) {
	root, _ := newProject(t)

	writeFile(t, filepath.Join(root, ConfigDirName, ConfigFileName), "{not json")

	if _, err := Load(root); err == nil {
		t.Fatal("expected an error for malformed json")
	}
}

func TestTemplateDirsOrder(t *testing.T) {
	root, userDir := newProject(t)

	writeFile(t, filepath.Join(root, ConfigDirName, ConfigFileName), `{"template_dir": "/tmp/configured"}`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	expected := []string{
		filepath.Join(root, ConfigDirName, TemplateDir),
		"/tmp/configured",
		filepath.Join(userDir, TemplateDir),
	}

	dirs := cfg.TemplateDirs()

	if len(dirs) != len(expected) {
		t.Fatalf("got %d dirs, want %d: %v", len(dirs), len(expected), dirs)
	}

	for i := range expected {
		if dirs[i] != expected[i] {
			t.Errorf("dir %d = %q, want %q", i, dirs[i], expected[i])
		}
	}
}

func TestParseModule(t *testing.T) {
	tests := map[string]string{
		"module github.com/nxdir-s/demo\n\ngo 1.26.4\n": "github.com/nxdir-s/demo",
		"// a comment\nmodule  spaced/path \n":          "spaced/path",
		"go 1.26.4\n":                                   "",
		"":                                              "",
	}

	for input, expected := range tests {
		if got := parseModule([]byte(input)); got != expected {
			t.Errorf("parseModule(%q) = %q, want %q", input, got, expected)
		}
	}
}
