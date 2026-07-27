package ports

import (
	"context"

	"github.com/nxdir-s/gopher/internal/core/entity"
)

// Generator defines how external entities drive code generation
type Generator interface {
	Generate(ctx context.Context, req *entity.Request) ([]*entity.Artifact, error)
}

// Catalog defines how external entities inspect and export templates
type Catalog interface {
	List() ([]*entity.TemplateInfo, error)
	Init(ctx context.Context, dir string, force bool) (*entity.InitResult, error)
}
