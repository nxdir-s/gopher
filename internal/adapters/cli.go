package adapters

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/nxdir-s/gopher/internal/config"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/internal/ports"
)

const (
	ExitOk    int = 0
	ExitError int = 1
	ExitUsage int = 2
)

const (
	GenerateCmd  string = "generate"
	ListCmd      string = "list"
	DescribeCmd  string = "describe"
	TemplatesCmd string = "templates"
	VersionCmd   string = "version"
	HelpCmd      string = "help"

	InitSubCmd string = "init"
	ListSubCmd string = "list"
)

type ErrDuplicateFlag struct {
	name string
}

func (e *ErrDuplicateFlag) Error() string {
	return "flag '" + e.name + "' is reserved by gopher"
}

// globals are the flags every generate command accepts on top of the ones its
// spec declares. They are reported by describe so callers can discover them
var globals = []entity.FlagSpec{
	{Name: "out", Usage: "directory the generated files are written to", Default: "."},
	{Name: "module", Usage: "go module path, defaults to the module in go.mod"},
	{Name: "force", Usage: "overwrite existing files", Type: entity.FlagBool, Default: "false"},
	{Name: "dry-run", Usage: "print the paths that would be written", Type: entity.FlagBool, Default: "false"},
	{Name: "stdout", Usage: "print the generated source instead of writing it", Type: entity.FlagBool, Default: "false"},
}

// verbs describe what a dry run would do to each artifact
var verbs = map[entity.ArtifactStatus]string{
	entity.StatusCreated:   "write",
	entity.StatusAppended:  "append to",
	entity.StatusUnchanged: "leave unchanged",
}

type CliOpt func(a *CliAdapter)

// WithStdout sets where command output is written
func WithStdout(w io.Writer) CliOpt {
	return func(a *CliAdapter) {
		a.stdout = w
	}
}

// WithStderr sets where errors and usage are written
func WithStderr(w io.Writer) CliOpt {
	return func(a *CliAdapter) {
		a.stderr = w
	}
}

type CliAdapter struct {
	generator ports.Generator
	catalog   ports.Catalog
	registry  ports.Registry
	cfg       *config.Config
	stdout    io.Writer
	stderr    io.Writer
	version   string
}

// NewCliAdapter creates the primary adapter that drives the core from argv
func NewCliAdapter(generator ports.Generator, catalog ports.Catalog, registry ports.Registry, cfg *config.Config, version string, opts ...CliOpt) *CliAdapter {
	adapter := &CliAdapter{
		generator: generator,
		catalog:   catalog,
		registry:  registry,
		cfg:       cfg,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		version:   version,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// Run dispatches the supplied arguments and returns the process exit code
func (a *CliAdapter) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.usage()

		return ExitUsage
	}

	switch args[0] {
	case GenerateCmd:
		return a.generate(ctx, args[1:])
	case ListCmd:
		return a.list(args[1:])
	case DescribeCmd:
		return a.describe(args[1:])
	case TemplatesCmd:
		return a.templates(ctx, args[1:])
	case VersionCmd:
		fmt.Fprintf(a.stdout, "gopher %s\n", a.version)

		return ExitOk
	case HelpCmd, "-h", "--help":
		a.usage()

		return ExitOk
	default:
		fmt.Fprintf(a.stderr, "unknown command: %s\n\n", args[0])
		a.usage()

		return ExitUsage
	}
}

