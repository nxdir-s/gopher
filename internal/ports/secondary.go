package ports

import (
	"context"

	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// FileWriter defines how the core reads and writes generated files
type FileWriter interface {
	Write(ctx context.Context, path string, data []byte) error
	Read(path string) ([]byte, error)
	Exists(path string) bool
}

// GoSource defines how the core merges generated declarations into existing go
// source, for the generators that add to a shared file
type GoSource interface {
	Declares(src []byte, name string) (bool, error)
	Merge(dst []byte, src []byte) ([]byte, error)
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
