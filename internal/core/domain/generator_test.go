package domain

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/adapters/fake"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

const testTemplate string = `package {{.Package}}

type {{.Name.Pascal}}Adapter struct{}
`

// newTestGenerator builds a generator over the supplied templates and returns it
// alongside the fake writer it emits to
func newTestGenerator(t testing.TB, templates map[string]string, spec *entity.GenSpec) (*Generator, *fake.Writer) {
	t.Helper()

	writer := fake.NewWriter()

	generator, err := NewGenerator(slog.New(slog.DiscardHandler),
		WithRegistry(NewRegistry(WithSpecs([]*entity.GenSpec{spec}))),
		WithTemplateSource(fake.NewStore(templates)),
		WithRenderer(adapters.NewTemplateAdapter()),
		WithFormatter(adapters.NewFormatAdapter()),
		WithFileWriter(writer),
		WithMerger(adapters.NewGoSourceAdapter()),
	)
	if err != nil {
		t.Fatalf("failed to create generator: %s", err.Error())
	}

	return generator, writer
}

// adapterSpec is the spec used by the tests in this file
func adapterSpec() *entity.GenSpec {
	return &entity.GenSpec{
		Type:    valobj.GenAdapter,
		Summary: "test adapter",
		Flags: []entity.FlagSpec{
			{Name: "name", Required: true},
			{Name: "kind", Default: "generic"},
			{Name: "pkg", Default: "adapters"},
		},
		Templates: []entity.TemplateRef{
			{Name: "adapter/{{.Kind}}", Out: "internal/adapters/{{.Name.Snake}}.go"},
		},
	}
}

// newRequest builds a request with the defaults the cli would have applied
func newRequest(name string) *entity.Request {
	return &entity.Request{
		Type:   valobj.GenAdapter,
		Flags:  map[string]string{"name": name, "kind": "generic", "pkg": "adapters"},
		OutDir: "out",
	}
}

func TestGenerateWritesArtifact(t *testing.T) {
	generator, writer := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, adapterSpec())

	artifacts, err := generator.Generate(context.Background(), newRequest("payment gateway"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if len(artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(artifacts))
	}

	expected := filepath.Join("out", "internal", "adapters", "payment_gateway.go")
	if artifacts[0].Path != expected {
		t.Errorf("path = %q, want %q", artifacts[0].Path, expected)
	}

	if !writer.Exists(expected) {
		t.Errorf("expected %q to be written, got %v", expected, writer.Paths())
	}

	if !strings.Contains(string(artifacts[0].Content), "type PaymentGatewayAdapter struct{}") {
		t.Errorf("unexpected content:\n%s", artifacts[0].Content)
	}
}

func TestGenerateRefusesToClobber(t *testing.T) {
	generator, writer := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, adapterSpec())

	expected := filepath.Join("out", "internal", "adapters", "events.go")
	writer.Seed(expected, "package adapters\n")

	_, err := generator.Generate(context.Background(), newRequest("Events"))

	var exists *ErrFileExists
	if !errors.As(err, &exists) {
		t.Fatalf("expected ErrFileExists, got %v", err)
	}

	if string(writer.Files[expected]) != "package adapters\n" {
		t.Error("existing file was modified")
	}
}

func TestGenerateForceOverwrites(t *testing.T) {
	generator, writer := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, adapterSpec())

	expected := filepath.Join("out", "internal", "adapters", "events.go")
	writer.Seed(expected, "package adapters\n")

	req := newRequest("Events")
	req.Force = true

	if _, err := generator.Generate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if !strings.Contains(string(writer.Files[expected]), "EventsAdapter") {
		t.Errorf("file was not overwritten:\n%s", writer.Files[expected])
	}
}

func TestGeneratePreviewsDoNotWrite(t *testing.T) {
	tests := map[string]func(req *entity.Request){
		"dry-run": func(req *entity.Request) { req.DryRun = true },
		"stdout":  func(req *entity.Request) { req.Stdout = true },
	}

	for name, apply := range tests {
		t.Run(name, func(t *testing.T) {
			generator, writer := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, adapterSpec())

			req := newRequest("Events")
			apply(req)

			artifacts, err := generator.Generate(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if len(artifacts) != 1 {
				t.Fatalf("got %d artifacts, want 1", len(artifacts))
			}

			if len(writer.Files) != 0 {
				t.Errorf("expected no writes, got %v", writer.Paths())
			}
		})
	}
}

func TestGenerateRequiresFlags(t *testing.T) {
	generator, _ := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, adapterSpec())

	req := newRequest("")

	_, err := generator.Generate(context.Background(), req)

	var missing *ErrMissingFlag
	if !errors.As(err, &missing) {
		t.Fatalf("expected ErrMissingFlag, got %v", err)
	}
}

func TestGenerateRejectsInvalidGo(t *testing.T) {
	broken := map[string]string{"adapter/generic": "package {{.Package}}\n\nfunc broken( {\n"}

	generator, writer := newTestGenerator(t, broken, adapterSpec())

	_, err := generator.Generate(context.Background(), newRequest("Events"))

	var render *ErrRenderTemplate
	if !errors.As(err, &render) {
		t.Fatalf("expected ErrRenderTemplate, got %v", err)
	}

	if len(writer.Files) != 0 {
		t.Errorf("expected no writes after a render failure, got %v", writer.Paths())
	}
}

func TestGenerateSkipsFormattingNonGoFiles(t *testing.T) {
	spec := adapterSpec()
	spec.Templates = []entity.TemplateRef{
		{Name: "adapter/{{.Kind}}", Out: "Makefile"},
	}

	generator, _ := newTestGenerator(t, map[string]string{"adapter/generic": "build:\n\tgo build ./...\n"}, spec)

	artifacts, err := generator.Generate(context.Background(), newRequest("Events"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if string(artifacts[0].Content) != "build:\n\tgo build ./...\n" {
		t.Errorf("unexpected content:\n%s", artifacts[0].Content)
	}
}

func TestGenerateReportsUnknownTemplate(t *testing.T) {
	spec := adapterSpec()

	generator, _ := newTestGenerator(t, map[string]string{"adapter/other": testTemplate}, spec)

	if _, err := generator.Generate(context.Background(), newRequest("Events")); err == nil {
		t.Fatal("expected an error for a missing template")
	}
}

func TestNewGeneratorRequiresDependencies(t *testing.T) {
	_, err := NewGenerator(slog.New(slog.DiscardHandler), WithRegistry(NewRegistry()))

	var nilDep *ErrNilDependency
	if !errors.As(err, &nilDep) {
		t.Fatalf("expected ErrNilDependency, got %v", err)
	}
}
