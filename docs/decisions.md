# Decisions

Why the codebase is the way it is, and what was rejected. Roughly chronological.

Read this before "fixing" something that looks wrong — several of these were
deliberate and are easy to undo by accident.

---

## Zero dependencies

`go.mod` has an empty `require` block, and it stays that way. Everything runs on
`text/template`, `go/format`, `go/parser`, `embed`, `encoding/json`, `log/slog`,
and `flag`.

gopher is a developer tool installed with `go install` and invoked by an agent in
a loop. A single self-contained binary with nothing to resolve is worth real
constraints.

**Rejected:** `urfave/cli` and `spf13/cobra` for the CLI, `go-toml/v2` for
config.

**Consequences:** config is JSON rather than TOML, despite `spec.md` showing a
`TomlAdapter` — that adapter is a *template*, and templates' dependencies belong
to generated code, not to gopher. Also, no `goimports`; see below.

## stdlib `flag` with hand-rolled dispatch

`CliAdapter.Run` switches on `args[0]` and each subcommand builds its own
`flag.NewFlagSet`.

**Consequence, and it bit once:** stdlib `flag` stops parsing at the first
non-flag argument. `gopher describe adapter -json` silently ignored `-json`,
because `adapter` terminated parsing. Fixed by `splitPositional` in
`internal/adapters/cli.go`, which lifts the positional out before parsing.
`generate` does not use it — its flags take values, so a value could be mistaken
for the type name; it takes the type from `args[0]` strictly.

## The registry is the single source of truth

One `entity.GenSpec` per type in `internal/core/domain/registry.go`, driving CLI
flag registration, `describe` output, and template selection.

The alternative — a `switch` in the CLI per type — means adding a type touches
argument parsing, help text, and generation separately, and they drift. The rule
that falls out: **adding a type must never require editing `cli.go`.**

## Embedded defaults plus a three-tier override chain

`go:embed` defaults so the binary works with zero setup, overridable per project
then per user (see [templates.md](templates.md)).

`spec.md` asked for user-configurable templates. Overrides are matched per file
by name, so replacing one template inherits the rest — a full-copy model would
mean a user's fork silently misses every later improvement.

## gopher is laid out like its own output

The repo is hexagonal because `gopher generate setup` emits that shape. Its own
tree is a live example, and the layout is exercised every time the tool is
built.

**Consequence:** `internal/core` must not import `internal/adapters`, verified
with `go list` (see [architecture.md](architecture.md)).

## The adapter templates are faithful transcriptions

`templates/files/adapter/*.tmpl` are near-verbatim copies of the reference
implementations in `spec.md`, with only the type name and package
parameterized.

While transcribing `postgres.tmpl` I "improved" `validStatement` from
`strings.ToLower(a) == strings.ToLower(b)` to `strings.EqualFold` and then
reverted it. The whole point of the tool is that its output matches what the
author would have written; a generator that quietly rewrites your idioms is the
problem it exists to solve. The same reasoning leaves that function's
short-input panic in place — inherited, not introduced.

This applies **only** to `adapter/`. Every other template directory has no
upstream source and is ordinary code.

## `go/format` as validator, no import fixing

Rendered `.go` output goes through `go/format.Source`. A template producing
source that does not parse fails the command instead of writing garbage, and the
error carries the template name and offending line.

`go/format` does not add or remove imports. `golang.org/x/tools/imports` would,
and it would be gopher's first dependency. Instead templates carry complete
import blocks, and generators expose `-import` where output can reference
arbitrary types.

**This is the standing escape hatch:** if import inference ever becomes
necessary, `x/tools` is the answer and the zero-dependency property is the
price. Make that call deliberately.

## Goldens refresh via env var, not `-update`

`GOPHER_UPDATE_GOLDEN=1 go test ./...`.

The conventional `-update` flag fails in every package that does not define it,
so `go test ./... -update` cannot do a repo-wide refresh — which is exactly what
you want after a template change.

## `ModeAppend` for ports

The ports module is three shared files, so `gopher generate port` must add an
interface to an existing file rather than create one. `GoSourceAdapter.Merge`
parses both sides, unions the imports with stdlib grouped first, and appends.

**Rejected:** one file per port. It would have sidestepped all the AST work, but
`spec.md` is explicit that ports live in `core.go`/`primary.go`/`secondary.go`.

**Rejected:** string-matching `type X interface {` to detect duplicates. Parsing
is barely more code and is not fooled by comments or formatting.

## `ModeEnsure` for the http companions

The http adapter is written against project-local types (`entity.Request`,
`valobj.Method`, …) that `spec.md` never defines, so `-kind http` also emits
them.

They sit at fixed paths independent of `-name`, which rules out both existing
modes. `ModeCreate` would fail the no-clobber check on a second http adapter.
`ModeAppend` is worse: its duplicate check keys on `data.Name.Pascal` — the
*adapter's* name, not `Request` — so it would never match and would append
duplicate types on every run.

