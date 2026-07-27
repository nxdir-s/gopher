package domain

import (
	"context"
	"log/slog"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/internal/ports"
)

const GoFileExt string = ".go"

type ErrMissingFlag struct {
	name string
}

func (e *ErrMissingFlag) Error() string {
	return "missing required flag: -" + e.name
}

type ErrMissingModule struct{}

func (e *ErrMissingModule) Error() string {
	return "this type needs a module path, pass -module or run inside a go module"
}

type ErrFileExists struct {
	path string
}

func (e *ErrFileExists) Error() string {
	return "'" + e.path + "' already exists, pass -force to overwrite"
}

type ErrRenderTemplate struct {
	name string
	err  error
}

func (e *ErrRenderTemplate) Error() string {
	return "template '" + e.name + "': " + e.err.Error()
}

func (e *ErrRenderTemplate) Unwrap() error {
	return e.err
}

type ErrMergeArtifact struct {
	path string
	err  error
}

func (e *ErrMergeArtifact) Error() string {
	return "failed to merge into '" + e.path + "': " + e.err.Error()
}

func (e *ErrMergeArtifact) Unwrap() error {
	return e.err
}

type ErrNilDependency struct {
	name string
}

func (e *ErrNilDependency) Error() string {
	return "error nil " + e.name + " in Generator"
}

type GeneratorOpt func(d *Generator) error

// WithRegistry sets the registry used to resolve generation specs
func WithRegistry(registry ports.Registry) GeneratorOpt {
	return func(d *Generator) error {
		d.registry = registry
		return nil
	}
}

// WithTemplateSource sets where template source is loaded from
func WithTemplateSource(source ports.TemplateSource) GeneratorOpt {
	return func(d *Generator) error {
		d.source = source
		return nil
	}
}

// WithRenderer sets the template renderer
func WithRenderer(renderer ports.Renderer) GeneratorOpt {
	return func(d *Generator) error {
		d.renderer = renderer
		return nil
	}
}

// WithFormatter sets the formatter applied to generated go source
func WithFormatter(formatter ports.Formatter) GeneratorOpt {
	return func(d *Generator) error {
		d.formatter = formatter
		return nil
	}
}

// WithFileWriter sets where generated artifacts are written
func WithFileWriter(writer ports.FileWriter) GeneratorOpt {
	return func(d *Generator) error {
		d.writer = writer
		return nil
	}
}

// WithMerger sets how declarations are merged into existing go source
func WithMerger(merger ports.GoSource) GeneratorOpt {
	return func(d *Generator) error {
		d.merger = merger
		return nil
	}
}

type Generator struct {
	registry  ports.Registry
	source    ports.TemplateSource
	renderer  ports.Renderer
	formatter ports.Formatter
	writer    ports.FileWriter
	merger    ports.GoSource
	logger    *slog.Logger
}

// NewGenerator creates the domain responsible for turning a request into files
func NewGenerator(logger *slog.Logger, opts ...GeneratorOpt) (*Generator, error) {
	generator := &Generator{
		logger: logger,
	}

	for _, opt := range opts {
		if err := opt(generator); err != nil {
			return nil, err
		}
	}

	if err := generator.validateDeps(); err != nil {
		return nil, err
	}

	return generator, nil
}

// Generate renders every artifact declared by the request's spec and writes
// them unless the request asked for a preview
func (d *Generator) Generate(ctx context.Context, req *entity.Request) ([]*entity.Artifact, error) {
	spec, err := d.registry.Spec(req.Type)
	if err != nil {
		return nil, err
	}

	if err := d.validateFlags(spec, req); err != nil {
		return nil, err
	}

	data, err := d.templateData(spec, req)
	if err != nil {
		return nil, err
	}

	artifacts := make([]*entity.Artifact, 0, len(spec.Templates))

	for i := range spec.Templates {
		artifact, err := d.render(spec.Templates[i], data, req)
		if err != nil {
			return nil, err
		}

		// a ref whose output path renders empty is switched off by a flag
		if artifact == nil {
			continue
		}

		artifacts = append(artifacts, artifact)
	}

	if req.DryRun || req.Stdout {
		return artifacts, nil
	}

	if !req.Force {
		for i := range artifacts {
			// append and ensure resolve existence themselves during render
			if artifacts[i].Mode != entity.ModeCreate {
				continue
			}

			if d.writer.Exists(artifacts[i].Path) {
				return nil, &ErrFileExists{artifacts[i].Path}
			}
		}
	}

	for i := range artifacts {
		if artifacts[i].Status == entity.StatusUnchanged {
			continue
		}

		if err := d.writer.Write(ctx, artifacts[i].Path, artifacts[i].Content); err != nil {
			return nil, err
		}

		d.logger.Debug("wrote artifact",
			slog.String("path", artifacts[i].Path),
			slog.String("template", artifacts[i].Template),
			slog.String("status", artifacts[i].Status.String()),
		)
	}

	return artifacts, nil
}

