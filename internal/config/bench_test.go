package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// benchTree builds a module rooted at a temp dir and returns the deepest
// directory in it, so the walk up has somewhere to walk from
func benchTree(t testing.TB, depth int) string {
	t.Helper()

	root := t.TempDir()

	gomod := "module github.com/nxdir-s/demo\n\ngo 1.26.4\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %s", err.Error())
	}

	dir := root
	for i := range depth {
		dir = filepath.Join(dir, "level"+strconv.Itoa(i))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to build tree: %s", err.Error())
	}

	return dir
}

// BenchmarkLoad measures the config resolution every invocation opens with. It
// runs two independent walks toward the filesystem root, one for the project
// root and one for the module, stat'ing at every level
//
// XDG_CONFIG_HOME is pointed at an empty dir so the number does not depend on
// whether the machine running it has a user config or templates installed
func BenchmarkLoad(b *testing.B) {
	b.Setenv(XdgConfigEnv, b.TempDir())

	depths := map[string]int{"in_repo": 1, "deep": 8}

	for label, depth := range depths {
		dir := benchTree(b, depth)

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := Load(dir); err != nil {
					b.Fatalf("failed to load config: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkFindModule measures the module lookup on its own, the walk an
// explicit -out re-runs against the output tree
func BenchmarkFindModule(b *testing.B) {
	b.Setenv(XdgConfigEnv, b.TempDir())

	dir := benchTree(b, 8)

	for b.Loop() {
		if module := FindModule(dir); len(module) == 0 {
			b.Fatal("got an empty module path, want the module the tree declares")
		}
	}
}
