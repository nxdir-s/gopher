package ports

import (
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// Registry defines how the generation types are discovered
type Registry interface {
	Spec(genType valobj.GenType) (*entity.GenSpec, error)
	Specs() []*entity.GenSpec
}