// render resolves a template ref and produces the artifact it declares
func (d *Generator) render(ref entity.TemplateRef, data *entity.TemplateData, req *entity.Request) (*entity.Artifact, error) {
	name, err := d.renderer.Render("name:"+ref.Name, []byte(ref.Name), data)
	if err != nil {
		return nil, err
	}

	out, err := d.renderer.Render("out:"+ref.Out, []byte(ref.Out), data)
	if err != nil {
		return nil, err
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}

	tmpl, err := d.source.Load(string(name))
	if err != nil {
		return nil, err
	}

	src, err := d.renderer.Render(string(name), tmpl, data)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(req.OutDir, filepath.FromSlash(string(out)))

	artifact := &entity.Artifact{
		Path:     path,
		Template: string(name),
		Mode:     ref.Mode,
		Status:   entity.StatusCreated,
		Content:  src,
	}

	if ref.Mode == entity.ModeAppend && d.writer.Exists(path) {
		if err := d.merge(artifact, data); err != nil {
			return nil, err
		}
	}

	if ref.Mode == entity.ModeEnsure && !req.Force && d.writer.Exists(path) {
		existing, err := d.writer.Read(path)
		if err != nil {
			return nil, err
		}

		artifact.Status = entity.StatusUnchanged
		artifact.Content = existing

		return artifact, nil
	}

	if !strings.HasSuffix(path, GoFileExt) {
		return artifact, nil
	}

	formatted, err := d.formatter.Format(artifact.Content)
	if err != nil {
		return nil, &ErrRenderTemplate{string(name), err}
	}

	artifact.Content = formatted

	return artifact, nil
}

// merge folds the rendered declarations into the file already on disk. A
// declaration that is already present makes the artifact a no op, so running
// the same generator twice never duplicates it
func (d *Generator) merge(artifact *entity.Artifact, data *entity.TemplateData) error {
	existing, err := d.writer.Read(artifact.Path)
	if err != nil {
		return err
	}

	declared, err := d.merger.Declares(existing, data.Name.Pascal)
	if err != nil {
		return &ErrMergeArtifact{artifact.Path, err}
	}

	if declared {
		artifact.Status = entity.StatusUnchanged
		artifact.Content = existing

		return nil
	}

	merged, err := d.merger.Merge(existing, artifact.Content)
	if err != nil {
		return &ErrMergeArtifact{artifact.Path, err}
	}

	artifact.Status = entity.StatusAppended
	artifact.Content = merged

	return nil
}

// templateData builds the render payload from the request. Every flag the spec
// declares is materialized so templates can index Flags and Lists directly
func (d *Generator) templateData(spec *entity.GenSpec, req *entity.Request) (*entity.TemplateData, error) {
	flags := make(map[string]string, len(spec.Flags))
	lists := make(map[string][]string, len(spec.Flags))

	for i := range spec.Flags {
		name := spec.Flags[i].Name

		if spec.Flags[i].Type == entity.FlagList {
			lists[name] = req.List(name)

			continue
		}

		value := req.Flag(name)
		if len(value) == 0 {
			value = spec.Flags[i].Default
		}

		flags[name] = value
	}

	fields, err := valobj.ParseFields(lists["field"])
	if err != nil {
		return nil, err
	}

	ports, err := valobj.ParseFields(lists["port"])
	if err != nil {
		return nil, err
	}

	methods, err := d.merger.Methods(lists["method"])
	if err != nil {
		return nil, err
	}

	name := flags["name"]
	if len(name) == 0 {
		name = path.Base(req.Module)
	}

	return &entity.TemplateData{
		Name:      valobj.NewNaming(name),
		Package:   flags["pkg"],
		Module:    req.Module,
		Kind:      flags["kind"],
		GoVersion: goVersion(),
		Fields:    fields,
		Ports:     ports,
		Methods:   methods,
		Flags:     flags,
		Lists:     lists,
		Tracer:    flags["tracer"] == "true",
		Logger:    flags["logger"] == "true",
	}, nil
}

// goVersion returns the version of the toolchain gopher was built with, for the
// go directive of a generated go.mod
func goVersion() string {
	version := strings.TrimPrefix(runtime.Version(), "go")

	for i := range version {
		if version[i] != '.' && (version[i] < '0' || version[i] > '9') {
			return version[:i]
		}
	}

	return version
}

// validateFlags checks that every required flag received a value
func (d *Generator) validateFlags(spec *entity.GenSpec, req *entity.Request) error {
	if spec.RequiresModule && len(req.Module) == 0 {
		return &ErrMissingModule{}
	}

	for i := range spec.Flags {
		if !spec.Flags[i].Required {
			continue
		}

		if spec.Flags[i].Type == entity.FlagList {
			if len(req.List(spec.Flags[i].Name)) == 0 {
				return &ErrMissingFlag{spec.Flags[i].Name}
			}

			continue
		}

		if len(req.Flag(spec.Flags[i].Name)) == 0 {
			return &ErrMissingFlag{spec.Flags[i].Name}
		}
	}

	return nil
}

// validateDeps checks that every port the domain drives was supplied
func (d *Generator) validateDeps() error {
	switch {
	case d.registry == nil:
		return &ErrNilDependency{"Registry"}
	case d.source == nil:
		return &ErrNilDependency{"TemplateSource"}
	case d.renderer == nil:
		return &ErrNilDependency{"Renderer"}
	case d.formatter == nil:
		return &ErrNilDependency{"Formatter"}
	case d.writer == nil:
		return &ErrNilDependency{"FileWriter"}
	case d.merger == nil:
		return &ErrNilDependency{"GoSource"}
	default:
		return nil
	}
}
