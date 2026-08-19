# Writing templates

Templates live in `templates/files/` and are compiled into the binary with
`go:embed` (see `templates/templates.go`). They are plain `text/template`.

## Lookup

A template is referenced by name without the extension, like `adapter/kafka` or
`http/request`. `StoreAdapter.Load` in `internal/adapters/store.go` resolves it
against, in order:

```
1. <project root>/.gopher/templates/<name>.tmpl
2. $XDG_CONFIG_HOME/gopher/templates/<name>.tmpl
3. embedded  templates/files/<name>.tmpl
```

First match wins, per file. A user who overrides `adapter/kafka.tmpl` still gets
the embedded set for everything else. `gopher templates list` prints where each
name currently resolves from; `gopher templates init` exports the embedded set
for editing.

The `files/` prefix exists so `go:embed all:files` can't swallow
`templates.go` itself. `templates.Root` holds it; the store strips it.

## The data contract

Every template renders against one struct, `entity.TemplateData` in
`internal/core/entity/data.go`. This is the contract that makes user overrides
safe to write, so treat changes to it as breaking.

| Field | Type | Comes from |
|---|---|---|
| `Name` | `valobj.Naming` | `-name`, or the last path segment of `-module` if absent |
| `Package` | `string` | `-pkg` |
| `Module` | `string` | `-module`, or the `go.mod` covering the output tree |
| `Kind` | `string` | `-kind` |
| `GoVersion` | `string` | the toolchain gopher was built with, for `go.mod` |
| `Fields` | `[]valobj.Field` | `-field Name:Type[:tag]`, repeatable |
| `Ports` | `[]valobj.Field` | `-port Field:ports.Interface`, repeatable |
| `Methods` | `[]valobj.Method` | `-method 'Save(ctx context.Context) error'`, parsed by `ports.GoSource` |
| `Flags` | `map[string]string` | every non-list flag the spec declares |
| `Lists` | `map[string][]string` | every list flag the spec declares |
| `Tracer` | `bool` | `-tracer` |
| `Logger` | `bool` | `-logger` |

`Flags` and `Lists` are populated for **every flag the spec declares**, so
`.Flags.side` and `range .Lists.method` are always safe. Indexing a key the spec
doesn't declare is a hard error: the renderer sets `missingkey=error`, which
turns a typo into a failed command rather than a silent `<no value>`.

### Naming

`valobj.Naming` (`internal/core/valobj/naming.go`) splits the input on
separators, case transitions, and acronym boundaries, then derives:

`.Pascal` `.Camel` `.Snake` `.Kebab` `.Lower` `.Upper` `.Plural` `.Words`

```
"payment gateway" → PaymentGateway  paymentGateway  payment_gateway  payment-gateway
"HTTPCache"       → HTTPCache       httpCache       http_cache
"user_id"         → UserID          userID          user_id
```

`user_id → UserID` rather than `UserId` comes from the initialisms switch in
`capitalize` at the bottom of that file. Add a case when a new one bites;
`naming_test.go` is a table test and cheap to extend.

`.Words` (`"order repository"`) exists for doc comments, where `.Snake` and
`.Lower` both read badly.

### Methods

`-method` strings are parsed into structured form by
`GoSourceAdapter.Methods`: it wraps them in a synthetic interface, parses that
with `go/parser`, and returns `Name`, `Params`, `Results`, `Args`, and
`HasResults`. `Args` is the forwarding call, so `mocks/fake.tmpl` can write:

```
func (f *Fake{{$.Name.Pascal}}) {{.Name.Pascal}}({{.Params}}) {{.Results}} {
	{{if .HasResults}}return {{end}}f.{{.Name.Pascal}}Func({{.Args}})
}
```

Unnamed parameters are given generated names (`arg1`, `arg2`) so they can be
forwarded, and variadic parameters get `...` appended in `Args`.

## Functions

The func map is small on purpose (`internal/adapters/template.go`):

```
pascal  camel  snake  plural      via valobj.Naming
lower   upper  contains  join     strings
```

## Gating patterns

**Conditional file.** An `Out` that renders empty drops the artifact:

```go
{Name: "setup/gomod", Out: `{{if eq .Flags.gomod "true"}}go.mod{{end}}`},
```

**Conditional block.** The usual `{{if}}`, seen throughout
`templates/files/adapter/generic.tmpl` for the tracer and logger toggles.

**Scan-then-decide.** Template variables are assignable, so a template can walk
its inputs before emitting anything. `templates/files/port/interface.tmpl` uses
this to import `context` only when a method signature mentions it:

```
{{- $needsContext := false -}}
{{- range .Lists.method}}{{if contains . "context.Context"}}{{$needsContext = true}}{{end}}{{end -}}
```

## Whitespace control

gofmt fixes indentation but **not** blank lines, so vertical spacing is the
template's job. The idiom, from `adapter/generic.tmpl`:

```
	if a.client == nil {
{{- if .Logger}}
		a.logger.Error("nil client in {{.Name.Pascal}}Adapter")
{{end}}
		return &ErrNil{{.Name.Pascal}}Client{}
	}
```

`{{- if}}` eats the newline after `{`, and the *undashed* `{{end}}` contributes
its own trailing newline. With the logger on you get a blank line before
`return`, matching house style; with it off you get no stray blank line. Getting
this wrong produces output that compiles and looks subtly wrong, which the
golden files will catch.

Empty composite literals need the same care. gofmt leaves `struct {\n}` alone,
so branch on it:

```
{{- if or .Ports .Logger .Tracer}}
type {{.Name.Pascal}} struct {
	...
}
{{- else}}
type {{.Name.Pascal}} struct{}
{{- end}}
```

## Imports are the template's job

`go/format` formats and syntax-checks. It does **not** add or remove imports.
That would need `golang.org/x/tools/imports`, which would be gopher's first
third-party dependency (see [decisions.md](decisions.md)).

So every template carries a complete import block, and any generator whose
output can reference arbitrary types exposes a repeatable `-import` flag:
`entity`, `valobj`, `domain`, `port`, and `mocks`. A field typed `time.Time`
needs `-import time`, and gopher won't infer it.

The one place imports are computed rather than declared is append mode:
`GoSourceAdapter.Merge` unions the two files' import blocks and re-emits them
with stdlib grouped ahead of everything else.

## The faithfulness rule

The adapter templates under `templates/files/adapter/` are near-verbatim
transcriptions of the reference implementations in `spec.md`, with only the type
name and package parameterized. **They are the style contract.** Changing logic
inside one changes every project generated from it, so treat it as a deliberate
decision rather than a cleanup.

Two things in there look like bugs and are deliberate:

- `postgres.tmpl` keeps `strings.ToLower(keyword) == strings.ToLower(stmt[:len(keyword)])`
  rather than `strings.EqualFold`. It matches the source in `spec.md`.
- That same `validStatement` panics when `sql` is shorter than the keyword. The
  behavior is inherited from the reference implementation, not introduced here.

The non-adapter templates (`core/`, `valobj/`, `port/`, `setup/`, `server/`,
`mocks/`, `test/`, `module/`, `http/`, `infra/`) have no upstream source and are
ordinary code. Improve them freely, then refresh the goldens.

## After editing any template

```bash
GOPHER_UPDATE_GOLDEN=1 go test ./...   # then READ the diff
go test ./...
```

See [testing.md](testing.md) for what the goldens do and do not prove.
