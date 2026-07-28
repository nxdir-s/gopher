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

// TestGenerateReportsRefRenderFailure pins that a failure in a ref's own name
// or output-path template still says which of the two fields failed
func TestGenerateReportsRefRenderFailure(t *testing.T) {
	spec := adapterSpec()
	spec.Templates = []entity.TemplateRef{
		{Name: "adapter/{{.Kind}}", Out: "{{.Missing}}"},
	}

	generator, _ := newTestGenerator(t, map[string]string{"adapter/generic": testTemplate}, spec)

	_, err := generator.Generate(context.Background(), newRequest("Events"))

	var ref *ErrRenderRef
	if !errors.As(err, &ref) {
		t.Fatalf("expected ErrRenderRef, got %v", err)
	}

	if !strings.Contains(err.Error(), "template ref out") {
		t.Errorf("error should identify the failing field: %s", err.Error())
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

// parallelSpec returns a spec with enough create refs to take the fanned path,
// two of them switched off by the default kind flag
func parallelSpec() *entity.GenSpec {
	return &entity.GenSpec{
		Type:    valobj.GenAdapter,
		Summary: "test parallel",
		Flags: []entity.FlagSpec{
			{Name: "name", Required: true},
			{Name: "kind", Default: "generic"},
			{Name: "pkg", Default: "adapters"},
		},
		Templates: []entity.TemplateRef{
			{Name: "multi/a", Out: "a.go"},
			{Name: "multi/b", Out: `{{if eq .Kind "other"}}b.go{{end}}`},
			{Name: "multi/c", Out: "c.go"},
			{Name: "multi/d", Out: `{{if eq .Kind "other"}}d.go{{end}}`},
			{Name: "multi/e", Out: "e.go"},
			{Name: "multi/f", Out: "f.go"},
		},
	}
}

// parallelTemplates seeds the store for parallelSpec's active refs
func parallelTemplates() map[string]string {
	return map[string]string{
		"multi/a": "package adapters\n",
		"multi/c": "package adapters\n",
		"multi/e": "package adapters\n",
		"multi/f": "package adapters\n",
	}
}

// TestGenerateKeepsArtifactOrder pins that concurrent rendering returns
// artifacts in declaration order with the switched-off refs dropped, no matter
// how the scheduler interleaves the workers
func TestGenerateKeepsArtifactOrder(t *testing.T) {
	for range 25 {
		generator, _ := newTestGenerator(t, parallelTemplates(), parallelSpec())

		artifacts, err := generator.Generate(context.Background(), newRequest("Events"))
		if err != nil {
			t.Fatalf("unexpected error: %s", err.Error())
		}

		expected := []string{"a.go", "c.go", "e.go", "f.go"}
		if len(artifacts) != len(expected) {
			t.Fatalf("got %d artifacts, want %d", len(artifacts), len(expected))
		}

		for i := range expected {
			want := filepath.Join("out", expected[i])
			if artifacts[i].Path != want {
				t.Fatalf("artifact %d = %q, want %q", i, artifacts[i].Path, want)
			}
		}
	}
}

// TestGenerateReportsLowestIndexError pins that the error reported by the
// fanned path is the one the serial loop would have hit first
func TestGenerateReportsLowestIndexError(t *testing.T) {
	spec := parallelSpec()
	spec.Templates = []entity.TemplateRef{
		{Name: "multi/a", Out: "a.go"},
		{Name: "multi/missing-first", Out: "b.go"},
		{Name: "multi/c", Out: "c.go"},
		{Name: "multi/missing-second", Out: "d.go"},
		{Name: "multi/e", Out: "e.go"},
	}

	for range 50 {
		generator, _ := newTestGenerator(t, parallelTemplates(), spec)

		_, err := generator.Generate(context.Background(), newRequest("Events"))
		if err == nil {
			t.Fatal("expected an error for a missing template")
		}

		if !strings.Contains(err.Error(), "multi/missing-first") {
			t.Fatalf("expected the lowest-index error, got: %s", err.Error())
		}
	}
}

// TestGenerateMixedModesKeepOrder pins that append and ensure refs keep their
// semantics and their place while the create refs around them render fanned out
func TestGenerateMixedModesKeepOrder(t *testing.T) {
	spec := parallelSpec()
	spec.Templates = []entity.TemplateRef{
		{Name: "multi/a", Out: "a.go"},
		{Name: "multi/append", Out: "ports.go", Mode: entity.ModeAppend},
		{Name: "multi/ensure", Out: "ensure.go", Mode: entity.ModeEnsure},
		{Name: "multi/c", Out: "c.go"},
	}

	templates := parallelTemplates()
	templates["multi/append"] = "package ports\n\ntype {{.Name.Pascal}} interface{}\n"
	templates["multi/ensure"] = "package adapters\n"

	generator, writer := newTestGenerator(t, templates, spec)

	writer.Seed(filepath.Join("out", "ports.go"), "package ports\n\ntype Other interface{}\n")
	writer.Seed(filepath.Join("out", "ensure.go"), "package existing\n")

	artifacts, err := generator.Generate(context.Background(), newRequest("Events"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	expected := []struct {
		path   string
		status entity.ArtifactStatus
	}{
		{"a.go", entity.StatusCreated},
		{"ports.go", entity.StatusAppended},
		{"ensure.go", entity.StatusUnchanged},
		{"c.go", entity.StatusCreated},
	}

	if len(artifacts) != len(expected) {
		t.Fatalf("got %d artifacts, want %d", len(artifacts), len(expected))
	}

	for i := range expected {
		want := filepath.Join("out", expected[i].path)
		if artifacts[i].Path != want {
			t.Errorf("artifact %d = %q, want %q", i, artifacts[i].Path, want)
		}

		if artifacts[i].Status != expected[i].status {
			t.Errorf("artifact %d status = %s, want %s", i, artifacts[i].Status.String(), expected[i].status.String())
		}
	}

	merged := string(writer.Files[filepath.Join("out", "ports.go")])
	if !strings.Contains(merged, "type Other interface{}") || !strings.Contains(merged, "type Events interface{}") {
		t.Errorf("append did not merge both declarations:\n%s", merged)
	}

	if got := string(writer.Files[filepath.Join("out", "ensure.go")]); got != "package existing\n" {
		t.Errorf("ensure overwrote an existing file:\n%s", got)
	}
}

// TestGenerateCanceledContext pins that a canceled context surfaces instead of
// silently dropping the refs the stream never handed out
func TestGenerateCanceledContext(t *testing.T) {
	generator, writer := newTestGenerator(t, parallelTemplates(), parallelSpec())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := generator.Generate(ctx, newRequest("Events"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(writer.Files) != 0 {
		t.Errorf("expected no writes after cancellation, got %v", writer.Paths())
	}
}

// readErrWriter fails reads for one path with a non-not-exist error, the way
// a permission problem would
type readErrWriter struct {
	*fake.Writer
	path string
}

func (w *readErrWriter) Read(path string) ([]byte, error) {
	if path == w.path {
		return nil, errors.New("permission denied")
	}

	return w.Writer.Read(path)
}

// TestGenerateUnreadableEnsureFileIsAnError pins the read-first contract: a
// file that exists but cannot be read must surface as an error rather than
// fall through to the create path and get clobbered
func TestGenerateUnreadableEnsureFileIsAnError(t *testing.T) {
	spec := adapterSpec()
	spec.Templates = []entity.TemplateRef{
		{Name: "adapter/{{.Kind}}", Out: "internal/adapters/{{.Name.Snake}}.go", Mode: entity.ModeEnsure},
	}

	writer := &readErrWriter{fake.NewWriter(), filepath.Join("out", "internal", "adapters", "events.go")}

	generator, err := NewGenerator(slog.New(slog.DiscardHandler),
		WithRegistry(NewRegistry(WithSpecs([]*entity.GenSpec{spec}))),
		WithTemplateSource(fake.NewStore(map[string]string{"adapter/generic": testTemplate})),
		WithRenderer(adapters.NewTemplateAdapter()),
		WithFormatter(adapters.NewFormatAdapter()),
		WithFileWriter(writer),
		WithMerger(adapters.NewGoSourceAdapter()),
	)
	if err != nil {
		t.Fatalf("failed to create generator: %s", err.Error())
	}

	if _, err := generator.Generate(context.Background(), newRequest("Events")); err == nil {
		t.Fatal("expected the unreadable file to surface as an error")
	}

	if len(writer.Files) != 0 {
		t.Errorf("expected no writes, got %v", writer.Paths())
	}
}

func TestNewGeneratorRequiresDependencies(t *testing.T) {
	_, err := NewGenerator(slog.New(slog.DiscardHandler), WithRegistry(NewRegistry()))

	var nilDep *ErrNilDependency
	if !errors.As(err, &nilDep) {
		t.Fatalf("expected ErrNilDependency, got %v", err)
	}
}
