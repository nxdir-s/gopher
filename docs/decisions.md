# Decisions

Why the codebase is the way it is, and what was rejected. Roughly chronological.

Read this before "fixing" something that looks wrong — several of these were
deliberate and are easy to undo by accident.

---

## Zero dependencies, one first-party exception

`go.mod` requires exactly one module — `github.com/nxdir-s/pipelines`, the
first-party fan-out/fan-in helpers behind concurrent rendering — and nothing
else, ever. It is MIT, stdlib-only, and has no dependencies of its own, so the
transitive closure of gopher is still gopher. Everything else runs on
`text/template`, `go/format`, `go/parser`, `embed`, `encoding/json`,
`log/slog`, and `flag`.

gopher is a developer tool installed with `go install` and invoked by an agent in
a loop. A single self-contained binary with nothing external to resolve is worth
real constraints, and a first-party leaf module preserves that property.

**Rejected:** `urfave/cli` and `spf13/cobra` for the CLI, `go-toml/v2` for
config — third-party trees with real transitive closures. Also rejected:
copying the fan-out/fan-in shapes into gopher to keep the `require` block
empty. They are maintained and tested once in `pipelines`; duplicating them
here trades one require line for silent drift between the copies.

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

Like the dependency list, the set of package-level `var`s is kept as close to
empty as the toolchain allows. Two remain, both mandated:

- `templates.FS`, because `//go:embed` can only target a package-level variable
- `main.Version`, because `-ldflags -X` can only write to one

Each is passed into a constructor at its single call site, so nothing reads them
ambiently.

Everything else became a function. The lookup tables backing `String()` and
`ParseGenType` are `switch` statements, since a value type's method has nowhere
to inject a table. `AdapterKinds` and friends return a fresh slice per call
rather than exposing a mutable exported one. `specs` became per-type
constructors behind `specFor`, so each `Registry` builds its own
`*entity.GenSpec` pointers — previously every registry shared one backing
array, and a write through `Registry.Spec()` would have leaked into every other
registry in the process.

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

## Source with no action skips the parser

`TemplateAdapter.Render` returns its input when the source holds no `{{`,
without building a `text/template` at all.

A `TemplateRef` carries its name and its output path as templates, so `render`
makes three renderer calls per ref and only the third is a template file. Nearly
all of the other two are literal strings — 28 of the registry's 31 `Name` values
contain no action, and `setup` alone renders 14 static names and 9 static paths.
Each was paying a `template.New`, a `Funcs` copy of the whole func map with a
`reflect.ValueOf` per entry, an `Option` string parse, a copy of the source, and
a full lex and parse, to hand back a constant. That is 21 ns instead of 2.0 µs
per call, and a quarter of what `gopher generate setup` cost.

It is safe because the renderer never calls `Delims`, so `{{` is the only
sequence that can open an action, `}}` outside one is literal text, and
`missingkey=error` cannot fire where there are no keys. `BenchmarkTemplateRender`
carries a `static` case so the two paths stay visible against each other.

**Not a template cache.** Caching parsed templates is the obvious next idea and
it is rejected in [testing.md](testing.md): within one invocation every template
string is distinct, so the hit rate is ~0% in the run a user gets and ~100% in a
benchmark that reuses a generator. This is the opposite shape — it removes work
rather than remembering it, so it shows up in `BenchmarkGenerateCold`.

## Override directories are resolved once per process

`NewStoreAdapter` stats each override directory and drops the ones that are
absent, instead of `Load` rediscovering them on every lookup.

`Config.TemplateDirs` always appends the user template directory whether or not
it exists, so the usual configuration carries two directories and no overrides.
Every `Load` was opening a path under each of them and taking `ENOENT`:
`gopher generate setup` spent 28 failed reads finding out the same thing 14
times. The lookup went from 3.3 µs to 231 ns, which is what the embedded read
alone costs.

Only directories that are definitively missing are dropped, so a path that
exists but is not a directory still fails at the first `Load` exactly as before.
Deciding existence once assumes a directory does not appear mid run, which holds
because gopher is one short lived process per invocation. `walkDir` already
guarded itself this way; this extends the same assumption to the read path.

