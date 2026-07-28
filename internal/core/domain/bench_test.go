package domain

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/adapters/fake"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/templates"
)

// benchModule is the module path the benchmark requests generate against
const benchModule string = "github.com/nxdir-s/demo"

// benchPorts is the file the append benchmarks merge into
const benchPorts string = `package ports

import "context"

// OrderRepository defines how the core drives the order repository
type OrderRepository interface {
	Save(ctx context.Context, id int) error
}
`

// benchRequests returns one request per distinct cost class rather than one per
// golden case. The registry has 25 golden combinations but most repeat a shape
// already covered here, and every extra case is paid again on both sides of a
// benchstat comparison
//
// setup is the ceiling at 14 refs, 42 renders and 10 formats. adapter_aws is
// render bound on the largest template in the set, adapter_http is format bound
// and the only create and ensure mix, port is the only append mode ref, and
// entity is the floor at one small template
func benchRequests() map[string]*entity.Request {
	requests := map[string]*entity.Request{
		"setup": {
			Type:  valobj.GenSetup,
			Flags: map[string]string{"name": "demo", "gomod": "false", "makefile": "true", "claude": "true"},
		},
		"adapter_aws": {
			Type: valobj.GenAdapter,
			Flags: map[string]string{
				"name": "Events", "kind": "aws", "pkg": "adapters",
				"tracer": entity.BoolTrue, "logger": entity.BoolTrue,
			},
		},
		"adapter_http": {
			Type: valobj.GenAdapter,
			Flags: map[string]string{
				"name": "Client", "kind": "http", "pkg": "adapters",
				"tracer": entity.BoolTrue, "logger": entity.BoolTrue,
			},
		},
		"infra": {
			Type:  valobj.GenInfra,
			Flags: map[string]string{"name": "Orders", "dir": "infra", "gomod": "false"},
		},
		"port": {
			Type:  valobj.GenPort,
			Flags: map[string]string{"name": "OrderRepository", "side": "secondary", "pkg": "ports"},
			Lists: map[string][]string{
				"method": {
					"Save(ctx context.Context, order *entity.Order) error",
					"Find(ctx context.Context, id int) (*entity.Order, error)",
				},
			},
		},
		"entity": {
			Type:  valobj.GenEntity,
			Flags: map[string]string{"name": "Order", "pkg": "entity", "json": entity.BoolFalse},
		},
		"domain": {
			Type:  valobj.GenDomain,
			Flags: map[string]string{"name": "Orders", "pkg": "domain", "logger": entity.BoolTrue, "tracer": entity.BoolFalse},
			Lists: map[string][]string{"port": {"repo:ports.OrderRepository", "events:ports.EventPublisher"}},
		},
		"mocks": {
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
	}

	// stdout returns before the existence check and the write loop, so the fake
	// writer is never mutated and no iteration can observe the one before it
	for name := range requests {
		requests[name].Module = benchModule
		requests[name].Stdout = true
	}

	return requests
}

// benchRequest returns a single named request from the shared set
func benchRequest(t testing.TB, name string) *entity.Request {
	t.Helper()

	req, ok := benchRequests()[name]
	if !ok {
		t.Fatalf("no benchmark request named %s", name)
	}

	return req
}

// TestBenchRequestsProduceOutput guards the benchmarks against measuring
// nothing. A request whose flags stop matching the spec, or whose refs all
// render an empty Out, still generates without error and still reports a
// perfectly stable number, it just stops covering what its name claims
//
// The counts are asserted rather than merely checked for being non zero,
// because a request that quietly emits fewer artifacts than it used to is the
// same failure in a milder form. Nothing else covers this set: the golden files
// are built from a table of their own
func TestBenchRequestsProduceOutput(t *testing.T) {
	artifactCounts := map[string]int{
		"setup":        13, // 14 refs, gomod is off
		"adapter_aws":  1,  // the four http refs render an empty Out
		"adapter_http": 5,
		"infra":        3, // 4 refs, gomod is off
		"port":         1,
		"entity":       1,
		"domain":       1,
		"mocks":        1,
	}

	generator := newEmbeddedGenerator(t)

	for label, req := range benchRequests() {
		t.Run(label, func(t *testing.T) {
			want, ok := artifactCounts[label]
			if !ok {
				t.Fatalf("no artifact count recorded for %s, add one alongside the request", label)
			}

			artifacts, err := generator.Generate(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if len(artifacts) != want {
				t.Errorf("got %d artifacts, want %d", len(artifacts), want)
			}

			for i := range artifacts {
				if len(artifacts[i].Content) == 0 {
					t.Errorf("got empty content for %s", artifacts[i].Path)
				}
			}
		})
	}
}

// BenchmarkGenerate measures the pipeline against a generator built once, so it
// reports steady state cost. A change that only helps because work carries over
// between iterations will show up here and not in BenchmarkGenerateCold
func BenchmarkGenerate(b *testing.B) {
	generator := newEmbeddedGenerator(b)
	ctx := context.Background()

	for label, req := range benchRequests() {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := generator.Generate(ctx, req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGenerateCold builds the generator inside the loop, so every
// iteration starts from the state a fresh process would. This is the number an
// optimization has to move: within one invocation every template string the
// renderer sees is distinct, so anything that only pays off once a cache is warm
// improves BenchmarkGenerate and leaves this untouched
func BenchmarkGenerateCold(b *testing.B) {
	ctx := context.Background()

	for _, label := range []string{"entity", "adapter_aws", "setup"} {
		req := benchRequest(b, label)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				generator, err := NewGenerator(slog.New(slog.DiscardHandler),
					WithRegistry(NewRegistry()),
					WithTemplateSource(adapters.NewStoreAdapter(templates.FS, templates.Root)),
					WithRenderer(adapters.NewTemplateAdapter()),
					WithFormatter(adapters.NewFormatAdapter()),
					WithFileWriter(fake.NewWriter()),
					WithMerger(adapters.NewGoSourceAdapter()),
				)
				if err != nil {
					b.Fatalf("failed to create generator: %s", err.Error())
				}

				if _, err := generator.Generate(ctx, req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGenerateOverrides is BenchmarkGenerateCold wired the way config
// wires it in production: two override directories ahead of the embedded
// defaults, neither of which exists. Every other Generate benchmark builds the
// store with no override dirs at all, so none of them can see what the lookup
// chain costs
//
// The dirs are subpaths under a temp dir rather than the real project and user
// directories, so the number does not depend on whether the machine running it
// has overrides installed
func BenchmarkGenerateOverrides(b *testing.B) {
	ctx := context.Background()

	for _, label := range []string{"entity", "setup"} {
		req := benchRequest(b, label)

		b.Run(label, func(b *testing.B) {
			root := b.TempDir()
			project := filepath.Join(root, "project", ".gopher", "templates")
			user := filepath.Join(root, "user", "gopher", "templates")

			for b.Loop() {
				generator, err := NewGenerator(slog.New(slog.DiscardHandler),
					WithRegistry(NewRegistry()),
					WithTemplateSource(adapters.NewStoreAdapter(templates.FS, templates.Root,
						adapters.WithTemplateDir(project),
						adapters.WithTemplateDir(user),
					)),
					WithRenderer(adapters.NewTemplateAdapter()),
					WithFormatter(adapters.NewFormatAdapter()),
					WithFileWriter(fake.NewWriter()),
					WithMerger(adapters.NewGoSourceAdapter()),
				)
				if err != nil {
					b.Fatalf("failed to create generator: %s", err.Error())
				}

				if _, err := generator.Generate(ctx, req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGenerateAppend measures the only append mode ref in the registry
// against a ports file that already exists. The merge case declares a name the
// file does not hold, so it parses the existing source, the addition, and then
// merges. The declared case matches a name already present and stops after the
// first parse
//
// Both run with stdout set so the seed is read every iteration but never
// rewritten. Were it rewritten, the second iteration onward would take the
// declared path while still claiming to measure a merge
func BenchmarkGenerateAppend(b *testing.B) {
	ctx := context.Background()

	names := map[string]string{"merge": "EventPublisher", "declared": "OrderRepository"}

	for label, name := range names {
		writer := fake.NewWriter()
		writer.Seed("internal/ports/secondary.go", benchPorts)

		generator := newEmbeddedGeneratorWith(b, writer)

		req := &entity.Request{
			Type:   valobj.GenPort,
			Flags:  map[string]string{"name": name, "side": "secondary", "pkg": "ports"},
			Lists:  map[string][]string{"method": {"Publish(ctx context.Context, id int) error"}},
			Module: benchModule,
			Stdout: true,
		}

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := generator.Generate(ctx, req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGenerateWrite drives the write path through the in memory writer.
// Read against BenchmarkGenerate it isolates the existence check, the write loop
// and the per artifact debug log without any syscall in the way
//
// Force is required. Without it the second iteration finds the paths the first
// one wrote and fails the clobber check, which would quietly turn this into a
// benchmark of the error path
func BenchmarkGenerateWrite(b *testing.B) {
	ctx := context.Background()

	for _, label := range []string{"entity", "setup"} {
		req := *benchRequest(b, label)
		req.Stdout = false
		req.Force = true
		req.OutDir = "out"

		generator := newEmbeddedGenerator(b)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := generator.Generate(ctx, &req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGenerateDisk drives the real filesystem writer. The temp dir is
// created once and Force is set, so every iteration overwrites the same paths
//
// That makes this steady state cost, not first run cost: MkdirAll takes its
// stat path and WriteFile truncates a file that already exists. A fresh
// directory per iteration would measure the create path instead, at the price of
// leaving a full copy of the output behind every time, which for setup is
// fourteen files a run
func BenchmarkGenerateDisk(b *testing.B) {
	ctx := context.Background()

	for _, label := range []string{"entity", "adapter_aws", "setup"} {
		req := *benchRequest(b, label)
		req.Stdout = false
		req.Force = true

		b.Run(label, func(b *testing.B) {
			req.OutDir = b.TempDir()
			generator := newEmbeddedGeneratorWith(b, adapters.NewFsAdapter())

			for b.Loop() {
				if _, err := generator.Generate(ctx, &req); err != nil {
					b.Fatalf("failed to generate: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkTemplateData measures the payload build that runs once per generate,
// covering both field list parses, the method parse and the naming derivation
func BenchmarkTemplateData(b *testing.B) {
	generator := newEmbeddedGenerator(b)
	registry := NewRegistry()

	for _, label := range []string{"entity", "mocks", "setup"} {
		req := benchRequest(b, label)

		spec, err := registry.Spec(req.Type)
		if err != nil {
			b.Fatalf("failed to resolve spec: %s", err.Error())
		}

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := generator.templateData(spec, req); err != nil {
					b.Fatalf("failed to build template data: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkRegistrySpec measures the linear scan every generate opens with
func BenchmarkRegistrySpec(b *testing.B) {
	registry := NewRegistry()

	for b.Loop() {
		if _, err := registry.Spec(valobj.GenTest); err != nil {
			b.Fatalf("failed to resolve spec: %s", err.Error())
		}
	}
}