// generate parses the flags declared by the requested type and runs the core
func (a *CliAdapter) generate(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(a.stderr, "usage: gopher generate <type> [flags]\n\n")
		a.printTypes(a.stderr)

		return ExitUsage
	}

	spec, code := a.lookup(args[0])
	if spec == nil {
		return code
	}

	set := flag.NewFlagSet("gopher "+GenerateCmd+" "+spec.Type.String(), flag.ContinueOnError)
	set.SetOutput(a.stderr)

	out := set.String("out", a.cfg.OutDir, globals[0].Usage)
	module := set.String("module", a.cfg.Module, globals[1].Usage)
	force := set.Bool("force", false, globals[2].Usage)
	dryRun := set.Bool("dry-run", false, globals[3].Usage)
	stdout := set.Bool("stdout", false, globals[4].Usage)

	strs := make(map[string]*string, len(spec.Flags))
	bools := make(map[string]*bool, len(spec.Flags))
	lists := make(map[string]*listValue, len(spec.Flags))

	for i := range spec.Flags {
		flagSpec := spec.Flags[i]

		if reserved(flagSpec.Name) {
			fmt.Fprintf(a.stderr, "%s\n", (&ErrDuplicateFlag{flagSpec.Name}).Error())

			return ExitError
		}

		value := flagSpec.Default
		if configured, ok := a.cfg.Default(flagSpec.Name); ok {
			value = configured
		}

		switch flagSpec.Type {
		case entity.FlagBool:
			bools[flagSpec.Name] = set.Bool(flagSpec.Name, value == "true", flagSpec.Usage)
		case entity.FlagList:
			list := &listValue{}
			set.Var(list, flagSpec.Name, flagSpec.Usage+" (repeatable)")
			lists[flagSpec.Name] = list
		default:
			strs[flagSpec.Name] = set.String(flagSpec.Name, value, flagSpec.Usage)
		}
	}

	if err := set.Parse(args[1:]); err != nil {
		return ExitUsage
	}

	provided := make(map[string]struct{}, set.NFlag())
	set.Visit(func(f *flag.Flag) {
		provided[f.Name] = struct{}{}
	})

	// the module has to describe the tree being written to, not the directory
	// gopher happens to run from, so an explicit -out re-resolves it
	if _, ok := provided["module"]; !ok {
		if _, redirected := provided["out"]; redirected {
			*module = config.FindModule(*out)
		}
	}

	req := &entity.Request{
		Type:   spec.Type,
		Flags:  make(map[string]string, len(spec.Flags)),
		Lists:  make(map[string][]string, len(lists)),
		OutDir: *out,
		Module: *module,
		Force:  *force,
		DryRun: *dryRun,
		Stdout: *stdout,
	}

	for name, value := range strs {
		req.Flags[name] = *value
	}

	for name, value := range bools {
		req.Flags[name] = strconv.FormatBool(*value)
	}

	for name, value := range lists {
		req.Lists[name] = value.values
	}

	artifacts, err := a.generator.Generate(ctx, req)
	if err != nil {
		fmt.Fprintf(a.stderr, "%s\n", err.Error())

		return ExitError
	}

	a.report(artifacts, req)

	return ExitOk
}

// list prints the registered generation types
func (a *CliAdapter) list(args []string) int {
	set := flag.NewFlagSet("gopher "+ListCmd, flag.ContinueOnError)
	set.SetOutput(a.stderr)

	asJSON := set.Bool("json", false, "print the types as json")

	if err := set.Parse(args); err != nil {
		return ExitUsage
	}

	specs := a.registry.Specs()

	if *asJSON {
		types := make([]listEntry, 0, len(specs))

		for i := range specs {
			types = append(types, listEntry{
				Type:    specs[i].Type.String(),
				Summary: specs[i].Summary,
			})
		}

		return a.writeJSON(types)
	}

	a.printTypes(a.stdout)

	return ExitOk
}

