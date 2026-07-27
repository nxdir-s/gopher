package domain

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// stdlibKinds are the adapter kinds that import nothing outside the standard
// library, so they can be type checked without fetching modules. The remaining
// kinds are only checked for formatting and syntax by the golden tests
var stdlibKinds = []string{"cmd", "zip"}

const testModule string = "github.com/nxdir-s/compilecheck"

// TestGeneratedCodeCompiles renders the stdlib only templates into a throwaway
// module and type checks them. gofmt proves source parses, this proves it builds
func TestGeneratedCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	generator := newEmbeddedGenerator(t)

	for _, kind := range stdlibKinds {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()

			gomod := "module " + testModule + "\n\ngo 1.26.4\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
				t.Fatalf("failed to write go.mod: %s", err.Error())
			}

			req := &entity.Request{
				Type: valobj.GenAdapter,
				Flags: map[string]string{
					"name":   "Events",
					"kind":   kind,
					"pkg":    "adapters",
					"tracer": "false",
					"logger": "false",
				},
				Module: testModule,
				OutDir: dir,
			}

			if _, err := generator.Generate(context.Background(), req); err != nil {
				t.Fatalf("failed to generate: %s", err.Error())
			}

			build(t, dir)
		})
	}
}

// TestGenericAdapterCompiles checks the generic skeleton with telemetry off
func TestGenericAdapterCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := t.TempDir()

	gomod := "module " + testModule + "\n\ngo 1.26.4\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %s", err.Error())
	}

	req := &entity.Request{
		Type: valobj.GenAdapter,
		Flags: map[string]string{
			"name":   "payment gateway",
			"kind":   "generic",
			"pkg":    "adapters",
			"tracer": "false",
			"logger": "false",
		},
		Module: testModule,
		OutDir: dir,
	}

	if _, err := newEmbeddedGenerator(t).Generate(context.Background(), req); err != nil {
		t.Fatalf("failed to generate: %s", err.Error())
	}

	build(t, dir)
}

// TestCoreTypesCompileTogether generates an entity, two ports, and a domain
// that drives them, then type checks the result. This is the check that the
// port append path and the module aware imports actually line up
func TestCoreTypesCompileTogether(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := newModule(t)

	requests := []*entity.Request{
		{
			Type:  valobj.GenEntity,
			Flags: map[string]string{"name": "Order", "pkg": "entity", "json": "true"},
			Lists: map[string][]string{"field": {"ID:int", "Total:float64"}},
		},
		{
			Type:  valobj.GenPort,
			Flags: map[string]string{"name": "OrderRepository", "side": "secondary", "pkg": "ports"},
			Lists: map[string][]string{
				"method": {"Save(ctx context.Context, order *entity.Order) error"},
				"import": {testModule + "/internal/core/entity"},
			},
		},
		{
			Type:  valobj.GenPort,
			Flags: map[string]string{"name": "EventPublisher", "side": "secondary", "pkg": "ports"},
			Lists: map[string][]string{"method": {"Publish(ctx context.Context, event string) error"}},
		},
		{
			Type:  valobj.GenDomain,
			Flags: map[string]string{"name": "Orders", "pkg": "domain", "logger": "true", "tracer": "false"},
			Lists: map[string][]string{"port": {"repo:ports.OrderRepository", "events:ports.EventPublisher"}},
		},
	}

	generator := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter())

	for _, req := range requests {
		req.Module = testModule
		req.OutDir = dir

		if _, err := generator.Generate(context.Background(), req); err != nil {
			t.Fatalf("failed to generate %s: %s", req.Type, err.Error())
		}
	}

	build(t, dir)
}

