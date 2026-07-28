# Testing

~2,000 lines of tests across four packages. The thing to understand before
changing templates is **what the tests actually prove** — and the honest answer
is that most templates are checked for syntax, not for types.

```bash
go test ./...                          # everything
go test -short ./...                   # skips the compile checks (they shell out to go build)
GOPHER_UPDATE_GOLDEN=1 go test ./...   # refresh the golden files
make bench                             # benchmarks, which no test run touches
```

## The layers

| Layer | Where | What it covers |
|---|---|---|
| Unit | `internal/core/valobj/naming_test.go` | `Naming` derivations, `ParseField`, `ParseGenType` |
| Adapter | `internal/adapters/store_test.go`, `gosource_test.go` | the lookup chain over an `fstest.MapFS`; AST merge and signature parsing |
| Config | `internal/config/config_test.go` | precedence, module detection, template dir order |
| Domain | `internal/core/domain/generator_test.go`, `catalog_test.go` | the engine against in-memory fakes |
| Golden | `internal/core/domain/golden_test.go` | every template's rendered output, byte for byte |
| Compile | `internal/core/domain/compile_test.go` | generated code actually type-checks |
| Bench | `internal/adapters/bench_test.go`, `cli_bench_test.go`, `internal/core/domain/bench_test.go`, `internal/config/bench_test.go`, `cmd/gopher/bench_test.go` | what the pipeline costs, per layer |

Create-mode refs render concurrently, so any change to the generator or an
adapter it drives must also pass the race detector — the `setup` and `infra`
golden cases push the fanned path through it:

```bash
go test -race ./internal/core/domain/ ./internal/adapters/
```

## Golden files

`internal/core/domain/testdata/*.golden` are the regression net for template
drift. Each is the concatenation of every artifact a request produces, prefixed
with `// <path>` headers, rendered through the **real** registry, store,
renderer, and formatter — only the writer is faked.

```bash
GOPHER_UPDATE_GOLDEN=1 go test ./...
git diff internal/core/domain/testdata/
```

**Read the diff before accepting it.** That is the entire review step for a
template change. An unexpected golden change means a template changed behavior.

Refreshing is driven by an environment variable rather than a `-update` flag
because `go test ./... -update` fails in every package that does not define the
flag, which makes a repo-wide refresh impossible. The constant is `UpdateEnv` in
`golden_test.go`.

Two cases deliberately generate with `go.mod` switched off — `setup` and
`infra`. Their `go.mod` embeds `GoVersion`, which tracks the toolchain gopher
was built with, and would make the golden fail on a different Go version. The
compile checks cover those files instead.

## What is really type-checked

Goldens prove output *parses* — `go/format` rejects anything that does not.
Only `go build` proves it *type-checks*, and that needs the imports to resolve.
Templates with third-party imports cannot be built in a hermetic test, so:

| Verified by | Templates |
|---|---|
| `go build` + `go vet` | `generic`, `cmd`, `zip`, `entity`, `valobj`, `domain`, `port`, `setup`, `server`, `mocks`, `test`, the stdlib `module` kinds, the `http` companions |
| `go/format` + golden only | `kafka`, `postgres`, `http`, `aws`, `google`, `toml`, `tmux`, `observability`, `infra` |

A type error in the second group — a renamed field, a wrong argument count —
**will not be caught by CI**. Check those by hand when editing.

The compile tests write into a `t.TempDir()` with a synthetic `go.mod`, run
`go build ./...` and sometimes `go vet ./...`, and skip when `-short` is set or
no `go` binary is on `PATH`.

The most valuable of them is `TestCoreTypesCompileTogether`: it generates an
entity, two ports, and a domain that drives them, into one module. That is the
test that catches a mismatch between the port append path and the module-aware
imports — the pieces only fail together.

### The http adapter

`adapter -kind http` is the one case where two template groups must agree and
nothing in CI proves they do. The adapter is written against the companion types
in `templates/files/http/`, but it imports `golang.org/x/oauth2`, so it cannot be
built offline. `TestHttpCompanionsCompile` renders the pair, writes only the
companions, and builds those.

After editing either side, check the whole thing by hand, with network:

```bash
go run ./cmd/gopher generate adapter -kind http -name Client \
  -module github.com/nxdir-s/demo -out /tmp/httpcheck
cd /tmp/httpcheck && go mod init github.com/nxdir-s/demo 2>/dev/null
go mod tidy && go build ./... && go vet ./...
```

## Invariant tests

These fail loudly on a malformed registry, long before a user would hit it.

| Test | Enforces |
|---|---|
| `TestRegistryIsWellFormed` | unique type per spec; non-empty summary; ≥1 template ref with both `Name` and `Out`; unique flag names; every flag has `Usage`; no required-flag-with-default; bool defaults are literally `"true"`/`"false"` |
| `TestRegistryTemplatesResolve` | every static template name (no `{{`) exists in the embedded set |
| `TestKindsHaveTemplates` | every advertised kind in `AdapterKinds()`, `ValobjKinds()`, `ModuleKinds()` has a template. `PortSides()` is excluded — a side names the ports file a declaration is appended to, not a template |

`TestRegistryTemplatesResolve` skips names containing `{{`, since those resolve
per-request. `TestKindsHaveTemplates` is what covers those.

## Fakes, not mocks

`internal/adapters/fake` holds hand-written in-memory implementations of the
secondary ports — `fake.Store` for `TemplateSource`/`TemplateCatalog`,
`fake.Writer` for `FileWriter`. They record rather than assert, matching the
interface-at-the-boundary style the generated code uses. `gopher generate mocks`
emits the same shape.

There is no mocking framework, and adding one would cost the zero-dependency
property.

Domain tests build a generator two ways:

- `newTestGenerator` — a hand-built spec over `fake.Store`, for engine behavior
  in isolation (clobber, force, dry-run, invalid Go, missing flags)
- `newEmbeddedGenerator` / `newEmbeddedGeneratorWith` — the real registry and
  real embedded templates, for goldens and compile checks

Reach for the first when testing the *engine*, the second when testing
*templates*.

## Benchmarks

The performance model behind these numbers — where the time actually goes,
the floors that will not move, and the optimization history — is in
[performance](performance.md).

Benchmarks measure, they do not assert. Nothing fails on a slow number and no
baseline is checked in — the comparison is something you run deliberately,
against a run you captured yourself.

```bash
make bench                                  # everything, -benchmem, tests skipped
make bench-quick                            # -benchtime 10x, proves they still run
make bench BENCH=Generate                   # one selector
make bench BENCH=Generate BENCHFLAGS='-count 10'
```

The target pins `-run '^$'`. Without it `go test -bench` runs the whole suite,
compile checks included, before the first benchmark.

### The rule for reading them

**An optimization is only real if it moves `BenchmarkGenerateCold`.**

Nothing caches a parsed template, and the obvious fix is to add a cache. But
within a single invocation every template string the renderer sees is distinct —
`setup` performs 42 parses across 14 refs and repeats none of them. A cache keyed
by template text has a ~0% hit rate for one process, and a ~100% hit rate in a
benchmark that reuses one generator across thousands of iterations.

So the two exist as a pair:

| | Generator built | Reports |
|---|---|---|
| `BenchmarkGenerate` | once, before the loop | steady state, flattered by anything that warms up |
| `BenchmarkGenerateCold` | inside the loop | what one `gopher generate` costs |

A change that improves the first and not the second has improved nothing a user
will ever run.

### The layers

| Selector | Covers |
|---|---|
| `Template`, `Format`, `Store`, `GoSource` | the adapters, individually. `TemplateParse` and `TemplateExecute` split `Render` in half, and `TemplateRender/static` is the action-free path that skips both |
| `NewNaming`, `ParseFields` | the value-object derivations templates call per name |
| `Generate`, `TemplateData`, `RegistrySpec` | the pipeline end to end, through the in-memory writer |
| `GenerateOverrides` | the same, wired the way production wires it |
| `CliRun` | argv to `Request`, with a generator stub in place |
| `Load`, `FindModule`, `Run`, `Startup` | config resolution and startup, in and out of process |