`FsAdapter.Write` skips `MkdirAll` for a directory it already created, on the
same reasoning and with the same limit: the state is per adapter and is not safe
to share across goroutines.

**Rejected:** growing the render buffer to the source length up front. It is
worth 6% on the largest template in isolation and nothing end to end — 1.5% on
`BenchmarkGenerateCold/setup`, inside the noise floor — because formatting, not
execution, is what a generate spends its time on. It also over-allocates for
small templates whose output is shorter than their source.

## Create refs render concurrently

`go/format` is roughly 80% of what a generate spends its CPU on and runs once
per `.go` artifact, independently per file. `renderAll` in
`internal/core/domain/generator.go` therefore fans create-mode refs across
`min(MaxRenderFan, GOMAXPROCS, refs)` workers with `pipelines.FanOutBuffer`
and places results back by ref index. The rules that keep it observably
identical to the serial loop it replaced:

- **Append and ensure refs stay on the calling goroutine.** They read and
  merge files on disk, and `fake.Writer` and `FsAdapter`'s `MkdirAll` memo are
  not goroutine-safe on purpose.
- **Fewer than two create refs takes the serial path.** `entity`, every
  `adapter` kind, and `port` stay byte-for-byte on the old loop, so the floor
  cases pay no goroutine overhead — and a single-file `adapter` generate could
  not benefit anyway, since one `format.Source` over one big file is its whole
  cost and cannot be split.
- **The lowest-index error wins.** The serial loop stopped at the first
  failing ref, so scanning the collected errors in declaration order
  reproduces the same error no matter which worker finished first. Refs after
  a failing one now render before the abort; create-ref rendering has no side
  effects, so nothing observes the difference.
- **No send may ever block.** The workers' send sits in a `select` `default`
  branch, so a blocked send would blind a worker to cancellation. Buffers are
  sized to hold every result, which also keeps the fan draining while the
  calling goroutine handles the serial refs.
- **A canceled context surfaces as its error.** Cancellation can stop the
  stream before every ref is handed out, and a ref that never rendered must
  not be mistaken for one a flag switched off. `renderAll` counts deliveries
  and returns `ctx.Err()` on a shortfall.

**Rejected:** fanning the write loop as well — the report order is part of the
interface, the `MkdirAll` memo is a plain map, and writes are a fraction of
render cost.