// TestCompositesCompile type checks the generators whose output stays inside
// the standard library: the http server, the stdlib module kinds, a fake, and a
// table test
func TestCompositesCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := newModule(t)

	requests := []*entity.Request{
		{
			Type:  valobj.GenServer,
			Flags: map[string]string{"name": "API", "pkg": "adapters", "port": "8080", "logger": "true", "tracer": "false"},
		},
		{
			Type:  valobj.GenModule,
			Flags: map[string]string{"name": "logs", "kind": "logs"},
		},
		{
			Type:  valobj.GenModule,
			Flags: map[string]string{"name": "config", "kind": "config"},
		},
		{
			Type:  valobj.GenModule,
			Flags: map[string]string{"name": "auth", "kind": "generic"},
		},
		{
			Type:  valobj.GenMocks,
			Flags: map[string]string{"name": "Notifier", "pkg": "fake", "dir": "internal/adapters/fake"},
			Lists: map[string][]string{
				"method": {"Notify(ctx context.Context, msg string) error", "Count() int", "Close()"},
				"import": {"context"},
			},
		},
		{
			Type:  valobj.GenTest,
			Flags: map[string]string{"name": "ParseOrder", "pkg": "adapters", "dir": "internal/adapters"},
			Lists: map[string][]string{"case": {"rejects an empty order"}},
		},
	}

	generator := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter())

	for _, req := range requests {
		req.Module = testModule
		req.OutDir = dir

		if _, err := generator.Generate(context.Background(), req); err != nil {
			t.Fatalf("failed to generate %s: %s", req.Type, err.Error())
		}
	}

	build(t, dir)
	vet(t, dir)
}

// TestHttpCompanionsCompile type checks the types the http adapter is written
// against. The adapter itself pulls golang.org/x/oauth2 so it cannot be built
// offline, but its companions are stdlib only and are checked here
func TestHttpCompanionsCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := newModule(t)

	req := &entity.Request{
		Type: valobj.GenAdapter,
		Flags: map[string]string{
			"name":   "Client",
			"kind":   "http",
			"pkg":    "adapters",
			"tracer": "false",
			"logger": "false",
		},
		Module: testModule,
		OutDir: dir,
	}

	// render through a fake writer so nothing lands on disk, then write only the
	// companions, leaving out the adapter and its third party imports
	artifacts, err := newEmbeddedGenerator(t).Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to generate: %s", err.Error())
	}

	if len(artifacts) != 5 {
		t.Fatalf("got %d artifacts, want the adapter plus 4 companions", len(artifacts))
	}

	writer := adapters.NewFsAdapter()
	adapterDir := filepath.Join(dir, "internal", "adapters")

	companions := 0

	for i := range artifacts {
		if strings.HasPrefix(artifacts[i].Path, adapterDir) {
			continue
		}

		if artifacts[i].Mode != entity.ModeEnsure {
			t.Errorf("%s should be an ensure artifact, got %s", artifacts[i].Path, artifacts[i].Mode)
		}

		if err := writer.Write(context.Background(), artifacts[i].Path, artifacts[i].Content); err != nil {
			t.Fatalf("failed to write %s: %s", artifacts[i].Path, err.Error())
		}

		companions++
	}

	if companions != 4 {
		t.Fatalf("wrote %d companions, want 4", companions)
	}

	build(t, dir)
	vet(t, dir)
}