Everything writes to an in-memory `fake.Writer` except `GenerateDisk`, `FsWrite`
and `Startup`, which are the only three that touch the disk or spawn a process —
`make bench BENCH='Disk|Fs|Startup'` selects exactly that set.

Every benchmark in the `Generate` family except `GenerateOverrides` builds its
store with **no override directories**, which is not how `config.TemplateDirs`
wires one — it always returns the user directory and never checks that anything
exists. So the lookup chain those runs measure is one embedded read, while a
real invocation pays a failed `os.ReadFile` per configured directory per
template first. `GenerateOverrides` is the one that models production, with two
directories that do not exist ahead of the embedded defaults. Read it against
`GenerateCold`, whose labels it shares, to see what resolution costs.

`BenchmarkStartup` is a budget, not a target: process spawn is milliseconds and
dwarfs a generate measured in microseconds. It is useful for two subtractions —
`generate_entity` minus `version` gives the generate delta at process scale, and
`BenchmarkStartup/version` minus `BenchmarkRun/version` gives the spawn overhead
that is not gopher's to fix.

### Comparing runs

`benchstat` is run straight from the module proxy. Adding it to `go.mod` would
cost the zero-dependency property for a tool that never ships in the binary.

```bash
make bench BENCHFLAGS='-count 10' > old.out
# ... make the change ...
make bench BENCHFLAGS='-count 10' > new.out
go run golang.org/x/perf/cmd/benchstat@latest old.out new.out
```

`-count 10` is not optional. Below six samples benchstat prints `± ∞` and
refuses to give a confidence interval at all. Both runs need identical flags,
which is why `-benchmem` and `-run '^$'` live inside the target rather than in
the command you type.

`*.out` is already ignored, so neither file is committed.

Calibrate against the machine before trusting a result. Two identical runs of
the `Generate` set on an M2 Pro land within 1–3.5% of each other, and benchstat
calls several of those differences significant — a low `p` means the difference
is consistent, not that it is large enough to care about. Anything under about
5% is the floor, not a finding. Close everything else, stay plugged in, and run
it twice against itself first. On macOS, work migrating between performance and
efficiency cores is the usual cause.

### Adding one

Benchmark names are an API: `benchstat` matches samples by their exact full
name, so renaming a sub-benchmark orphans every baseline anyone captured. Keep
the names flat, lowercase, and one `/` deep.

Use `for b.Loop()`, not `for i := 0; i < b.N; i++`. It resets the timer when the
loop starts, so setup in the same function is already excluded, and it keeps
arguments and results alive, so no sink variable is needed — which matters here,
because a package-level sink would break the no-package-level-variables rule in
[decisions.md](decisions.md).

Watch for state carried between iterations. Requests run with `Stdout` set
because that returns before the existence check and the write loop, leaving the
fake writer untouched; `BenchmarkGenerateWrite` sets `Force` instead, because
otherwise the second iteration hits the clobber check and quietly measures the
error path. To find a leak, run at two iteration counts and compare — a number
that drifts more than ~10% depends on how many iterations preceded it.

Give both counts enough iterations to be meaningful. A sub-microsecond
benchmark like `NewNaming` reads roughly 2x high at `-benchtime 20000x` and
only settles from about `200000x`, which looks exactly like a leak and is not
one.

Anything reading config or the store's default directories needs
`b.Setenv(config.XdgConfigEnv, b.TempDir())`, or the result depends on whether
the machine happens to have user templates installed.

If you add a request to `benchRequests`, add its artifact count to
`TestBenchRequestsProduceOutput` in the same file. That test is the only thing
standing between a benchmark and measuring nothing: a request whose flags drift
out of step with its spec still generates without error and still reports a
stable number, it just stops covering what its name says it does. The goldens
do not help here — they run off a table of their own.

## Adding a test

Follow the surrounding style: table tests keyed by a descriptive name, `t.Run`
per case, `errors.As` against the concrete error type rather than string
matching. Error types that wrap implement `Unwrap`, so `errors.As` reaches
through:

```go
var exists *ErrFileExists
if !errors.As(err, &exists) {
    t.Fatalf("expected ErrFileExists, got %v", err)
}
```

For a new generation type, see the golden and compile-check steps in
[adding-a-type.md](adding-a-type.md).
