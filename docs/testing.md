# Testing

~2,000 lines of tests across four packages. The thing to understand before
changing templates is **what the tests actually prove** — and the honest answer
is that most templates are checked for syntax, not for types.

```bash
go test ./...                          # everything
go test -short ./...                   # skips the compile checks (they shell out to go build)
GOPHER_UPDATE_GOLDEN=1 go test ./...   # refresh the golden files
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
