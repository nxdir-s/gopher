package adapters

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// TestRenderMissingMapKeyErrors pins the missingkey=error option: losing it
// would turn a typo into a silent "<no value>"
func TestRenderMissingMapKeyErrors(t *testing.T) {
	renderer := NewTemplateAdapter()

	if _, err := renderer.Render("bad", []byte("{{.missing}}"), map[string]string{}); err == nil {
		t.Fatal("expected an error for a missing key")
	}
}

// TestRenderFuncs proves every registered template function is callable and
// produces the expected casing
func TestRenderFuncs(t *testing.T) {
	renderer := NewTemplateAdapter()

	tests := map[string]struct {
		src  string
		want string
	}{
		"pascal":   {`{{pascal "order item"}}`, "OrderItem"},
		"camel":    {`{{camel "order item"}}`, "orderItem"},
		"snake":    {`{{snake "OrderItem"}}`, "order_item"},
		"plural":   {`{{plural "order"}}`, "Orders"},
		"lower":    {`{{lower "ORDER"}}`, "order"},
		"upper":    {`{{upper "order"}}`, "ORDER"},
		"contains": {`{{contains "orders" "ord"}}`, "true"},
		"join":     {`{{join .Parts ", "}}`, "a, b"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := renderer.Render(name, []byte(tc.src), struct{ Parts []string }{[]string{"a", "b"}})
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderStaticPassthrough pins the action-free fast path: the bytes come
// back equal but never aliased, so a caller mutating the result cannot reach
// the embedded source
func TestRenderStaticPassthrough(t *testing.T) {
	renderer := NewTemplateAdapter()

	src := []byte("package adapters // no actions here")

	got, err := renderer.Render("static", src, nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if !bytes.Equal(got, src) {
		t.Errorf("got %q, want %q", got, src)
	}

	if &got[0] == &src[0] {
		t.Error("static path must clone, not alias")
	}
}

// TestRenderConcurrent drives parsing renders through one shared adapter from
// many goroutines, for the race detector — the generator fans renders out
func TestRenderConcurrent(t *testing.T) {
	renderer := NewTemplateAdapter()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			src := []byte(`{{pascal "order item"}}-` + strings.Repeat("x", i+1))

			for range 50 {
				got, err := renderer.Render("concurrent", src, nil)
				if err != nil {
					t.Errorf("unexpected error: %s", err.Error())

					return
				}

				if !strings.HasPrefix(string(got), "OrderItem-") {
					t.Errorf("unexpected output: %q", got)

					return
				}
			}
		}()
	}

	wg.Wait()
}
