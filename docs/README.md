# gopher engineering docs

These docs are for engineers changing gopher itself — its Go code and its
templates. For installing and using the CLI, see the root [README.md](../README.md).

gopher turns a fixed set of Go conventions into templates and exposes them
behind a CLI, so a coding agent instantiates the style instead of imitating it.
It has no dependencies outside the standard library, and it is laid out
hexagonally — the same shape `gopher generate setup` emits, which makes its own
tree a working example of its output.

## The docs

| Doc | What it covers |
|---|---|
| [architecture.md](architecture.md) | Package map, the port catalog, the dependency rule |
| [pipeline.md](pipeline.md) | argv → bytes on disk, the three artifact modes, safety rails |
| [templates.md](templates.md) | Authoring templates: the data contract, funcs, gating, gotchas |
| [adding-a-type.md](adding-a-type.md) | Two worked walkthroughs, plus a checklist |
| [testing.md](testing.md) | Golden workflow, what is really compile-checked, what is not |
| [decisions.md](decisions.md) | Why the codebase is the way it is, and what was rejected |

## First hour

1. Skim the root [README.md](../README.md) and run the tool — `go run ./cmd/gopher list`,
   then `go run ./cmd/gopher generate adapter -name Stripe -stdout`. Seeing the
   output first makes the rest of this concrete.
2. Read [architecture.md](architecture.md). The one idea to take away: **the
   registry is the single source of truth**, and adding a type must never
   require touching the CLI.
3. Read [pipeline.md](pipeline.md) alongside `internal/core/domain/generator.go`.
   It is ~300 lines and is the whole engine.
4. Follow [adding-a-type.md](adding-a-type.md) for a throwaway adapter kind and
   delete it. You will have touched every layer.
5. Keep [decisions.md](decisions.md) for when something looks wrong. Several
   things that look like oversights are deliberate and recorded there.

`CLAUDE.md` at the repo root is the same material compressed into working rules
for coding agents. If you change a constraint here, change it there too.

## Where do I change X?

| I want to… | Touch |
|---|---|
| Add an adapter kind | `templates/files/adapter/<kind>.tmpl` + `AdapterKinds` in `internal/core/domain/registry.go` |
| Add a flag to a type | that type's `GenSpec.Flags` in `internal/core/domain/registry.go` — **not** `internal/adapters/cli.go` |
| Change what a type emits | its `.tmpl` under `templates/files/`, then refresh goldens |
| Add a whole new type | [adding-a-type.md](adding-a-type.md) |
| Add a template function | the func map in `internal/adapters/template.go` |
| Change where templates resolve from | `internal/adapters/store.go` and `Config.TemplateDirs` in `internal/config/config.go` |
| Add a top-level command | `CliAdapter.Run` in `internal/adapters/cli.go` — the one place a new *command* (not type) belongs |
| Change generated-project conventions | the templates, not the Go code. See the faithfulness rule in [templates.md](templates.md) |

## Ground rules

- **Zero dependencies.** The `require` block in `go.mod` stays empty. Third-party
  imports inside `templates/` belong to *generated* code, not to gopher.
- **The core never imports the adapters.** See the dependency rule in
  [architecture.md](architecture.md).
- **Read golden diffs before accepting them.** An unexpected change means a
  template changed behavior.
