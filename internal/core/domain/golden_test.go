package domain

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/adapters/fake"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/internal/ports"
	"github.com/nxdir-s/gopher/templates"
)

// UpdateEnv refreshes the golden files when set. An environment variable is
// used instead of a flag so 'go test ./...' works across every package
const UpdateEnv string = "GOPHER_UPDATE_GOLDEN"

// newEmbeddedGenerator builds a generator over the real registry and the
// templates compiled into the binary
func newEmbeddedGenerator(t testing.TB) *Generator {
	t.Helper()

	return newEmbeddedGeneratorWith(t, fake.NewWriter())
}

// newEmbeddedGeneratorWith builds a generator over the real registry and
// templates, emitting through the supplied writer
func newEmbeddedGeneratorWith(t testing.TB, writer ports.FileWriter) *Generator {
	t.Helper()

	generator, err := NewGenerator(slog.New(slog.DiscardHandler),
		WithRegistry(NewRegistry()),
		WithTemplateSource(adapters.NewStoreAdapter(templates.FS, templates.Root)),
		WithRenderer(adapters.NewTemplateAdapter()),
		WithFormatter(adapters.NewFormatAdapter()),
		WithFileWriter(writer),
		WithMerger(adapters.NewGoSourceAdapter()),
	)
	if err != nil {
		t.Fatalf("failed to create generator: %s", err.Error())
	}

	return generator
}

