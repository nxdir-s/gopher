# Performance

Where a `gopher` invocation spends its time, how changes to that are measured,
and a history of the optimization rounds with their reasoning and numbers.
This doc narrates; the permanent rationale for each individual decision lives
in [decisions](decisions.md), and the benchmark suite that produces every
number here is described in [testing](testing.md).

## Where the time goes

One fact organizes everything else: **`go/format.Source` is roughly 80% of the
CPU a generate spends**, run once per `.go` artifact. Formatting at runtime
isn't optional overhead. It's the safety net that keeps user override
templates and interpolation-dependent layout (gofmt alignment shifts with name
lengths) producing clean output. So the levers are about *when and how wide*
it runs, never about skipping it.

That fact splits the workloads:

- **Single-file generates sit at a floor.** `adapter -kind aws` is one ~12.6KB
  file whose single `format.Source` call is ~745µs of its ~790µs total. One
  parse-and-print can't be split, so `BenchmarkGenerateCold/adapter_aws`
  moves only with the Go toolchain. Treat a flat number there as correct, not
  as a missed opportunity.
- **Multi-file generates parallelize.** `setup` is 14 refs and 10 formats;
  `renderAll` in `internal/core/domain/generator.go` fans create-mode refs
  across `min(MaxRenderFan, GOMAXPROCS, refs)` workers. The wall time of a fan
  is `max(total/workers, biggest ref)`, and on the widest spec the biggest ref
  is about a third of the total. That's why `MaxRenderFan` is 3 and not the
  machine width (see the history below for what the machine width did).

Around the render loop, the costs are: template parsing (~20% of render, paid
per render because every template string in one invocation is distinct; that's
the documented reason there is no parse cache), the registry building specs,
the config load's upward walk and file reads, and the process itself.

On startup: spawning the binary costs ~3.2ms before `main` runs
(`BenchmarkStartup/version` minus `BenchmarkRun/version`), which dwarfs
everything gopher controls. That isn't a reason to stop measuring. It's the
reason the benchmarks, not wall-clock feel, are the scoreboard.
`BenchmarkGenerateCold` is the metric of record: an optimization is only real
if it moves it, and a change elsewhere answers to its own benchmark family but
reports Cold anyway.

## How to measure

The mechanics (`make bench`, benchstat, `-count 10`, the ~5% noise floor) are
in [testing](testing.md). Round 2 added two protocol lessons worth keeping:

- **Thermal state contaminates whole runs.** A `-count 10` full-suite run
  started on a hot machine reads 3–6% high *across the board*, including
  benchmarks whose code didn't change. Let the machine sit idle a few minutes
  before a measurement run, and treat a surprising cross-run delta on
  untouched code as suspect until a targeted A/B on the same machine state
  confirms it. Two labels drifting together is a thermometer, not a finding.
- **Pre-gate candidates with targeted A/Bs.** A full-suite `-count 10` run
  costs ~15 minutes; a targeted run of the one benchmark family a candidate
  can move costs two. Measure before/after back-to-back on the same machine
  state, and only spend the full run on candidates that survive. The FuncMap
  prototype candidate in round 2 died at its pre-gate without ever costing a
  full run.

## History

### Round 2: July 2026

Driven by CPU and allocation profiles of `BenchmarkGenerateCold` after the
benchmark suite landed. The profile said: formatting ~79% of render CPU,
template parsing ~20%, registry construction 12% of allocation space,
`text/template` func-map setup ~10%. Round 1 (the commit that introduced the
static-template fast path, override-dir pruning, and friends) had already
taken the local fruit, so this round was structural.

**Concurrent rendering.** Create-mode refs render and format independently,
so `renderAll` fans them out via `github.com/nxdir-s/pipelines`, the
first-party, stdlib-only module that is the one entry in the `require` block
(the zero-dependency rule was amended for it; rationale in
[decisions](decisions.md)). Behavior is pinned identical to the serial loop:
results land by ref index so artifact order and goldens can't change, the
error reported is the lowest-index failure the serial loop would have hit,
append/ensure refs and the write loop stay on the calling goroutine, and specs
with fewer than two create refs never leave the serial path.

