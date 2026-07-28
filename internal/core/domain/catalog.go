package domain

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/ports"
)

const TemplateExt string = ".tmpl"

type ErrNoTemplateDir struct{}

func (e *ErrNoTemplateDir) Error() string {
	return "no template directory to export to, pass -dir"
}

type CatalogOpt func(d *Catalog) error

// WithTemplateCatalog sets where templates are inspected from
func WithTemplateCatalog(catalog ports.TemplateCatalog) CatalogOpt {
	return func(d *Catalog) error {
		d.catalog = catalog
		return nil
	}
}

// WithCatalogWriter sets where exported templates are written
func WithCatalogWriter(writer ports.FileWriter) CatalogOpt {
	return func(d *Catalog) error {
		d.writer = writer
		return nil
	}
}

type Catalog struct {
	catalog ports.TemplateCatalog
	writer  ports.FileWriter
	logger  *slog.Logger
}

// NewCatalog creates the domain that inspects and exports templates
func NewCatalog(logger *slog.Logger, opts ...CatalogOpt) (*Catalog, error) {
	catalog := &Catalog{
		logger: logger,
	}

	for _, opt := range opts {
		if err := opt(catalog); err != nil {
			return nil, err
		}
	}

	switch {
	case catalog.catalog == nil:
		return nil, &ErrNilDependency{"TemplateCatalog"}
	case catalog.writer == nil:
		return nil, &ErrNilDependency{"FileWriter"}
	default:
		return catalog, nil
	}
}

// List returns every available template and the path it resolves from
func (d *Catalog) List() ([]*entity.TemplateInfo, error) {
	names, err := d.catalog.List()
	if err != nil {
		return nil, err
	}

	infos := make([]*entity.TemplateInfo, 0, len(names))

	for i := range names {
		origin, overridden := d.catalog.Origin(names[i])

		infos = append(infos, &entity.TemplateInfo{
			Name:       names[i],
			Origin:     origin,
			Overridden: overridden,
		})
	}

	return infos, nil
}

// Init copies the embedded templates into the supplied directory so they can be
// edited. Existing files are left alone unless force is set
func (d *Catalog) Init(ctx context.Context, dir string, force bool) (*entity.InitResult, error) {
	if len(dir) == 0 {
		return nil, &ErrNoTemplateDir{}
	}

	names, err := d.catalog.Embedded()
	if err != nil {
		return nil, err
	}

	result := &entity.InitResult{
		Dir:     dir,
		Written: make([]string, 0, len(names)),
		Skipped: make([]string, 0),
	}

	for i := range names {
		path := filepath.Join(dir, filepath.FromSlash(names[i])+TemplateExt)

		if !force && d.writer.Exists(path) {
			result.Skipped = append(result.Skipped, path)

			continue
		}

		src, err := d.catalog.Load(names[i])
		if err != nil {
			return nil, err
		}

		if err := d.writer.Write(ctx, path, src); err != nil {
			return nil, err
		}

		if d.logger.Enabled(ctx, slog.LevelDebug) {
			d.logger.Debug("exported template", slog.String("path", path))
		}

		result.Written = append(result.Written, path)
	}

	return result, nil
}
