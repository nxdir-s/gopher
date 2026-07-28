package adapters

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"text/template"

	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/templates"
)

// benchRefName is the ref name template the generator renders before it can
// load a template. Two of the three renders per ref look like this one and the
// next, which is why they are measured on their own
const benchRefName string = "adapter/{{.Kind}}"

// benchRefOut is the ref output path template the generator renders per ref
const benchRefOut string = "internal/adapters/{{.Name.Snake}}.go"

// benchRefStatic is a ref string holding no action at all. Most of the
// registry's ref names and output paths are literal like this, so the renderer
// returns them without parsing and the gap between this case and the two above
// is what that fast path is worth
const benchRefStatic string = "internal/adapters/store.go"

// benchData returns the render payload the benchmarks share. Every map key any
// template indexes is populated because the renderer runs with missingkey=error
func benchData() *entity.TemplateData {
	return &entity.TemplateData{
		Name:      valobj.NewNaming("payment gateway"),
		Package:   "adapters",
		Module:    "github.com/nxdir-s/demo",
		Kind:      "generic",
		GoVersion: "1.26.4",
		Fields: []valobj.Field{
			{Name: valobj.NewNaming("id"), Type: "int"},
			{Name: valobj.NewNaming("total"), Type: "float64"},
		},
		Ports: []valobj.Field{
			{Name: valobj.NewNaming("repo"), Type: "ports.OrderRepository"},
		},
		Methods: []valobj.Method{
			{
				Name:       valobj.NewNaming("Save"),
				Params:     "ctx context.Context, id int",
				Results:    "error",
				Args:       "ctx, id",
				HasResults: true,
			},
		},
		Flags: map[string]string{
			"dir":  "internal/core/domain",
			"json": entity.BoolTrue,
			"port": "8080",
			"side": "secondary",
		},
		Lists: map[string][]string{
			"case":   {"returns the total"},
			"import": {"context"},
			"method": {"Save(ctx context.Context, id int) error"},
			"value":  {"Pending", "Delivered"},
		},
		Tracer: true,
		Logger: true,
	}
}

// benchStore returns a store over the embedded templates with no override dirs
func benchStore() *StoreAdapter {
	return NewStoreAdapter(templates.FS, templates.Root)
}

// benchTemplate loads a named template out of the embedded set
func benchTemplate(t testing.TB, name string) []byte {
	t.Helper()

	tmpl, err := benchStore().Load(name)
	if err != nil {
		t.Fatalf("failed to load template: %s", err.Error())
	}

	return tmpl
}

// benchSource renders a named template into the Go source the format and merge
// benchmarks consume, so their inputs are the size real output actually is
func benchSource(t testing.TB, name string) []byte {
	t.Helper()

	return benchSourceAs(t, name, "adapters")
}

// benchSourceAs renders a named template into the supplied package. The merge
// benchmark needs its addition to declare the same package as the file it is
// merged into, or the adapter rejects the pair before doing any work
func benchSourceAs(t testing.TB, name string, pkg string) []byte {
	t.Helper()

	data := benchData()
	data.Package = pkg

	src, err := NewTemplateAdapter().Render(name, benchTemplate(t, name), data)
	if err != nil {
		t.Fatalf("failed to render template: %s", err.Error())
	}

	return src
}

// benchBodies are the template bodies the render benchmarks span, from the
// smallest template in the set to the largest
func benchBodies() map[string]string {
	return map[string]string{
		"entity": "core/entity",
		"domain": "core/domain",
		"aws":    "adapter/aws",
	}
}

