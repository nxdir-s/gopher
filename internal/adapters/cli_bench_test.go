package adapters

import (
	"context"
	"io"
	"testing"

	"github.com/nxdir-s/gopher/internal/config"
	"github.com/nxdir-s/gopher/internal/core/domain"
	"github.com/nxdir-s/gopher/internal/core/entity"
)

// newBenchCli builds a cli over the real registry, discarding its output. The
// tests use a spec set of their own so their assertions do not move when the
// registry does, but a benchmark wants the flag counts the real specs declare,
// since registering them is most of what the generate path measures here
//
// It does not reuse newTestCli: those buffers keep every byte written across
// every iteration, which turns the number into a measure of buffer growth
func newBenchCli(t testing.TB) (*CliAdapter, *testGenerator) {
	t.Helper()

	generator := &testGenerator{}
	catalog := &testCatalog{infos: []*entity.TemplateInfo{}}
	cfg := &config.Config{OutDir: ".", Defaults: map[string]string{}}

	cli := NewCliAdapter(generator, catalog, domain.NewRegistry(), cfg, "1.2.3",
		WithStdout(io.Discard),
		WithStderr(io.Discard),
	)

	return cli, generator
}

// BenchmarkCliRun measures dispatch and flag handling with a generator stub in
// place, so nothing here renders a template. The generate cases are the cost of
// turning argv into a request: a new flag set, five globals, every flag the spec
// declares, and the parse
func BenchmarkCliRun(b *testing.B) {
	cli, _ := newBenchCli(b)
	ctx := context.Background()

	invocations := map[string][]string{
		"version":        {VersionCmd},
		"list":           {ListCmd},
		"describe":       {DescribeCmd, "adapter"},
		"describe_json":  {DescribeCmd, "adapter", "-json"},
		"templates_list": {TemplatesCmd, ListSubCmd},
		"generate_minimal": {
			GenerateCmd, "adapter", "-name", "Events",
		},
		"generate_full": {
			GenerateCmd, "mocks",
			"-name", "OrderRepository",
			"-pkg", "fake",
			"-dir", "internal/adapters/fake",
			"-method", "Save(ctx context.Context, order *entity.Order) error",
			"-method", "Find(ctx context.Context, id int) (*entity.Order, error)",
			"-method", "Close()",
			"-import", "context",
			"-out", ".",
			"-module", "github.com/nxdir-s/demo",
			"-stdout",
		},
	}

	for label, args := range invocations {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if code := cli.Run(ctx, args); code != ExitOk {
					b.Fatalf("got exit code %d, want %d", code, ExitOk)
				}
			}
		})
	}
}
