package adapters

import (
	"errors"
	"strings"
	"testing"
)

const existingPorts string = `package ports

import "context"

// OrderRepository defines how the core drives the order repository
type OrderRepository interface {
	Save(ctx context.Context, id int) error
}
`

func TestMergeAppendsDeclarations(t *testing.T) {
	addition := `package ports

import (
	"io"
)

type EventPublisher interface {
	Publish(w io.Writer) error
}
`

	merged, _, err := NewGoSourceAdapter().Merge([]byte(existingPorts), []byte(addition), "EventPublisher")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	formatted, err := NewFormatAdapter().Format(merged)
	if err != nil {
		t.Fatalf("merged source does not parse: %s", err.Error())
	}

	result := string(formatted)

	for _, expected := range []string{"OrderRepository interface", "EventPublisher interface", `"context"`, `"io"`} {
		if !strings.Contains(result, expected) {
			t.Errorf("merged source is missing %q:\n%s", expected, result)
		}
	}

	if strings.Count(result, "import") != 1 {
		t.Errorf("expected a single import block:\n%s", result)
	}
}

func TestMergeGroupsStdlibImportsFirst(t *testing.T) {
	dst := `package ports

import "github.com/nxdir-s/demo/internal/core/entity"

type Repo interface {
	Save(order *entity.Order) error
}
`

	addition := `package ports

import "context"

type Publisher interface {
	Publish(ctx context.Context) error
}
`

	merged, _, err := NewGoSourceAdapter().Merge([]byte(dst), []byte(addition), "Publisher")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	block := string(merged)

	stdlib := strings.Index(block, `"context"`)
	external := strings.Index(block, `"github.com/nxdir-s/demo/internal/core/entity"`)

	if stdlib < 0 || external < 0 {
		t.Fatalf("expected both imports to survive:\n%s", block)
	}

	if stdlib > external {
		t.Errorf("expected the standard library import first:\n%s", block)
	}
}

func TestMergeDedupesImports(t *testing.T) {
	addition := `package ports

import "context"

type Publisher interface {
	Publish(ctx context.Context) error
}
`

	merged, _, err := NewGoSourceAdapter().Merge([]byte(existingPorts), []byte(addition), "Publisher")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if count := strings.Count(string(merged), `"context"`); count != 1 {
		t.Errorf("context imported %d times, want 1:\n%s", count, merged)
	}
}

func TestMergePreservesHeaderComments(t *testing.T) {
	dst := "//go:build linux\n\n" + existingPorts

	merged, _, err := NewGoSourceAdapter().Merge([]byte(dst), []byte("package ports\n\ntype Extra interface{}\n"), "Extra")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if !strings.HasPrefix(string(merged), "//go:build linux") {
		t.Errorf("build tag was lost:\n%s", merged)
	}
}

func TestMergeRejectsPackageMismatch(t *testing.T) {
	_, _, err := NewGoSourceAdapter().Merge([]byte(existingPorts), []byte("package other\n\ntype Extra interface{}\n"), "Extra")

	var mismatch *ErrPackageMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected ErrPackageMismatch, got %v", err)
	}
}

func TestMergeReportsUnparseableSource(t *testing.T) {
	_, _, err := NewGoSourceAdapter().Merge([]byte("package ports\n\nfunc broken( {\n"), []byte(existingPorts), "Extra")

	var parseErr *ErrParseSource
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ErrParseSource, got %v", err)
	}
}

// TestMergeDeclaredIsNoOp pins the short-circuit: a dst that already declares
// the name comes back byte-identical, flagged declared, without src parsing
func TestMergeDeclaredIsNoOp(t *testing.T) {
	merged, declared, err := NewGoSourceAdapter().Merge([]byte(existingPorts), []byte("package other\n\ntype OrderRepository interface{}\n"), "OrderRepository")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if !declared {
		t.Fatal("expected the existing declaration to be reported")
	}

	if string(merged) != existingPorts {
		t.Errorf("dst was modified:\n%s", merged)
	}
}

func TestDeclares(t *testing.T) {
	src := `package ports

import "context"

const Version string = "1"

type OrderRepository interface {
	Save(ctx context.Context) error
}

func Helper() {}
`

	merger := NewGoSourceAdapter()

	tests := map[string]bool{
		"OrderRepository": true,
		"Version":         true,
		"Helper":          true,
		"EventPublisher":  false,
		"Save":            false,
	}

	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			declared, err := merger.Declares([]byte(src), name)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if declared != expected {
				t.Errorf("Declares(%q) = %v, want %v", name, declared, expected)
			}
		})
	}
}