// listEntry is a single row of the list payload
type listEntry struct {
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

// describeOutput is the describe payload, a spec plus the global flags
type describeOutput struct {
	*entity.GenSpec
	Globals []entity.FlagSpec `json:"globals"`
}

// describe prints the flags and templates a single type declares
func (a *CliAdapter) describe(args []string) int {
	name, rest := splitPositional(args)

	if len(name) == 0 {
		fmt.Fprintf(a.stderr, "usage: gopher describe <type> [-json]\n\n")
		a.printTypes(a.stderr)

		return ExitUsage
	}

	set := flag.NewFlagSet("gopher "+DescribeCmd, flag.ContinueOnError)
	set.SetOutput(a.stderr)

	asJSON := set.Bool("json", false, "print the type as json")

	if err := set.Parse(rest); err != nil {
		return ExitUsage
	}

	spec, code := a.lookup(name)
	if spec == nil {
		return code
	}

	if *asJSON {
		return a.writeJSON(describeOutput{spec, globals})
	}

	fmt.Fprintf(a.stdout, "%s - %s\n\n", spec.Type.String(), spec.Summary)

	fmt.Fprintf(a.stdout, "flags:\n")
	a.printFlags(spec.Flags)

	fmt.Fprintf(a.stdout, "\nglobal flags:\n")
	a.printFlags(globals)

	fmt.Fprintf(a.stdout, "\ngenerates:\n")
	for i := range spec.Templates {
		fmt.Fprintf(a.stdout, "  %-34s from %s\n", spec.Templates[i].Out, spec.Templates[i].Name)
	}

	return ExitOk
}

// templates dispatches the template subcommands
func (a *CliAdapter) templates(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(a.stderr, "usage: gopher templates <init|list> [flags]\n")

		return ExitUsage
	}

	switch args[0] {
	case ListSubCmd:
		return a.templatesList(args[1:])
	case InitSubCmd:
		return a.templatesInit(ctx, args[1:])
	default:
		fmt.Fprintf(a.stderr, "unknown templates command: %s\n", args[0])

		return ExitUsage
	}
}

// templatesList prints every template and where it resolves from
func (a *CliAdapter) templatesList(args []string) int {
	set := flag.NewFlagSet("gopher "+TemplatesCmd+" "+ListSubCmd, flag.ContinueOnError)
	set.SetOutput(a.stderr)

	asJSON := set.Bool("json", false, "print the templates as json")

	if err := set.Parse(args); err != nil {
		return ExitUsage
	}

	infos, err := a.catalog.List()
	if err != nil {
		fmt.Fprintf(a.stderr, "%s\n", err.Error())

		return ExitError
	}

	if *asJSON {
		return a.writeJSON(infos)
	}

	for i := range infos {
		fmt.Fprintf(a.stdout, "%-28s %s\n", infos[i].Name, infos[i].Origin)
	}

	return ExitOk
}

// templatesInit exports the embedded templates so they can be edited
func (a *CliAdapter) templatesInit(ctx context.Context, args []string) int {
	set := flag.NewFlagSet("gopher "+TemplatesCmd+" "+InitSubCmd, flag.ContinueOnError)
	set.SetOutput(a.stderr)

	dir := set.String("dir", config.UserTemplateDir(), "directory the templates are written to")
	force := set.Bool("force", false, "overwrite existing templates")
	asJSON := set.Bool("json", false, "print the result as json")

	if err := set.Parse(args); err != nil {
		return ExitUsage
	}

	result, err := a.catalog.Init(ctx, *dir, *force)
	if err != nil {
		fmt.Fprintf(a.stderr, "%s\n", err.Error())

		return ExitError
	}

	if *asJSON {
		return a.writeJSON(result)
	}

	for i := range result.Written {
		fmt.Fprintf(a.stdout, "%s\n", result.Written[i])
	}

	for i := range result.Skipped {
		fmt.Fprintf(a.stdout, "skipped %s, pass -force to overwrite\n", result.Skipped[i])
	}

	return ExitOk
}

// lookup resolves a type name to its spec, reporting usage errors
func (a *CliAdapter) lookup(name string) (*entity.GenSpec, int) {
	genType, err := valobj.ParseGenType(name)
	if err != nil {
		fmt.Fprintf(a.stderr, "%s\n\n", err.Error())
		a.printTypes(a.stderr)

		return nil, ExitUsage
	}

	spec, err := a.registry.Spec(genType)
	if err != nil {
		fmt.Fprintf(a.stderr, "%s\n\n", err.Error())
		a.printTypes(a.stderr)

		return nil, ExitUsage
	}

	return spec, ExitOk
}

