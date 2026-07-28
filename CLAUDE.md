# gopher

A CLI that generates Go code from templates encoding a fixed set of
conventions. See `README.md` for the user-facing interface and `spec.md` for the
original design brief and the canonical adapter sources.

This file is the compressed working brief. `docs/` has the same material with
the reasoning attached — reach for it when a rule here is not enough:
[architecture](docs/architecture.md) · [pipeline](docs/pipeline.md) ·
[templates](docs/templates.md) · [adding a type](docs/adding-a-type.md) ·
[testing](docs/testing.md) · [decisions](docs/decisions.md).

Keep the two in sync: changing a constraint here means changing it there.

## Hard constraints

- **Zero third-party dependencies, one first-party exception.** The `require`
  block in `go.mod` holds exactly `github.com/nxdir-s/pipelines` — first-party,
  stdlib-only, dependency-free — and nothing else, ever. Otherwise standard
  library only: `text/template`, `go/format`, `go/parser`, `embed`,
  `encoding/json`, `log/slog`, `flag`. Third-party imports that appear inside
  `templates/` belong to *generated* code, not to gopher.
- **stdlib `flag`** for parsing, one `flag.NewFlagSet` per subcommand.
- Layout is hexagonal, matching what `gopher generate setup` produces. The core
  (`internal/core`) never imports `internal/adapters`.

Rationale for each, and what was rejected, is in [docs/decisions.md](docs/decisions.md).

## Where things live

```
cmd/gopher/main.go              wire dependencies, dispatch
internal/adapters/              cli (primary), fs, template, store, format, gosource
internal/config/                config precedence and go.mod detection
internal/core/domain/           generator.go, catalog.go, registry.go
internal/core/entity/           GenSpec, Artifact, Request, TemplateData
internal/core/valobj/           GenType, Naming, Field, Method
internal/ports/                 core.go, primary.go, secondary.go
templates/files/                the embedded templates
```

## The registry is the spine

Every generatable type is one `entity.GenSpec` in
`internal/core/domain/registry.go`. That declaration drives three things: CLI
flag registration, the `describe` output, and template selection at render time.

**Adding a type means adding a spec constructor, a `specFor` case, a
`genTypes` entry, and a template — nothing else.** If a change requires
touching `internal/adapters/cli.go` to support a new type, the abstraction is
being worked around; fix the registry instead.

Four mechanisms worth knowing:

- A `TemplateRef` whose `Out` renders to an empty string is skipped. That is how
  `setup` switches `go.mod`/`Makefile`/`CLAUDE.md` on and off from a flag.
- A `TemplateRef` with `Mode: entity.ModeAppend` merges into an existing file via
  `ports.GoSource` instead of creating one. Only `port` uses it. Its duplicate
  check keys on `data.Name.Pascal`, so it only fits refs whose declaration is
  named after the `-name` flag.
- A `TemplateRef` with `Mode: entity.ModeEnsure` writes only when the file is
  missing and otherwise reports `unchanged`. It fits scaffolding at a fixed path
  that the caller then edits — `adapter -kind http` uses it for the
  `entity.Request`/`valobj.Method` types the adapter is written against. `-force`
  still regenerates.
- Create-mode refs render concurrently (`renderAll` in
  `internal/core/domain/generator.go`); append/ensure refs and specs with fewer
  than two create refs stay serial. Artifact order and the error reported are
  deterministic either way — see [pipeline](docs/pipeline.md).

## Conventions in this codebase

These are the same conventions the templates emit, so the code and its output
stay consistent.

- Errors are custom struct types with an `Error() string` method — never
  `fmt.Errorf`. Wrap the cause: `return &ErrReadFile{path, err}`. Add `Unwrap()`
  when callers need `errors.As` through it.
- Constructors take functional options: `NewXAdapter(logger, opts ...XOpt)`.
- Guard nil dependencies up front and return a typed error.
- Blank line before a `return` that follows a statement block.
- Range with an index (`for i := range items`) when reading struct slices.

## Testing

- `go test ./...` — golden files, unit tests, and compile checks
- `go test -short ./...` — skips the compile checks, which shell out to `go build`
- `go test -race ./internal/core/domain/ ./internal/adapters/` — required after
  changes to the generator's fanned rendering or the adapters it drives
- `GOPHER_UPDATE_GOLDEN=1 go test ./...` — refresh the golden files. An
  environment variable rather than a `-update` flag so a repo-wide refresh works
  in packages that do not define the flag.
- `make bench` / `make bench-quick` — benchmarks. No test run touches them and
  no baseline is committed.

Benchmarks measure four layers: the adapters, the pipeline, the CLI, and
startup. **An optimization is only real if it moves `BenchmarkGenerateCold`.**
Nothing caches a parsed template, but within one invocation every template
string is distinct — so a cache is ~100% warm in `BenchmarkGenerate`, which
reuses a generator, and ~0% warm in the run a user actually gets.

A change outside the generate path answers to the benchmark that measures it
instead — `Load` for config resolution, `NewNaming` for the value objects — but
it does not get to claim a user-visible win on that basis. Report the
`BenchmarkGenerateCold` number either way. `BenchmarkGenerateOverrides` is the
one member of the `Generate` family wired the way `config.TemplateDirs` wires a
real run; the rest build the store with no override directories.

Compare runs with `make bench BENCHFLAGS='-count 10' > old.out`, then
`go run golang.org/x/perf/cmd/benchstat@latest old.out new.out`. It stays a
`go run` so `go.mod` keeps its empty `require` block. Details, including the
rules for adding a benchmark, are in [docs/testing.md](docs/testing.md).

Golden files in `internal/core/domain/testdata/` are the regression net for
template drift. **Read the diff before refreshing them** — an unexpected golden
change means a template changed behavior.

Compile checks only cover templates whose output is standard-library-only
(`cmd`, `zip`, `generic`, the core types, `setup`, `server`, the stdlib module
kinds, and the `http` companions). Templates with third-party imports — `kafka`,
`postgres`, `http`, `aws`, `google`, `toml`, `tmux`, `observability`, `infra` —
are verified by `go/format` for syntax plus their golden files. Type errors in
those are not caught automatically; check them by hand when editing.

The `http` adapter is the one to watch: it is written against the companion
types in `templates/files/http/`, and nothing in CI proves the two still agree,
because the adapter pulls `golang.org/x/oauth2`. After editing either side, check
the pair by hand with network access:

```bash
go run ./cmd/gopher generate adapter -kind http -name Client \
  -module github.com/nxdir-s/demo -out /tmp/httpcheck
cd /tmp/httpcheck && go mod init github.com/nxdir-s/demo; go mod tidy && go build ./...
```

Golden cases that would embed `GoVersion` (`setup`, `infra`) generate with
`gomod=false`, because the `go` directive tracks the toolchain gopher was built
with and would make the golden fail on a different Go version.

## Editing templates

Templates live in `templates/files/` and are embedded with `go:embed`. The
adapter templates are near-verbatim transcriptions of `spec.md` with the type
name and package parameterized — keep them faithful. Improving the logic inside
one changes what every generated project looks like, so treat a behavior change
there as a deliberate decision, not a cleanup.
