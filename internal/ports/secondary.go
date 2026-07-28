package ports

import (
	"context"

	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// FileWriter defines how the core reads and writes generated files. Read
// reports a missing file with an error satisfying errors.Is(err,
// fs.ErrNotExist), which is how the append and ensure paths tell "create it"
// apart from a file that exists but cannot be read
type FileWriter interface {
	Write(ctx context.Context, path string, data []byte) error
	Read(path string) ([]byte, error)
	Exists(path string) bool
}

// GoSource defines how the core merges generated declarations into existing go
// source, for the generators that add to a shared file. Merge reports declared
// true, with dst unchanged, when dst already declares the name
type GoSource interface {
	Merge(dst []byte, src []byte, name string) ([]byte, bool, error)
	Methods(decls []string) ([]valobj.Method, error)
}

// Formatter defines how the core formats and validates generated go source
type Formatter interface {
	Format(src []byte) ([]byte, error)
}

// TemplateSource defines how the core loads template source
type TemplateSource interface {
	Load(name string) ([]byte, error)
}

// TemplateCatalog defines how the core inspects the available templates
type TemplateCatalog interface {
	TemplateSource
	List() ([]string, error)
	Embedded() ([]string, error)
	Origin(name string) (string, bool)
}

// Renderer defines how the core renders templates
type Renderer interface {
	Render(name string, tmpl []byte, data any) ([]byte, error)
}