The width was the lesson. The first cut fanned as wide as the machine
(`min(GOMAXPROCS, refs)`, ten workers on the dev M2 Pro) and made
`GenerateCold/setup` **9.7% slower**: its profile was 81% scheduler wakeups
(`usleep`, `pthread_cond_wait/signal`, `stealWork`) at 347% CPU, because
waking a parked thread costs about as much as the 5–70µs ref it comes to
steal. A width sweep found the `max(total/workers, biggest ref)` bound is
reached at three workers: 159µs, against 174µs at two, 172µs at four, 234µs at
ten, 213µs serial. Hence `MaxRenderFan = 3`.

**Lazy registry.** A real invocation resolves one spec, but every process
built all eleven (~5.6KB, a fifth of `Cold/entity`'s allocations).
`Registry.Spec` now builds only the requested type through `specFor` and
caches the pointer per registry; `Specs()` materializes the full table for
`list`/`describe` through the same cache, preserving order and pointer
identity. The four `-kind`/`-side` usage strings joined on every construction
became compile-time consts with a drift-guard test. Cost accepted: adding a
type is three touchpoints in `internal/core/domain/registry.go` instead of one
slice entry, policed by `TestSpecsCanonicalOrder`.

**One parse on the append path.** `generate port` parsed the existing ports
file twice: `Declares`, then `Merge` on the same bytes. The declares scan now
rides inside `Merge(dst, src, name)` on its one parse of `dst`. In the same
pass, append and ensure resolve existence by the `Read` they were about to do
anyway (missing files satisfy `errors.Is(err, fs.ErrNotExist)` per the
`ports.FileWriter` contract), which removed a stat per ref and fixed a real
bug: an existing but unreadable file used to be routed down the create path
and clobbered; now it's a hard error.

**Commands wire only what they use.** `run()` used to build the entire
hexagon (config load, go.mod read, store stats, FuncMap, generator, catalog)
before reading `args[0]`. Dispatch now happens first: `version`, `list`,
`describe`, and `help` get a cli wired with only the registry, `templates`
adds the store and catalog, and only `generate` builds everything.
`TestRunFastPathsNeedOnlyRegistry` pins the nil-dependency contract. Behavior
change riding along, deliberately: a malformed config file no longer breaks
the commands that never read it.

**Hygiene.** Per-artifact debug attrs are built only when debug logging is on;
repeated `string()` conversions in the render loop were hoisted; the
`name:`/`out:` error-label concatenations moved into an error type built only
on failure (error text changed shape); a stray-`./config.json` read when no
user config dir resolves was fixed; multi-artifact reports buffer into one
write syscall.

Measured, cumulative (`benchstat` old vs new, `-count 10`, M2 Pro, go1.26.4;
indicative, since no baseline is committed):

| Benchmark | Before | After | Delta |
|---|---|---|---|
| `GenerateCold/setup` | 224.4µs | 167.4µs | −25.4% |
| `GenerateCold/entity` | 21.5µs | 20.8µs | −3.6% time, −19.9% B/op, −9.8% allocs |
| `GenerateCold/adapter_aws` | 772.8µs | 789.1µs | ~flat (the format floor) |
| `Generate/setup` | 213.2µs | 162.0µs | −24.1% |
| `Generate/infra` | 100.1µs | 86.6µs | −13.5% |
| `GenerateAppend/merge` | 57.2µs | 51.9µs | −9.4% |
| `Run/version` | 88.4µs | 617ns | −99.3% |
| `Run/list` | 96.4µs | 8.6µs | −91.0% |
| `Startup/version` | 3.40ms | 3.17ms | −6.6% |
| `RegistrySpec` | 4.8ns | 3.2ns | −31.8% |

**Rejected, with the evidence recorded in [decisions](decisions.md):** the
machine-wide fan (slower, 81%-wakeup profile); a FuncMap prototype cloned per
render (died at its pre-gate: `Clone`'s bookkeeping cost more than the
reflection it saved); buffering single-artifact reports (+18% on
`BenchmarkCliRun/generate_full` to batch one line; the buffer is gated on
more than one artifact); string-typed template ports (rejected on paper: one
copy saved on one path, inside the floor, priced at a two-port signature
ripple).

Verified: goldens byte-identical, the fanned path race-clean, and generated
output diff-verified byte-for-byte against a binary built from the pre-round
tree across `setup`, `adapter` (aws and http with companions), `entity`, and a
double `port` append.
