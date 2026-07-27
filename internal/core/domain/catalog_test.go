package domain

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters/fake"
)

// newTestCatalog builds a catalog over the supplied templates
func newTestCatalog(t *testing.T, templates map[string]string) (*Catalog, *fake.Writer) {
	t.Helper()

	writer := fake.NewWriter()

	catalog, err := NewCatalog(slog.New(slog.DiscardHandler),
		WithTemplateCatalog(fake.NewStore(templates)),
		WithCatalogWriter(writer),
	)
	if err != nil {
		t.Fatalf("failed to create catalog: %s", err.Error())
	}

	return catalog, writer
}

func TestCatalogList(t *testing.T) {
	catalog, _ := newTestCatalog(t, map[string]string{
		"adapter/generic": "generic",
		"adapter/kafka":   "kafka",
	})

	infos, err := catalog.List()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(infos) != 2 {
		t.Fatalf("got %d templates, want 2", len(infos))
	}

	if infos[0].Name != "adapter/generic" {
		t.Errorf("first template = %q, want adapter/generic", infos[0].Name)
	}

	if infos[0].Overridden {
		t.Error("expected the fake store to report no overrides")
	}
}

func TestCatalogInitExportsTemplates(t *testing.T) {
	catalog, writer := newTestCatalog(t, map[string]string{
		"adapter/generic": "generic",
		"core/entity":     "entity",
	})

	dir := filepath.Join("home", "templates")

	result, err := catalog.Init(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(result.Written) != 2 {
		t.Fatalf("wrote %v, want 2 templates", result.Written)
	}

	expected := filepath.Join(dir, "adapter", "generic.tmpl")
	if !writer.Exists(expected) {
		t.Errorf("expected %q to be written, got %v", expected, writer.Paths())
	}

	if string(writer.Files[expected]) != "generic" {
		t.Errorf("content = %q, want the template source", writer.Files[expected])
	}
}

func TestCatalogInitSkipsExisting(t *testing.T) {
	catalog, writer := newTestCatalog(t, map[string]string{"adapter/generic": "generic"})

	dir := "templates"
	path := filepath.Join(dir, "adapter", "generic.tmpl")

	writer.Seed(path, "edited by the user")

	result, err := catalog.Init(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(result.Written) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("written = %v, skipped = %v", result.Written, result.Skipped)
	}

	if string(writer.Files[path]) != "edited by the user" {
		t.Error("an existing template was overwritten without force")
	}
}

func TestCatalogInitForceOverwrites(t *testing.T) {
	catalog, writer := newTestCatalog(t, map[string]string{"adapter/generic": "generic"})

	dir := "templates"
	path := filepath.Join(dir, "adapter", "generic.tmpl")

	writer.Seed(path, "edited by the user")

	if _, err := catalog.Init(context.Background(), dir, true); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if string(writer.Files[path]) != "generic" {
		t.Errorf("content = %q, want the template to be restored", writer.Files[path])
	}
}

func TestCatalogInitRequiresDir(t *testing.T) {
	catalog, _ := newTestCatalog(t, map[string]string{"adapter/generic": "generic"})

	_, err := catalog.Init(context.Background(), "", false)

	var noDir *ErrNoTemplateDir
	if !errors.As(err, &noDir) {
		t.Fatalf("expected ErrNoTemplateDir, got %v", err)
	}
}