// report prints the outcome of a generation
func (a *CliAdapter) report(artifacts []*entity.Artifact, req *entity.Request) {
	if req.Stdout {
		for i := range artifacts {
			if len(artifacts) > 1 {
				fmt.Fprintf(a.stdout, "// %s\n", artifacts[i].Path)
			}

			a.stdout.Write(artifacts[i].Content)
		}

		return
	}

	for i := range artifacts {
		if req.DryRun {
			fmt.Fprintf(a.stdout, "would %s %s\n", verbs[artifacts[i].Status], artifacts[i].Path)

			continue
		}

		if artifacts[i].Status == entity.StatusCreated {
			fmt.Fprintf(a.stdout, "%s\n", artifacts[i].Path)

			continue
		}

		fmt.Fprintf(a.stdout, "%s %s\n", artifacts[i].Status.String(), artifacts[i].Path)
	}
}

// writeJSON encodes the supplied value to stdout
func (a *CliAdapter) writeJSON(value any) int {
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(a.stderr, "failed to encode json: %s\n", err.Error())

		return ExitError
	}

	return ExitOk
}

// printFlags writes a flag table
func (a *CliAdapter) printFlags(flags []entity.FlagSpec) {
	for i := range flags {
		suffix := ""

		switch {
		case flags[i].Required:
			suffix = " (required)"
		case len(flags[i].Default) > 0:
			suffix = " (default " + flags[i].Default + ")"
		}

		fmt.Fprintf(a.stdout, "  -%-16s %s%s\n", flags[i].Name, flags[i].Usage, suffix)
	}
}

// printTypes writes the registered generation types
func (a *CliAdapter) printTypes(w io.Writer) {
	specs := a.registry.Specs()

	fmt.Fprintf(w, "types:\n")

	for i := range specs {
		fmt.Fprintf(w, "  %-10s %s\n", specs[i].Type.String(), specs[i].Summary)
	}
}

// usage writes the top level help
func (a *CliAdapter) usage() {
	fmt.Fprintf(a.stderr, "gopher generates go code, templates, and project scaffolding\n\n")
	fmt.Fprintf(a.stderr, "usage:\n  gopher <command> [arguments]\n\n")
	fmt.Fprintf(a.stderr, "commands:\n")
	fmt.Fprintf(a.stderr, "  %-24s %s\n", GenerateCmd+" <type>", "generate code of the supplied type")
	fmt.Fprintf(a.stderr, "  %-24s %s\n", ListCmd+" [-json]", "list the available types")
	fmt.Fprintf(a.stderr, "  %-24s %s\n", DescribeCmd+" <type> [-json]", "print the flags a type accepts")
	fmt.Fprintf(a.stderr, "  %-24s %s\n", TemplatesCmd+" <init|list>", "inspect and export the templates")
	fmt.Fprintf(a.stderr, "  %-24s %s\n", VersionCmd, "print the gopher version")
	fmt.Fprintf(a.stderr, "  %-24s %s\n\n", HelpCmd, "print this help")
	a.printTypes(a.stderr)
	fmt.Fprintf(a.stderr, "\nrun 'gopher describe <type>' for the flags a type accepts\n")
}

// splitPositional pulls the first non flag argument out of args. The commands
// that use it declare bool flags only, so a value can never be mistaken for it
func splitPositional(args []string) (string, []string) {
	for i := range args {
		if strings.HasPrefix(args[i], "-") {
			continue
		}

		rest := make([]string, 0, len(args)-1)
		rest = append(rest, args[:i]...)
		rest = append(rest, args[i+1:]...)

		return args[i], rest
	}

	return "", args
}

// reserved reports whether a name collides with a global flag
func reserved(name string) bool {
	for i := range globals {
		if globals[i].Name == name {
			return true
		}
	}

	return false
}

// listValue collects a repeatable flag into a slice
type listValue struct {
	values []string
}

// String returns the collected values
func (v *listValue) String() string {
	return strings.Join(v.values, ",")
}

// Set appends a value
func (v *listValue) Set(value string) error {
	v.values = append(v.values, value)

	return nil
}