`ModeEnsure` is the right primitive for scaffolding the caller then edits: write
once, never touch again, `unchanged` thereafter. `-force` still regenerates.

## Empty `Out` means skip

Conditional files are expressed by letting `TemplateRef.Out` render to an empty
string:

```go
{Name: "setup/gomod", Out: `{{if eq .Flags.gomod "true"}}go.mod{{end}}`},
```

**Rejected:** a `When` or `Condition` field on `TemplateRef`. `Out` is already a
template, so the capability was free — a new field would have been a second way
to express the same thing.

## Module resolution follows `-out`

If `-out` is passed explicitly and `-module` is not, the module is re-resolved
from the output tree via `config.FindModule`.

Found while verifying the scaffold: generating into `/tmp/demo` from inside the
gopher repo stamped `github.com/nxdir-s/gopher` into the generated imports. The
module must describe the tree being written to, not the shell's location.

## No `service` type

`spec.md` lists `service`, but the hexagonal layout has no service layer —
domains are already the orchestrators. A second concept for the same role would
only invite drift, so `GenService` was removed rather than left as an unused
enum value.

## `infra` is AWS CDK in Go

`spec.md` lists `infra / cdk` with no detail. Chosen: a CDK app in Go, written to
`infra/` as its own module.

The nested `go.mod` means the parent module's `go build ./...` skips it, which is
what you want — the CDK dependency tree stays out of the application's.

## Formatting is skipped for non-Go output

`render` only formats when the output path ends in `.go`, so `Makefile`,
`go.mod`, `cdk.json`, and `.gitignore` pass through untouched. Obvious in
hindsight; worth stating because the check is a single `strings.HasSuffix` that
would be easy to drop.

## No package-level variables

The `require` block is not the only thing kept empty — so is the set of package
level `var`s. Two remain, both mandated:

- `templates.FS`, because `//go:embed` can only target a package-level variable
- `main.Version`, because `-ldflags -X` can only write to one

Each is passed into a constructor at its single call site, so nothing reads them
ambiently.

Everything else became a function. The lookup tables backing `String()` and
`ParseGenType` are `switch` statements, since a value type's method has nowhere
to inject a table. `AdapterKinds` and friends return a fresh slice per call
rather than exposing a mutable exported one. `specs` became `defaultSpecs()`, so
each `Registry` owns its own `*entity.GenSpec` pointers — previously every
registry shared one backing array, and a write through `Registry.Spec()` would
have leaked into every other registry in the process.

**Rejected:** leaving the read-only tables alone because "they are never
written." True today, but the guarantee costs nothing to make structural, and a
`switch` is faster and allocates nothing.

The `cli.go` globals were the concrete motivation. The global flag table was read
by position — `globals[0].Usage` beside a hardcoded `"out"` — so reordering the
slice would have silently attached the wrong help text to a flag. It is now a
`CliAdapter` field keyed by name through `usageFor`, with the flag names as
constants, and `cli_test.go` asserts each flag is followed by its own usage.

## Strings that carry meaning are constants

A string is named when it crosses a boundary the compiler does not check. Four
of those existed:

- **Flag names.** `registry.go` declares `FlagSpec{Name: ...}`; `templateData`
  in `generator.go` reads eight of them back out of the materialized maps. Two
  files, one contract, no link. They now share
  [flags.go](../internal/core/domain/flags.go).
- **Kinds and sides.** A kind flag's `Default` has to be a member of the list
  the same flag advertises. `TestDefaultKindsAreAdvertised` now proves it, using
  `entity.GenSpec.Flag` to read the real default rather than trusting the
  constant.
- **`"true"` / `"false"`.** A bool flag travels as a string so `Flags` can stay
  one map, which made the spelling a protocol between five sites. It is
  `entity.BoolTrue` / `entity.BoolFalse`, declared next to `Request.Bool`.
- **CLI flag names.** `cli.go` had a const block for the global flags and then
  spelled `"json"` out four more times for the inspection commands.

**Deliberately left as literals:**

- **Anything inside a template string.** `` Out: `{{if eq .Flags.gomod "true"}}go.mod{{end}}` ``
  is `text/template` source, not a Go value. Substituting a constant means
  concatenation, which costs readability and buys nothing — the template engine
  never sees the Go identifier.
- **Single-use data in the spec table.** `Out:` paths, template names, package
  defaults, usage prose. A constant referenced once is indirection, not safety;
  the declaration reads better spelled out.
- **Format strings, error fragments, separators.** `"%s\n"` is clearer than a
  name for it.

**Rejected:** distinct named types (`type FlagName string`). It would catch a
kind passed where a flag name belongs, but `FlagSpec.Name` and the `Flags` /
`Lists` map keys would all have to change type, pushing a naming concern into
the template data contract. The explicit `const X string = "..."` form matches
the rest of the codebase and was enough.
