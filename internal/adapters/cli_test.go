package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nxdir-s/gopher/internal/config"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

type ErrNoTestSpec struct {
	genType valobj.GenType
}

func (e *ErrNoTestSpec) Error() string {
	return "no spec registered for type: " + e.genType.String()
}

// testRegistry is an in memory ports.Registry. The cli is tested against a spec
// set of its own so the assertions do not move when the real registry does
type testRegistry struct {
	specs []*entity.GenSpec
}

func (r *testRegistry) Spec(genType valobj.GenType) (*entity.GenSpec, error) {
	for i := range r.specs {
		if r.specs[i].Type == genType {
			return r.specs[i], nil
		}
	}

	return nil, &ErrNoTestSpec{genType}
}

func (r *testRegistry) Specs() []*entity.GenSpec {
	return r.specs
}

// testGenerator is an in memory ports.Generator that records the request it was
// handed and replays the artifacts it was seeded with
type testGenerator struct {
	artifacts []*entity.Artifact
	err       error
	req       *entity.Request
}

func (g *testGenerator) Generate(ctx context.Context, req *entity.Request) ([]*entity.Artifact, error) {
	g.req = req

	if g.err != nil {
		return nil, g.err
	}

	return g.artifacts, nil
}

// testCatalog is an in memory ports.Catalog
type testCatalog struct {
	infos  []*entity.TemplateInfo
	result *entity.InitResult
	err    error
}

func (c *testCatalog) List() ([]*entity.TemplateInfo, error) {
	if c.err != nil {
		return nil, c.err
	}

	return c.infos, nil
}

func (c *testCatalog) Init(ctx context.Context, dir string, force bool) (*entity.InitResult, error) {
	if c.err != nil {
		return nil, c.err
	}

	return c.result, nil
}

// testSpec returns a spec exercising every flag type
func testSpec() *entity.GenSpec {
	return &entity.GenSpec{
		Type:    valobj.GenAdapter,
		Summary: "generate a secondary adapter",
		Flags: []entity.FlagSpec{
			{Name: "name", Usage: "adapter name, ex. Events", Required: true},
			{Name: "kind", Usage: "adapter kind", Default: "generic"},
			{Name: "tracer", Usage: "include otel tracing", Type: entity.FlagBool, Default: "true"},
			{Name: "field", Usage: "struct field", Type: entity.FlagList},
		},
		Templates: []entity.TemplateRef{
			{Name: "adapter/{{.Kind}}", Out: "internal/adapters/{{.Name.Snake}}.go"},
		},
	}
}

// newTestCli builds a cli over the supplied spec and returns it alongside the
// buffers it writes to and the generator it drives
func newTestCli(t testing.TB, spec *entity.GenSpec) (*CliAdapter, *testGenerator, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	generator := &testGenerator{}
	catalog := &testCatalog{}
	registry := &testRegistry{specs: []*entity.GenSpec{spec}}
	cfg := &config.Config{OutDir: ".", Defaults: map[string]string{}}

	cli := NewCliAdapter(generator, catalog, registry, cfg, "1.2.3",
		WithStdout(&stdout),
		WithStderr(&stderr),
	)

	return cli, generator, &stdout, &stderr
}

// TestDescribeListsEveryGlobalFlag guards the pairing of a global flag name with
// its own usage text. The generate flag set registers them one by one, so a name
// that drifts from its help string would otherwise go unnoticed
func TestDescribeListsEveryGlobalFlag(t *testing.T) {
	cli, _, stdout, _ := newTestCli(t, testSpec())

	if code := cli.Run(context.Background(), []string{DescribeCmd, "adapter"}); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	output := stdout.String()

	for _, flagSpec := range defaultGlobals() {
		line := "  -" + flagSpec.Name

		index := strings.Index(output, line)
		if index < 0 {
			t.Errorf("describe output is missing the %s flag", flagSpec.Name)

			continue
		}

		if !strings.Contains(output[index:], flagSpec.Usage) {
			t.Errorf("flag %s is not followed by its usage %q", flagSpec.Name, flagSpec.Usage)
		}
	}
}

