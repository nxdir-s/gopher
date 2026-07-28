package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nxdir-s/gopher/internal/config"
)

// benchArgs are the invocations the startup benchmarks replay. Generate runs
// with -stdout so the real filesystem writer never emits anything into the
// directory the benchmark happens to run in
func benchArgs() map[string][]string {
	return map[string][]string{
		"version": {"version"},
		"list":    {"list"},
		"generate_entity": {
			"generate", "entity",
			"-name", "Order",
			"-module", "github.com/nxdir-s/demo",
			"-stdout",
		},
	}
}

// benchSilence redirects the standard streams to the null device and returns a
// func restoring them. run reads the globals directly rather than taking
// writers, so this is the only way to keep benchmark output readable
func benchSilence(t testing.TB) func() {
	t.Helper()

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open the null device: %s", err.Error())
	}

	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devnull, devnull

	return func() {
		os.Stdout, os.Stderr = stdout, stderr
		devnull.Close()
	}
}

// BenchmarkRun measures wiring in process: the logger, the config load, the
// template dirs, all six constructors and the dispatch. This is the startup
// cost that is actually gopher's to reduce
func BenchmarkRun(b *testing.B) {
	b.Setenv(config.XdgConfigEnv, b.TempDir())

	restore := benchSilence(b)
	defer restore()

	args := os.Args
	defer func() { os.Args = args }()

	for label, argv := range benchArgs() {
		b.Run(label, func(b *testing.B) {
			os.Args = append([]string{"gopher"}, argv...)

			for b.Loop() {
				if code := run(); code != 0 {
					b.Fatalf("got exit code %d, want 0", code)
				}
			}
		})
	}
}

// BenchmarkStartup measures the built binary end to end. Process spawn and
// dynamic loading are milliseconds and will dominate a generate measured in
// microseconds, so this is a budget rather than an optimization target
//
// It earns its place two ways. Subtracting version from generate_entity gives
// the generate delta at process scale, which is the honest answer to whether a
// change would be noticeable. Subtracting BenchmarkRun/version from
// BenchmarkStartup/version gives the spawn overhead, the part of the latency
// that is not gopher's to fix
func BenchmarkStartup(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping process startup benchmark in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		b.Skip("go toolchain not available")
	}

	// building here rather than inside a sub benchmark keeps the cost out of
	// every timer, since b.Loop starts the clock at the loop
	binary := filepath.Join(b.TempDir(), "gopher")

	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		b.Fatalf("failed to build the binary: %s\n%s", err.Error(), output)
	}

	home := b.TempDir()

	for label, argv := range benchArgs() {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				cmd := exec.Command(binary, argv...)
				cmd.Env = append(os.Environ(), config.XdgConfigEnv+"="+home)

				if err := cmd.Run(); err != nil {
					b.Fatalf("failed to run the binary: %s", err.Error())
				}
			}
		})
	}
}