// TestGoldenTemplates renders the embedded templates and compares them against
// the checked in output. Set GOPHER_UPDATE_GOLDEN to refresh
func TestGoldenTemplates(t *testing.T) {
	kinds := AdapterKinds()

	tests := make([]struct {
		golden string
		req    *entity.Request
	}, 0, len(kinds)+1)

	for _, kind := range kinds {
		tests = append(tests, struct {
			golden string
			req    *entity.Request
		}{
			golden: "adapter_" + kind,
			req: &entity.Request{
				Type: valobj.GenAdapter,
				Flags: map[string]string{
					"name":   "Events",
					"kind":   kind,
					"pkg":    "adapters",
					"tracer": "true",
					"logger": "true",
				},
				Module: "github.com/nxdir-s/demo",
				Stdout: true,
			},
		})
	}

	extra := []struct {
		golden string
		req    *entity.Request
	}{
		{
			golden: "adapter_generic_minimal",
			req: &entity.Request{
				Type: valobj.GenAdapter,
				Flags: map[string]string{
					"name":   "http cache",
					"kind":   "generic",
					"pkg":    "adapters",
					"tracer": "false",
					"logger": "false",
				},
			},
		},
		{
			golden: "core_entity",
			req: &entity.Request{
				Type:  valobj.GenEntity,
				Flags: map[string]string{"name": "Order", "pkg": "entity", "json": "true"},
				Lists: map[string][]string{
					"field":  {"ID:int", "Total:float64", "placed at:time.Time"},
					"import": {"time"},
				},
			},
		},
		{
			golden: "core_entity_empty",
			req: &entity.Request{
				Type:  valobj.GenEntity,
				Flags: map[string]string{"name": "Order", "pkg": "entity", "json": "false"},
			},
		},
		{
			golden: "valobj_struct",
			req: &entity.Request{
				Type:  valobj.GenValobj,
				Flags: map[string]string{"name": "Header", "kind": "struct", "pkg": "valobj"},
				Lists: map[string][]string{"field": {"Key:string", "Value:string", "Enabled:bool"}},
			},
		},
		{
			golden: "valobj_enum",
			req: &entity.Request{
				Type:  valobj.GenValobj,
				Flags: map[string]string{"name": "Status", "kind": "enum", "pkg": "valobj"},
				Lists: map[string][]string{"value": {"Pending", "in transit", "Delivered"}},
			},
		},
		{
			golden: "core_domain",
			req: &entity.Request{
				Type:  valobj.GenDomain,
				Flags: map[string]string{"name": "Orders", "pkg": "domain", "logger": "true", "tracer": "false"},
				Lists: map[string][]string{"port": {"repo:ports.OrderRepository", "events:ports.EventPublisher"}},
			},
		},
		{
			golden: "core_domain_bare",
			req: &entity.Request{
				Type:  valobj.GenDomain,
				Flags: map[string]string{"name": "Orders", "pkg": "domain", "logger": "false", "tracer": "false"},
			},
		},
		{
			golden: "port_secondary",
			req: &entity.Request{
				Type:  valobj.GenPort,
				Flags: map[string]string{"name": "OrderRepository", "side": "secondary", "pkg": "ports"},
				Lists: map[string][]string{
					"method": {"Save(ctx context.Context, order *entity.Order) error", "Find(ctx context.Context, id int) (*entity.Order, error)"},
				},
			},
		},
		{
			// go.mod is left out because its go directive tracks the toolchain
			// gopher was built with, which would make the golden unstable. The
			// compile check covers it instead
			golden: "setup",
			req: &entity.Request{
				Type:  valobj.GenSetup,
				Flags: map[string]string{"name": "demo", "gomod": "false", "makefile": "true"},
			},
		},
		{
			golden: "server_http",
			req: &entity.Request{
				Type:  valobj.GenServer,
				Flags: map[string]string{"name": "API", "pkg": "adapters", "port": "8080", "logger": "true", "tracer": "false"},
			},
		},
		{
			golden: "mocks_fake",
			req: &entity.Request{
				Type:  valobj.GenMocks,
				Flags: map[string]string{"name": "OrderRepository", "pkg": "fake", "dir": "internal/adapters/fake"},
				Lists: map[string][]string{
					"method": {
						"Save(ctx context.Context, order *entity.Order) error",
						"Find(ctx context.Context, id int) (*entity.Order, error)",
						"Close()",
					},
					"import": {"context"},
				},
			},
		},
		{
			golden: "test_table",
			req: &entity.Request{
				Type:  valobj.GenTest,
				Flags: map[string]string{"name": "ParseOrder", "pkg": "domain", "dir": "internal/core/domain"},
				Lists: map[string][]string{"case": {"rejects an empty order", "parses a valid order"}},
			},
		},
		{
			// go.mod is left out because its go directive tracks the toolchain
			golden: "infra_cdk",
			req: &entity.Request{
				Type:  valobj.GenInfra,
				Flags: map[string]string{"name": "Orders", "dir": "infra", "gomod": "false"},
			},
		},
		{
			golden: "module_generic",
			req: &entity.Request{
				Type:  valobj.GenModule,
				Flags: map[string]string{"name": "auth", "kind": "generic"},
			},
		},
		{
			golden: "port_primary",
			req: &entity.Request{
				Type:  valobj.GenPort,
				Flags: map[string]string{"name": "API", "side": "primary", "pkg": "ports"},
				Lists: map[string][]string{"method": {"PlaceOrder(ctx context.Context, total float64) (int, error)"}},
			},
		},
	}

	for i := range extra {
		extra[i].req.Module = "github.com/nxdir-s/demo"
		extra[i].req.Stdout = true

		tests = append(tests, extra[i])
	}

	generator := newEmbeddedGenerator(t)

	for _, test := range tests {
		t.Run(test.golden, func(t *testing.T) {
			artifacts, err := generator.Generate(context.Background(), test.req)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			var buf bytes.Buffer
			for i := range artifacts {
				buf.WriteString("// " + filepath.ToSlash(artifacts[i].Path) + "\n")
				buf.Write(artifacts[i].Content)
			}

			path := filepath.Join("testdata", test.golden+".golden")

			if len(os.Getenv(UpdateEnv)) > 0 {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("failed to create testdata: %s", err.Error())
				}

				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatalf("failed to update golden: %s", err.Error())
				}

				return
			}

			expected, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read golden, run '%s=1 go test ./...': %s", UpdateEnv, err.Error())
			}

			if !bytes.Equal(buf.Bytes(), expected) {
				t.Errorf("output does not match %s, refresh with %s=1 and review the diff\n--- got ---\n%s\n--- want ---\n%s", path, UpdateEnv, buf.String(), expected)
			}
		})
	}
}
