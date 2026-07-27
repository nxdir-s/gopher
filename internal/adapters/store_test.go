package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// embeddedFS mimics the templates compiled into the binary
func embeddedFS() fstest.MapFS {
	return fstest.MapFS{
		"files/adapter/generic.tmpl": {Data: []byte("embedded generic")},
		"files/adapter/kafka.tmpl":   {Data: []byte("embedded kafka")},
		"files/core/entity.tmpl":     {Data: []byte("embedded entity")},
	}
}

// writeTemplate creates an override template on disk
func writeTemplate(t *testing.T, dir string, name string, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(name)+TemplateExt)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %s", err.Error())
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write template: %s", err.Error())
	}
}

func TestStoreLoadFallsBackToEmbedded(t *testing.T) {
	store := NewStoreAdapter(embeddedFS(), "files")

	src, err := store.Load("adapter/generic")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if string(src) != "embedded generic" {
		t.Errorf("got %q, want the embedded default", src)
	}
}

func TestStoreLoadPrefersFirstOverrideDir(t *testing.T) {
	project := t.TempDir()
	user := t.TempDir()

	writeTemplate(t, project, "adapter/generic", "project generic")
	writeTemplate(t, user, "adapter/generic", "user generic")
	writeTemplate(t, user, "adapter/kafka", "user kafka")

	store := NewStoreAdapter(embeddedFS(), "files", WithTemplateDir(project), WithTemplateDir(user))

	tests := map[string]string{
		"adapter/generic": "project generic",
		"adapter/kafka":   "user kafka",
		"core/entity":     "embedded entity",
	}

	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			src, err := store.Load(name)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if string(src) != expected {
				t.Errorf("got %q, want %q", src, expected)
			}
		})
	}
}

func TestStoreOriginReportsOverrides(t *testing.T) {
	project := t.TempDir()

	writeTemplate(t, project, "adapter/generic", "project generic")

	store := NewStoreAdapter(embeddedFS(), "files", WithTemplateDir(project))

	origin, overridden := store.Origin("adapter/generic")
	if !overridden {
		t.Errorf("expected adapter/generic to be reported as overridden, got %q", origin)
	}

	origin, overridden = store.Origin("adapter/kafka")
	if overridden {
		t.Errorf("expected adapter/kafka to come from the embedded set, got %q", origin)
	}

	if !strings.HasPrefix(origin, EmbeddedPrefix) {
		t.Errorf("origin = %q, want the embedded prefix", origin)
	}
}

func TestStoreListIncludesOverridesOnce(t *testing.T) {
	project := t.TempDir()

	writeTemplate(t, project, "adapter/generic", "project generic")
	writeTemplate(t, project, "adapter/custom", "project custom")

	store := NewStoreAdapter(embeddedFS(), "files", WithTemplateDir(project))

	names, err := store.List()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	expected := []string{"adapter/custom", "adapter/generic", "adapter/kafka", "core/entity"}

	if len(names) != len(expected) {
		t.Fatalf("got %v, want %v", names, expected)
	}

	for i := range expected {
		if names[i] != expected[i] {
			t.Errorf("name %d = %q, want %q", i, names[i], expected[i])
		}
	}
}

func TestStoreEmbeddedIgnoresOverrides(t *testing.T) {
	project := t.TempDir()

	writeTemplate(t, project, "adapter/custom", "project custom")

	store := NewStoreAdapter(embeddedFS(), "files", WithTemplateDir(project))

	names, err := store.Embedded()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	for i := range names {
		if names[i] == "adapter/custom" {
			t.Error("Embedded returned an override")
		}
	}

	if len(names) != 3 {
		t.Errorf("got %v, want the 3 embedded templates", names)
	}
}

func TestStoreLoadReportsMissing(t *testing.T) {
	store := NewStoreAdapter(embeddedFS(), "files")

	_, err := store.Load("adapter/nope")

	var notFound *ErrTemplateNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}