func TestDescribeJSONCarriesGlobals(t *testing.T) {
	cli, _, stdout, _ := newTestCli(t, testSpec())

	if code := cli.Run(context.Background(), []string{DescribeCmd, "adapter", "-json"}); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	// entity.FlagType marshals to its name but declares no UnmarshalJSON, so the
	// payload is decoded into a shape that reads the type back as a string
	type flagRow struct {
		Name  string `json:"name"`
		Usage string `json:"usage"`
		Type  string `json:"type"`
	}

	var payload struct {
		Type    string    `json:"type"`
		Globals []flagRow `json:"globals"`
		Flags   []flagRow `json:"flags"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode describe output: %s", err.Error())
	}

	if payload.Type != "adapter" {
		t.Errorf("got type %s, want adapter", payload.Type)
	}

	expected := defaultGlobals()

	if len(payload.Globals) != len(expected) {
		t.Fatalf("got %d globals, want %d", len(payload.Globals), len(expected))
	}

	for i := range expected {
		if payload.Globals[i].Name != expected[i].Name {
			t.Errorf("got global %s, want %s", payload.Globals[i].Name, expected[i].Name)
		}

		if payload.Globals[i].Usage != expected[i].Usage {
			t.Errorf("global %s carries usage %q, want %q", expected[i].Name, payload.Globals[i].Usage, expected[i].Usage)
		}

		if payload.Globals[i].Type != expected[i].Type.String() {
			t.Errorf("global %s carries type %s, want %s", expected[i].Name, payload.Globals[i].Type, expected[i].Type.String())
		}
	}
}

// TestUsageForResolvesByName checks the lookup the generate flag set relies on,
// including a name that is not a global
func TestUsageForResolvesByName(t *testing.T) {
	cli, _, _, _ := newTestCli(t, testSpec())

	for _, flagSpec := range defaultGlobals() {
		if usage := cli.usageFor(flagSpec.Name); usage != flagSpec.Usage {
			t.Errorf("usageFor(%s) returned %q, want %q", flagSpec.Name, usage, flagSpec.Usage)
		}
	}

	if usage := cli.usageFor("name"); len(usage) > 0 {
		t.Errorf("usageFor(name) returned %q, want an empty string", usage)
	}
}

func TestReservedCoversTheGlobals(t *testing.T) {
	cli, _, _, _ := newTestCli(t, testSpec())

	for _, flagSpec := range defaultGlobals() {
		if !cli.reserved(flagSpec.Name) {
			t.Errorf("%s is a global flag but is not reserved", flagSpec.Name)
		}
	}

	if cli.reserved("name") {
		t.Error("name is a spec flag but reports as reserved")
	}
}

// TestGenerateRejectsReservedFlag checks that a spec redeclaring a global is
// refused before the core is reached
func TestGenerateRejectsReservedFlag(t *testing.T) {
	spec := testSpec()
	spec.Flags = append(spec.Flags, entity.FlagSpec{Name: ForceFlag, Usage: "collides with a global"})

	cli, generator, _, stderr := newTestCli(t, spec)

	if code := cli.Run(context.Background(), []string{GenerateCmd, "adapter", "-name", "Events"}); code != ExitError {
		t.Fatalf("got exit code %d, want %d", code, ExitError)
	}

	if generator.req != nil {
		t.Error("the generator ran despite the flag collision")
	}

	if !strings.Contains(stderr.String(), (&ErrDuplicateFlag{ForceFlag}).Error()) {
		t.Errorf("got %q, want a duplicate flag error", stderr.String())
	}
}

// TestGenerateBuildsRequest checks that the global flags reach the request
func TestGenerateBuildsRequest(t *testing.T) {
	cli, generator, _, _ := newTestCli(t, testSpec())

	args := []string{
		GenerateCmd, "adapter",
		"-name", "Events",
		"-out", "/tmp/generated",
		"-module", "github.com/nxdir-s/demo",
		"-force",
		"-field", "Total:float64",
	}

	if code := cli.Run(context.Background(), args); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	if generator.req == nil {
		t.Fatal("the generator was never called")
	}

	if generator.req.OutDir != "/tmp/generated" {
		t.Errorf("got out dir %s, want /tmp/generated", generator.req.OutDir)
	}

	if generator.req.Module != "github.com/nxdir-s/demo" {
		t.Errorf("got module %s, want github.com/nxdir-s/demo", generator.req.Module)
	}

	if !generator.req.Force {
		t.Error("force did not reach the request")
	}

	if generator.req.DryRun || generator.req.Stdout {
		t.Error("dry-run and stdout defaulted to true")
	}

	if generator.req.Flags["kind"] != "generic" {
		t.Errorf("got kind %s, want the spec default generic", generator.req.Flags["kind"])
	}

	if generator.req.Flags["tracer"] != "true" {
		t.Errorf("got tracer %s, want the spec default true", generator.req.Flags["tracer"])
	}

	if len(generator.req.Lists["field"]) != 1 {
		t.Errorf("got %d fields, want 1", len(generator.req.Lists["field"]))
	}
}

// TestReportNamesEveryDryRunVerb checks the verb every artifact status maps to
func TestReportNamesEveryDryRunVerb(t *testing.T) {
	tests := []struct {
		status entity.ArtifactStatus
		want   string
	}{
		{entity.StatusCreated, "would write internal/adapters/events.go"},
		{entity.StatusAppended, "would append to internal/adapters/events.go"},
		{entity.StatusUnchanged, "would leave unchanged internal/adapters/events.go"},
	}

	for i := range tests {
		test := tests[i]

		t.Run(test.status.String(), func(t *testing.T) {
			cli, generator, stdout, _ := newTestCli(t, testSpec())

			generator.artifacts = []*entity.Artifact{
				{Path: "internal/adapters/events.go", Status: test.status},
			}

			args := []string{GenerateCmd, "adapter", "-name", "Events", "-dry-run"}
			if code := cli.Run(context.Background(), args); code != ExitOk {
				t.Fatalf("got exit code %d, want %d", code, ExitOk)
			}

			if got := strings.TrimSpace(stdout.String()); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestListPrintsRegisteredTypes(t *testing.T) {
	cli, _, stdout, _ := newTestCli(t, testSpec())

	if code := cli.Run(context.Background(), []string{ListCmd}); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	if !strings.Contains(stdout.String(), "adapter") {
		t.Errorf("got %q, want the adapter type", stdout.String())
	}
}

func TestListJSONPrintsRegisteredTypes(t *testing.T) {
	cli, _, stdout, _ := newTestCli(t, testSpec())

	if code := cli.Run(context.Background(), []string{ListCmd, "-json"}); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	var types []listEntry
	if err := json.Unmarshal(stdout.Bytes(), &types); err != nil {
		t.Fatalf("failed to decode list output: %s", err.Error())
	}

	if len(types) != 1 {
		t.Fatalf("got %d types, want 1", len(types))
	}

	if types[0].Type != "adapter" {
		t.Errorf("got type %s, want adapter", types[0].Type)
	}
}

func TestVersionPrintsTheInjectedVersion(t *testing.T) {
	cli, _, stdout, _ := newTestCli(t, testSpec())

	if code := cli.Run(context.Background(), []string{VersionCmd}); code != ExitOk {
		t.Fatalf("got exit code %d, want %d", code, ExitOk)
	}

	if got := strings.TrimSpace(stdout.String()); got != "gopher 1.2.3" {
		t.Errorf("got %q, want 'gopher 1.2.3'", got)
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "no arguments", args: []string{}, want: ExitUsage},
		{name: "unknown command", args: []string{"nope"}, want: ExitUsage},
		{name: "unknown type", args: []string{DescribeCmd, "nope"}, want: ExitUsage},
		{name: "unregistered type", args: []string{DescribeCmd, "setup"}, want: ExitUsage},
		{name: "describe without a type", args: []string{DescribeCmd}, want: ExitUsage},
		{name: "generate without a type", args: []string{GenerateCmd}, want: ExitUsage},
		{name: "unknown flag", args: []string{GenerateCmd, "adapter", "-nope"}, want: ExitUsage},
		{name: "templates without a subcommand", args: []string{TemplatesCmd}, want: ExitUsage},
		{name: "unknown templates subcommand", args: []string{TemplatesCmd, "nope"}, want: ExitUsage},
		{name: "help", args: []string{HelpCmd}, want: ExitOk},
		{name: "short help flag", args: []string{HelpShortFlag}, want: ExitOk},
		{name: "long help flag", args: []string{HelpLongFlag}, want: ExitOk},
	}

	for i := range tests {
		test := tests[i]

		t.Run(test.name, func(t *testing.T) {
			cli, _, _, _ := newTestCli(t, testSpec())

			if code := cli.Run(context.Background(), test.args); code != test.want {
				t.Errorf("got exit code %d, want %d", code, test.want)
			}
		})
	}
}

// TestRunFastPathsNeedOnlyRegistry pins the wiring contract main.go relies on:
// version, list, describe, help, and usage errors are dispatched with a nil
// generator, catalog, and config, so those paths must never touch them — a
// path that does panics here instead of in a user's terminal
func TestRunFastPathsNeedOnlyRegistry(t *testing.T) {
	tests := map[string]struct {
		args []string
		code int
	}{
		"version":  {[]string{"version"}, ExitOk},
		"list":     {[]string{"list"}, ExitOk},
		"describe": {[]string{"describe", "entity"}, ExitOk},
		"help":     {[]string{"help"}, ExitOk},
		"unknown":  {[]string{"nope"}, ExitUsage},
		"empty":    {nil, ExitUsage},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			registry := &testRegistry{specs: []*entity.GenSpec{{Type: valobj.GenEntity, Summary: "test entity"}}}

			cli := NewCliAdapter(nil, nil, registry, nil, "1.2.3",
				WithStdout(&stdout),
				WithStderr(&stderr),
			)

			if code := cli.Run(context.Background(), tc.args); code != tc.code {
				t.Errorf("exit code = %d, want %d", code, tc.code)
			}
		})
	}
}