// TestHttpCompanionsAreEnsured checks that a second http adapter leaves the
// companion types exactly as they are, including any edits made to them
func TestHttpCompanionsAreEnsured(t *testing.T) {
	dir := t.TempDir()

	generator := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter())

	newAdapter := func(name string) *entity.Request {
		return &entity.Request{
			Type: valobj.GenAdapter,
			Flags: map[string]string{
				"name":   name,
				"kind":   "http",
				"pkg":    "adapters",
				"tracer": "false",
				"logger": "false",
			},
			Module: testModule,
			OutDir: dir,
		}
	}

	first, err := generator.Generate(context.Background(), newAdapter("Client"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	for i := range first {
		if first[i].Status != entity.StatusCreated {
			t.Errorf("%s status = %s, want created", first[i].Path, first[i].Status)
		}
	}

	// an edit the user might make to a generated companion
	timing := filepath.Join(dir, "internal", "core", "valobj", "timing.go")

	edited, err := os.ReadFile(timing)
	if err != nil {
		t.Fatalf("failed to read %s: %s", timing, err.Error())
	}

	edited = append(edited, "\n// edited by the user\n"...)

	if err := os.WriteFile(timing, edited, 0o644); err != nil {
		t.Fatalf("failed to write %s: %s", timing, err.Error())
	}

	second, err := generator.Generate(context.Background(), newAdapter("Other"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	adapterDir := filepath.Join(dir, "internal", "adapters")

	for i := range second {
		if strings.HasPrefix(second[i].Path, adapterDir) {
			if second[i].Status != entity.StatusCreated {
				t.Errorf("the adapter should still be created, got %s", second[i].Status)
			}

			continue
		}

		if second[i].Status != entity.StatusUnchanged {
			t.Errorf("%s status = %s, want unchanged", second[i].Path, second[i].Status)
		}
	}

	after, err := os.ReadFile(timing)
	if err != nil {
		t.Fatalf("failed to read %s: %s", timing, err.Error())
	}

	if string(after) != string(edited) {
		t.Errorf("the user's edit was overwritten:\n%s", after)
	}
}

// TestHttpCompanionsHonorForce checks that -force still regenerates them
func TestHttpCompanionsHonorForce(t *testing.T) {
	dir := t.TempDir()

	generator := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter())

	req := &entity.Request{
		Type: valobj.GenAdapter,
		Flags: map[string]string{
			"name":   "Client",
			"kind":   "http",
			"pkg":    "adapters",
			"tracer": "false",
			"logger": "false",
		},
		Module: testModule,
		OutDir: dir,
	}

	if _, err := generator.Generate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	timing := filepath.Join(dir, "internal", "core", "valobj", "timing.go")

	if err := os.WriteFile(timing, []byte("package valobj\n"), 0o644); err != nil {
		t.Fatalf("failed to write %s: %s", timing, err.Error())
	}

	req.Force = true

	forced, err := generator.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	for i := range forced {
		if forced[i].Status != entity.StatusCreated {
			t.Errorf("%s status = %s, want created under force", forced[i].Path, forced[i].Status)
		}
	}

	restored, err := os.ReadFile(timing)
	if err != nil {
		t.Fatalf("failed to read %s: %s", timing, err.Error())
	}

	if !strings.Contains(string(restored), "type Timing struct") {
		t.Errorf("force did not regenerate the companion:\n%s", restored)
	}
}

// TestSetupScaffoldCompiles generates a whole project and type checks it. The
// scaffold has no third party imports, so this is a real end to end check
func TestSetupScaffoldCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile check in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	dir := t.TempDir()

	req := &entity.Request{
		Type:   valobj.GenSetup,
		Flags:  map[string]string{"name": "demo", "gomod": "true", "makefile": "true"},
		Module: testModule,
		OutDir: dir,
	}

	artifacts, err := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter()).Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to scaffold: %s", err.Error())
	}

	expected := []string{
		"go.mod",
		"Makefile",
		".gitignore",
		filepath.Join("cmd", "demo", "main.go"),
		filepath.Join("internal", "ports", "core.go"),
		filepath.Join("internal", "core", "entity", "doc.go"),
	}

	written := make(map[string]struct{}, len(artifacts))
	for i := range artifacts {
		rel, err := filepath.Rel(dir, artifacts[i].Path)
		if err != nil {
			t.Fatalf("failed to resolve %q: %s", artifacts[i].Path, err.Error())
		}

		written[rel] = struct{}{}
	}

	for i := range expected {
		if _, ok := written[expected[i]]; !ok {
			t.Errorf("scaffold is missing %s", expected[i])
		}
	}

	build(t, dir)
}