// BenchmarkTemplateRender measures a full parse and execute. The name and out
// cases are the one line ref strings the generator renders twice per ref before
// it touches the body, so they dominate a multi artifact type by count
func BenchmarkTemplateRender(b *testing.B) {
	renderer := NewTemplateAdapter()
	data := benchData()

	refs := map[string]string{"name": benchRefName, "out": benchRefOut, "static": benchRefStatic}

	for label, src := range refs {
		tmpl := []byte(src)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := renderer.Render(label, tmpl, data); err != nil {
					b.Fatalf("failed to render: %s", err.Error())
				}
			}
		})
	}

	for label, name := range benchBodies() {
		tmpl := benchTemplate(b, name)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := renderer.Render(name, tmpl, data); err != nil {
					b.Fatalf("failed to render: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkTemplateParse isolates the parse half of Render. Nothing caches a
// parsed template, so this cost is paid on every render the pipeline performs
func BenchmarkTemplateParse(b *testing.B) {
	funcs := NewTemplateAdapter().funcs

	sources := map[string]string{"name": benchRefName, "out": benchRefOut}
	for label, name := range benchBodies() {
		sources[label] = string(benchTemplate(b, name))
	}

	for label, src := range sources {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				_, err := template.New(label).Funcs(funcs).Option("missingkey=error").Parse(src)
				if err != nil {
					b.Fatalf("failed to parse: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkTemplateExecute isolates the execute half of Render against a
// template parsed up front. Read against BenchmarkTemplateParse it bounds what
// removing the repeated parse could ever save
func BenchmarkTemplateExecute(b *testing.B) {
	funcs := NewTemplateAdapter().funcs
	data := benchData()

	sources := map[string]string{"name": benchRefName, "out": benchRefOut}
	for label, name := range benchBodies() {
		sources[label] = string(benchTemplate(b, name))
	}

	for label, src := range sources {
		parsed, err := template.New(label).Funcs(funcs).Option("missingkey=error").Parse(src)
		if err != nil {
			b.Fatalf("failed to parse: %s", err.Error())
		}

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				var buf bytes.Buffer
				if err := parsed.Execute(&buf, data); err != nil {
					b.Fatalf("failed to execute: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkFormatSource measures go/format over generated output, from a 69
// byte entity to the 12 KB aws adapter
func BenchmarkFormatSource(b *testing.B) {
	formatter := NewFormatAdapter()

	for label, name := range benchBodies() {
		src := benchSource(b, name)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := formatter.Format(src); err != nil {
					b.Fatalf("failed to format: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkStoreLoad measures template lookup. The overrides case is the real
// production path, since TemplateDirs always appends the user template dir
// whether or not it exists
//
// Neither directory here exists, so the constructor drops both and the two
// cases should report the same number. A gap reopening between them means a
// failed read is being paid per template again
func BenchmarkStoreLoad(b *testing.B) {
	missing := b.TempDir()

	stores := map[string]*StoreAdapter{
		"embedded": benchStore(),
		"overrides": NewStoreAdapter(templates.FS, templates.Root,
			WithTemplateDir(missing+"/project"),
			WithTemplateDir(missing+"/user"),
		),
	}

	for label, store := range stores {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := store.Load("core/entity"); err != nil {
					b.Fatalf("failed to load: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkStoreList measures the walk over the embedded set that backs
// 'gopher templates list'
func BenchmarkStoreList(b *testing.B) {
	store := benchStore()

	for b.Loop() {
		if _, err := store.List(); err != nil {
			b.Fatalf("failed to list: %s", err.Error())
		}
	}
}

// BenchmarkStoreOrigin measures the per template stat the catalog performs for
// every name it lists
func BenchmarkStoreOrigin(b *testing.B) {
	missing := b.TempDir()

	stores := map[string]*StoreAdapter{
		"embedded": benchStore(),
		"overrides": NewStoreAdapter(templates.FS, templates.Root,
			WithTemplateDir(missing+"/project"),
			WithTemplateDir(missing+"/user"),
		),
	}

	for label, store := range stores {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				store.Origin("core/entity")
			}
		})
	}
}

// BenchmarkGoSourceMethods measures the parse of a synthesized interface that
// templateData performs whenever -method is supplied
func BenchmarkGoSourceMethods(b *testing.B) {
	source := NewGoSourceAdapter()

	decls := map[string][]string{
		"one": {"Save(ctx context.Context, id int) error"},
		"three": {
			"Save(ctx context.Context, id int) error",
			"Find(ctx context.Context, id int) (*entity.Order, error)",
			"Delete(ctx context.Context, id int) error",
		},
	}

	for label, list := range decls {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := source.Methods(list); err != nil {
					b.Fatalf("failed to parse methods: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGoSourceDeclares measures the parse and scan the append path runs
// before it decides whether a merge is needed
func BenchmarkGoSourceDeclares(b *testing.B) {
	source := NewGoSourceAdapter()
	existing := []byte(existingPorts)

	names := map[string]string{"hit": "OrderRepository", "miss": "EventPublisher"}

	for label, name := range names {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := source.Declares(existing, name); err != nil {
					b.Fatalf("failed to scan source: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGoSourceMerge measures the two parses and the import union behind
// the only append mode ref in the registry
func BenchmarkGoSourceMerge(b *testing.B) {
	source := NewGoSourceAdapter()
	existing := []byte(existingPorts)
	addition := benchSourceAs(b, "port/interface", "ports")

	for b.Loop() {
		if _, _, err := source.Merge(existing, addition, "EventPublisher"); err != nil {
			b.Fatalf("failed to merge: %s", err.Error())
		}
	}
}

// BenchmarkFsWrite touches the real filesystem. The writer calls MkdirAll ahead
// of every artifact, so the two cases separate building a tree from the stat
// that every artifact after the first one in a directory pays
//
// Both cases write the smallest template in the set. The delta between them is
// the directory handling, and new_dir leaves one directory behind per
// iteration, so a larger payload would cost gigabytes at the default benchtime
func BenchmarkFsWrite(b *testing.B) {
	writer := NewFsAdapter()
	ctx := context.Background()
	content := benchSource(b, "core/entity")

	b.Run("existing_dir", func(b *testing.B) {
		path := filepath.Join(b.TempDir(), "adapter.go")

		for b.Loop() {
			if err := writer.Write(ctx, path, content); err != nil {
				b.Fatalf("failed to write: %s", err.Error())
			}
		}
	})

	b.Run("new_dir", func(b *testing.B) {
		root := b.TempDir()
		depth := 0

		for b.Loop() {
			depth++
			path := filepath.Join(root, strconv.Itoa(depth), "internal", "adapters", "adapter.go")

			if err := writer.Write(ctx, path, content); err != nil {
				b.Fatalf("failed to write: %s", err.Error())
			}
		}
	})
}