**Rejected, measured:** a fan as wide as the machine. `min(GOMAXPROCS, refs)`
— ten workers on an M2 Pro — made `BenchmarkGenerateCold/setup` 9.7% *slower*
than the serial loop (213µs → 234µs), and its profile was 81% scheduler
wakeups (`usleep`, `pthread_cond_wait/signal`, `stealWork`) at 347% CPU. The
refs are 5–70µs of work each; waking a parked thread costs about as much as
the ref it comes to steal. A fan's wall time is `max(total/workers, biggest
ref)`, and on the widest spec the biggest ref is about a third of the total,
so `MaxRenderFan = 3` reaches that bound: 159µs against 174µs at two workers,
172µs at four, and 234µs at ten.

## Specs are built one type at a time

A real invocation resolves exactly one spec, but every process built all
eleven — about 5.6KB and a fifth of `BenchmarkGenerateCold/entity`'s
allocations spent on specs nobody asked for. `Registry.Spec` now builds the
requested type through `specFor` on first use and caches the pointer per
registry; `Specs()` materializes the full table in `genTypes` order through
the same cache, so `list` and `describe` see the same order and the same
pointers as before. `WithSpecs` bypasses laziness entirely, which keeps every
fake-spec test on the old resolve-against-a-slice path. The registry stays
single-goroutine — rendering fans out, spec resolution never does — so the
cache needs no lock.

The four `-kind`/`-side` usage strings that were joined from the kind lists on
every construction are now compile-time consts, with
`TestKindUsageStringsMatchKinds` pinning each to its list so a new kind cannot
be advertised in one place and not the other.

**Cost accepted:** adding a type is now three touchpoints in `registry.go` —
constructor, `specFor` case, `genTypes` entry — instead of one slice entry.
`TestSpecsCanonicalOrder` fails loudly when they drift.

## Commands wire only what they use

`run()` used to build the entire hexagon — config load and go.mod read, three
store stats, the renderer's FuncMap, generator, catalog — before looking at
`args[0]`, so `gopher version` paid for all of it and a malformed config file
broke every command including `help`. Dispatch now happens first: `version`,
`list`, `describe`, `help`, and usage errors get a cli wired with only the
registry (nil generator, catalog, and config), `templates` adds the store and
catalog, and only `generate` builds everything.
`TestRunFastPathsNeedOnlyRegistry` drives every fast path against those nils,
so a future edit that reaches for one panics in the test, not in a terminal.
Deliberate behavior change riding along: a broken config no longer fails
`version`/`list`/`describe`/`help` — they never read it.

Smaller cuts in the same pass: `parseModule` walks the go.mod bytes instead of
copying the file into a string; the user config dir is resolved once per
`Load` instead of once per caller; the generate flag values land in the
request through one binding slice instead of three pointer maps materialized
twice; and `report` buffers a multi-artifact run into one write syscall.
Buffering unconditionally was tried first and backed out: a single-artifact
generate — the common case — paid a 4KB buffer (+18% on
`BenchmarkCliRun/generate_full`) to batch one line, so the buffer is gated on
having more than one artifact to print.

## The append path parses the existing file once

`generate port` against an existing ports file used to parse Go source four
times: `Declares(existing)`, then `Merge(existing, new)` re-parsing the same
bytes, plus the rendered source and the final `format.Source`. The declares
scan now rides inside `Merge(dst, src, name)` on the one parse of `dst`, and
the port's contract reports `declared` back instead of being asked twice.
`Declares` survives as an exported adapter method so its benchmark keeps
measuring the scan.

In the same change the append and ensure paths dropped their `Exists` calls:
existence is resolved by the `Read` they were about to do anyway, with a
missing file reported as `fs.ErrNotExist` through the `ports.FileWriter`
contract (`ErrReadFile` and the fake's error both unwrap to it). Deliberate
behavior fix riding along: `Exists` treated *any* stat failure as "absent", so
an existing but unreadable file was routed down the create path and clobbered;
read-first makes it the hard error it should have been.

## Rejected: string-typed template ports

`embed.FS.ReadFile` copies the template body, and `string(tmpl)` at the parse
site copies it again, so retyping `TemplateSource.Load` and `Renderer.Render`
to `string` looks like a free copy removed. Analyzed and rejected on paper:
it saves one copy on the parse path only (~1–2µs of the 780µs aws generate,
2–3% of alloc space), while the override path still copies (`os.ReadFile` to
string), the action-free fast path still copies into `Artifact.Content`, and
execution buffer growth — the actual allocation cost of rendering — is
untouched. Inside the noise floor on every Cold label, priced at a signature
ripple across two ports, the store, the renderer, the fakes, and every
adapter test.

## Rejected: a FuncMap prototype cloned per render

The idea: `Funcs(a.funcs)` re-registers eight functions through
`reflect.ValueOf` on every parsed template, so build one prototype template in
the constructor and `Clone()` it per render instead. Measured on
`BenchmarkTemplateRender` with `-count 10`: the small ref templates gained
3–4.7%, the large bodies were flat to 1.8% *worse*, and every parsing label
gained an allocation (+1.5% B/op on the refs) — `Clone` copies both func maps
and its bookkeeping costs more than the reflection it avoids. Under the 10%
pre-gate it never reached a full-suite run. Closed; the shared-prototype
variant without Clone is off the table permanently because parsing into a
group that is concurrently executing races, and the fan in `renderAll` does
exactly that.

## Ref render failures carry the field, not a synthetic name

`render` used to prefix a ref's name and output-path templates with `name:` /
`out:` so their errors stayed distinguishable, paying two string concats per
ref on the happy path for labels only ever read inside an error message.
`ErrRenderRef` now wraps those failures with the field name, built only in the
error branch. Error text changed shape: what read
`failed to parse template 'out:{{...}}'` now reads
`template ref out: failed to parse template '{{...}}'`.

## Debug attrs are built only when debug is on

`slog` checks the level inside `Logger.Debug`, after the caller has already
built the attrs. The per-artifact write log and the catalog's per-template
export log now sit behind `logger.Enabled`, which costs a branch and saves
three attrs and a variadic slice per artifact at the default warn level.