// TestSetupHonorsFlags checks that a ref whose output path renders empty is
// skipped, which is how the gomod and makefile flags switch files off
func TestSetupHonorsFlags(t *testing.T) {
	dir := t.TempDir()

	req := &entity.Request{
		Type:   valobj.GenSetup,
		Flags:  map[string]string{"name": "demo", "gomod": "false", "makefile": "false"},
		Module: testModule,
		OutDir: dir,
		DryRun: true,
	}

	artifacts, err := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter()).Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to scaffold: %s", err.Error())
	}

	for i := range artifacts {
		base := filepath.Base(artifacts[i].Path)

		if base == "go.mod" || base == "Makefile" || base == ".gitignore" {
			t.Errorf("%s was generated despite being switched off", base)
		}
	}
}

// TestSetupRequiresModule checks the scaffold refuses to guess a module path
func TestSetupRequiresModule(t *testing.T) {
	req := &entity.Request{
		Type:   valobj.GenSetup,
		Flags:  map[string]string{"name": "demo", "gomod": "true", "makefile": "true"},
		OutDir: t.TempDir(),
		DryRun: true,
	}

	_, err := newEmbeddedGenerator(t).Generate(context.Background(), req)

	var missing *ErrMissingModule
	if !errors.As(err, &missing) {
		t.Fatalf("expected ErrMissingModule, got %v", err)
	}
}

// TestPortAppendIsIdempotent checks that adding the same port twice leaves the
// ports file untouched, and that a second port lands in the same file
func TestPortAppendIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	generator := newEmbeddedGeneratorWith(t, adapters.NewFsAdapter())

	newPort := func(name string) *entity.Request {
		return &entity.Request{
			Type:   valobj.GenPort,
			Flags:  map[string]string{"name": name, "side": "secondary", "pkg": "ports"},
			Lists:  map[string][]string{"method": {"Do(ctx context.Context) error"}},
			Module: testModule,
			OutDir: dir,
		}
	}

	first, err := generator.Generate(context.Background(), newPort("OrderRepository"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if first[0].Status != entity.StatusCreated {
		t.Errorf("first port status = %s, want created", first[0].Status)
	}

	second, err := generator.Generate(context.Background(), newPort("EventPublisher"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if second[0].Status != entity.StatusAppended {
		t.Errorf("second port status = %s, want appended", second[0].Status)
	}

	before, err := os.ReadFile(second[0].Path)
	if err != nil {
		t.Fatalf("failed to read ports file: %s", err.Error())
	}

	third, err := generator.Generate(context.Background(), newPort("EventPublisher"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if third[0].Status != entity.StatusUnchanged {
		t.Errorf("repeat port status = %s, want unchanged", third[0].Status)
	}

	after, err := os.ReadFile(third[0].Path)
	if err != nil {
		t.Fatalf("failed to read ports file: %s", err.Error())
	}

	if string(before) != string(after) {
		t.Errorf("ports file changed on a repeat run:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}

	if count := strings.Count(string(after), "EventPublisher interface"); count != 1 {
		t.Errorf("EventPublisher declared %d times, want 1:\n%s", count, after)
	}
}

// newModule creates a temp directory holding an empty go module
func newModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	gomod := "module " + testModule + "\n\ngo 1.26.4\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %s", err.Error())
	}

	return dir
}

// build runs go build in the supplied module and fails the test on error
func build(t *testing.T, dir string) {
	t.Helper()

	run(t, dir, "build")
}

// vet runs go vet in the supplied module and fails the test on error
func vet(t *testing.T, dir string) {
	t.Helper()

	run(t, dir, "vet")
}

// run executes a go subcommand against every package in the supplied module
func run(t *testing.T, dir string, subcommand string) {
	t.Helper()

	cmd := exec.Command("go", subcommand, "./...")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %s\n%s", subcommand, err.Error(), output)
	}
}
