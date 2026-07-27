# Adding to the generator

Two walkthroughs, because the two common changes are very different sizes.

The invariant behind both: **the registry is the single source of truth.** One
`entity.GenSpec` in `internal/core/domain/registry.go` drives CLI flag
registration, the `describe` output, and template selection. If a change here
makes you edit `internal/adapters/cli.go`, stop — the abstraction is being
worked around.

---

## A. A new adapter kind

Say `redis`. Four steps.

**1. Write the template.** `templates/files/adapter/redis.tmpl`, following the
conventions in [templates.md](templates.md) — custom error structs, functional
options, nil-client guards:

```
package {{.Package}}

import (
	"context"
	"log/slog"
)

type ErrNil{{.Name.Pascal}}Client struct{}

func (e *ErrNil{{.Name.Pascal}}Client) Error() string {
	return "error nil client in {{.Name.Pascal}}Adapter"
}

type {{.Name.Pascal}}Opt func(a *{{.Name.Pascal}}Adapter) error
...
```

**2. Advertise it.** Add `"redis"` to `AdapterKinds` in
`internal/core/domain/registry.go`. That slice feeds the `-kind` usage string
and `TestKindsHaveTemplates`, which will now fail until step 1 exists.

**3. Add a golden case.** The golden test in
`internal/core/domain/golden_test.go` already loops over `AdapterKinds`, so
there is nothing to write — just generate the file:

```bash
GOPHER_UPDATE_GOLDEN=1 go test ./internal/core/domain/
```

**4. Check the diff and the output.**

```bash
git diff internal/core/domain/testdata/          # should be one new file
go test ./...
go run ./cmd/gopher generate adapter -kind redis -name Cache -stdout
```

If the kind's output is standard-library-only, also add it to `stdlibKinds` in
`internal/core/domain/compile_test.go` so it gets a real `go build`.

---

## B. A new generation type

Say `middleware`, emitting `internal/adapters/middleware/<name>.go`.

### 1. Add the `GenType`

`internal/core/valobj/gentype.go` — a constant, a `genTypeNames` entry, and an
alias if the type has an obvious second name:

```go
GenMiddleware               // in the iota block
GenMiddleware: "middleware" // in genTypeNames
```

**Append new constants at the end of the iota block.** The values are not
persisted anywhere, but reordering churns every switch and map in one commit for
no reason.

### 2. Add the spec

In `specs` in `internal/core/domain/registry.go`. Every field:

```go
{
    Type:           valobj.GenMiddleware,
    Summary:        "generate an http middleware",  // shown by `list`
    RequiresModule: false,                          // true if templates need .Module
    Flags: []entity.FlagSpec{
        {
            Name:     "name",
            Usage:    "middleware name, ex. RequestID",  // required, non-empty
            Required: true,                              // then no Default
        },
        {
            Name:    "pkg",
            Usage:   "package name",
            Default: "middleware",
        },
        {
            Name:  "import",
            Usage: "import path required by the handler",
            Type:  entity.FlagList,   // repeatable → lands in .Lists
        },
        {
            Name:    "logger",
            Usage:   "include an slog logger",
            Type:    entity.FlagBool, // Default must be "true" or "false"
            Default: "false",
        },
    },
    Templates: []entity.TemplateRef{
        {
            Name: "middleware/handler",                              // template to load
            Out:  "internal/adapters/middleware/{{.Name.Snake}}.go", // where it goes
            Mode: entity.ModeCreate,                                 // the zero value
        },
    },
},
```

Both `Name` and `Out` are rendered as templates first, so either can branch on a
flag. An `Out` that renders empty drops the artifact — that is how optional
files work. See [pipeline.md](pipeline.md).

Flag names may not collide with the globals (`out`, `module`, `force`,
`dry-run`, `stdout`); the CLI rejects that at runtime with `ErrDuplicateFlag`.

### 3. Write the template

`templates/files/middleware/handler.tmpl`. Namespace the directory by type so
`gopher templates list` stays readable and names cannot collide with an existing
kind lookup.

### 4. Add a golden case

In `TestGoldenTemplates` in `internal/core/domain/golden_test.go`, append to the
`extra` slice:

```go
{
    golden: "middleware_handler",
    req: &entity.Request{
        Type:  valobj.GenMiddleware,
        Flags: map[string]string{"name": "RequestID", "pkg": "middleware", "logger": "false"},
        Lists: map[string][]string{"import": {"net/http"}},
    },
},
```

The loop stamps `Module` and `Stdout` on every entry, so leave those out. If the
output embeds `GoVersion`, generate that case with the `go.mod` switched off —
the directive tracks the local toolchain and would make the golden fail on a
different Go version.

### 5. Add a compile check if you can

If the output imports only the standard library, add it to
`TestCompositesCompile` in `internal/core/domain/compile_test.go`. Goldens prove
the output *parses*; only `go build` proves it *type-checks*. See the coverage
matrix in [testing.md](testing.md).

### 6. Refresh, verify, document

```bash
GOPHER_UPDATE_GOLDEN=1 go test ./...
git diff internal/core/domain/testdata/
go build ./... && go vet ./... && go test ./...

go run ./cmd/gopher list
go run ./cmd/gopher describe middleware -json
go run ./cmd/gopher generate middleware -name RequestID -stdout
```

Then add a row to the types table in the root [README.md](../README.md).

---

## Checklist

```
[ ] GenType constant + genTypeNames entry
[ ] GenSpec in registry.go — Summary, every flag with a Usage
[ ] template under templates/files/<type>/
[ ] golden case in golden_test.go
[ ] compile check if the output is stdlib-only
[ ] goldens refreshed AND the diff read
[ ] go build && go vet && go test
[ ] `list` / `describe` / `generate` tried by hand
[ ] row added to the README types table
[ ] internal/adapters/cli.go NOT touched
```

## Common mistakes

**A flag with no `Usage`.** `TestRegistryIsWellFormed` fails. It also enforces:
unique type per spec, non-empty summary, at least one template ref, complete
`Name`/`Out` on each ref, unique flag names, and that a bool flag's default is
literally `"true"` or `"false"`.

**A required flag with a default.** Also caught by `TestRegistryIsWellFormed` —
the two are contradictory, and the CLI would silently satisfy the requirement
with the default.

**Reaching for `ModeAppend` when you want `ModeEnsure`.** Append's idempotency
check looks for a declaration named after `-name`. If the file's declarations
are *not* named after `-name`, it will never match and every run appends
duplicates. Fixed-path scaffolding wants `ModeEnsure`. See [pipeline.md](pipeline.md).

**Forgetting to refresh goldens**, or refreshing without reading the diff. The
diff is the review — an unexpected change means a template changed behavior.

**Indexing a flag the spec does not declare.** `missingkey=error` turns
`.Flags.nope` into a failed command. Declare it or do not read it.

**Assuming imports are inferred.** They are not; see [templates.md](templates.md).

**Editing `cli.go` to make a type work.** The registry drives flag registration.
`cli.go` changes only when adding a top-level *command* — a sibling of
`generate`, `list`, `describe`, `templates`.
