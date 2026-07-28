package domain

import (
	"strings"

	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// the adapter kinds. Each one names a template under templates/files/adapter
const (
	AdapterGeneric  string = "generic"
	AdapterKafka    string = "kafka"
	AdapterPostgres string = "postgres"
	AdapterHTTP     string = "http"
	AdapterAWS      string = "aws"
	AdapterGoogle   string = "google"
	AdapterCmd      string = "cmd"
	AdapterTmux     string = "tmux"
	AdapterToml     string = "toml"
	AdapterZip      string = "zip"
)

// the value object kinds. Each one names a template under templates/files/valobj
const (
	ValobjStruct string = "struct"
	ValobjEnum   string = "enum"
)

// the module kinds. Each one names a template under templates/files/module.
// ModuleGeneric shares a spelling with AdapterGeneric but is a separate kind
const (
	ModuleGeneric       string = "generic"
	ModuleLogs          string = "logs"
	ModuleConfig        string = "config"
	ModuleObservability string = "observability"
)

// the ports files a declaration can be appended to. A side names a file, not a
// template, so unlike the kinds above these have no template of their own
const (
	SideCore      string = "core"
	SidePrimary   string = "primary"
	SideSecondary string = "secondary"
)

// AdapterKinds are the adapter templates that ship with gopher. Any other kind
// resolves against the template directories, so users can add their own
func AdapterKinds() []string {
	return []string{
		AdapterGeneric,
		AdapterKafka,
		AdapterPostgres,
		AdapterHTTP,
		AdapterAWS,
		AdapterGoogle,
		AdapterCmd,
		AdapterTmux,
		AdapterToml,
		AdapterZip,
	}
}

// ValobjKinds are the shapes a value object can take
func ValobjKinds() []string {
	return []string{
		ValobjStruct,
		ValobjEnum,
	}
}

// ModuleKinds are the internal modules that ship with a real implementation
func ModuleKinds() []string {
	return []string{
		ModuleGeneric,
		ModuleLogs,
		ModuleConfig,
		ModuleObservability,
	}
}

// PortSides are the files the ports module is split into
func PortSides() []string {
	return []string{
		SideCore,
		SidePrimary,
		SideSecondary,
	}
}

type ErrNoSpec struct {
	genType valobj.GenType
}

func (e *ErrNoSpec) Error() string {
	return "no spec registered for type: " + e.genType.String()
}

// defaultSpecs declares every generation type gopher supports. A spec drives
// cli flag registration, the describe output, and template selection, so adding
// a type means adding an entry here and a template, nothing else
func defaultSpecs() []*entity.GenSpec {
	return []*entity.GenSpec{
		{
			Type:           valobj.GenSetup,
			Summary:        "scaffold a hexagonal go project",
			RequiresModule: true,
			Flags: []entity.FlagSpec{
				{
					Name:  NameFlag,
					Usage: "application name, defaults to the last segment of the module path",
				},
				{
					Name:    GoModFlag,
					Usage:   "generate a go.mod",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
				{
					Name:    MakefileFlag,
					Usage:   "generate a Makefile and .gitignore",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
				{
					Name:    ClaudeFlag,
					Usage:   "generate a CLAUDE.md describing the layout and conventions",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
			},
			Templates: []entity.TemplateRef{
				{Name: "setup/main", Out: "cmd/{{.Name.Snake}}/main.go"},
				{Name: "setup/logger", Out: "internal/logs/logger.go"},
				{Name: "setup/config", Out: "internal/config/config.go"},
				{Name: "setup/ports_core", Out: "internal/ports/core.go"},
				{Name: "setup/ports_primary", Out: "internal/ports/primary.go"},
				{Name: "setup/ports_secondary", Out: "internal/ports/secondary.go"},
				{Name: "setup/doc_adapters", Out: "internal/adapters/doc.go"},
				{Name: "setup/doc_domain", Out: "internal/core/domain/doc.go"},
				{Name: "setup/doc_entity", Out: "internal/core/entity/doc.go"},
				{Name: "setup/doc_valobj", Out: "internal/core/valobj/doc.go"},
				{Name: "setup/gomod", Out: `{{if eq .Flags.gomod "true"}}go.mod{{end}}`},
				{Name: "setup/makefile", Out: `{{if eq .Flags.makefile "true"}}Makefile{{end}}`},
				{Name: "setup/gitignore", Out: `{{if eq .Flags.makefile "true"}}.gitignore{{end}}`},
				{Name: "setup/claude", Out: `{{if eq .Flags.claude "true"}}CLAUDE.md{{end}}`},
			},
		},
		{
			Type:    valobj.GenAdapter,
			Summary: "generate a secondary adapter",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "adapter name, ex. Events",
					Required: true,
				},
				{
					Name:    KindFlag,
					Usage:   "adapter kind: " + strings.Join(AdapterKinds(), ", "),
					Default: AdapterGeneric,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "adapters",
				},
				{
					Name:    TracerFlag,
					Usage:   "include otel tracing",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
				{
					Name:    LoggerFlag,
					Usage:   "include an slog logger",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "adapter/{{.Kind}}",
					Out:  "internal/adapters/{{.Name.Snake}}.go",
				},
				// the http adapter is written against project local types. They are
				// ensured rather than created so a second http adapter, or an edited
				// copy of these files, is left alone
				{
					Name: "http/request",
					Out:  `{{if eq .Kind "http"}}internal/core/entity/request.go{{end}}`,
					Mode: entity.ModeEnsure,
				},
				{
					Name: "http/response",
					Out:  `{{if eq .Kind "http"}}internal/core/entity/response.go{{end}}`,
					Mode: entity.ModeEnsure,
				},
				{
					Name: "http/valobj",
					Out:  `{{if eq .Kind "http"}}internal/core/valobj/http.go{{end}}`,
					Mode: entity.ModeEnsure,
				},
				{
					Name: "http/timing",
					Out:  `{{if eq .Kind "http"}}internal/core/valobj/timing.go{{end}}`,
					Mode: entity.ModeEnsure,
				},
			},
		},
		{
			Type:    valobj.GenEntity,
			Summary: "generate a core entity, a domain object type",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "entity name, ex. Order",
					Required: true,
				},
				{
					Name:  FieldFlag,
					Usage: "struct field as Name:Type[:tag], ex. Total:float64",
					Type:  entity.FlagList,
				},
				{
					Name:  ImportFlag,
					Usage: "import path required by a field type, ex. time",
					Type:  entity.FlagList,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "entity",
				},
				{
					Name:    JSONTagFlag,
					Usage:   "add json tags to untagged fields",
					Type:    entity.FlagBool,
					Default: entity.BoolFalse,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "core/entity",
					Out:  "internal/core/entity/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:    valobj.GenValobj,
			Summary: "generate a value object, a shared immutable type",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "value object name, ex. Status",
					Required: true,
				},
				{
					Name:    KindFlag,
					Usage:   "value object kind: " + strings.Join(ValobjKinds(), ", "),
					Default: ValobjStruct,
				},
				{
					Name:  FieldFlag,
					Usage: "struct field as Name:Type[:tag], for the struct kind",
					Type:  entity.FlagList,
				},
				{
					Name:  ValueFlag,
					Usage: "enum member, ex. Pending, for the enum kind",
					Type:  entity.FlagList,
				},
				{
					Name:  ImportFlag,
					Usage: "import path required by a field type, ex. time",
					Type:  entity.FlagList,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "valobj",
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "valobj/{{.Kind}}",
					Out:  "internal/core/valobj/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:    valobj.GenDomain,
			Summary: "generate a domain, an orchestrator for domain use cases",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "domain name, ex. Orders",
					Required: true,
				},
				{
					Name:  PortFlag,
					Usage: "port the domain drives as Field:ports.Interface",
					Type:  entity.FlagList,
				},
				{
					Name:  ImportFlag,
					Usage: "additional import path, ex. time",
					Type:  entity.FlagList,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "domain",
				},
				{
					Name:    TracerFlag,
					Usage:   "include otel tracing",
					Type:    entity.FlagBool,
					Default: entity.BoolFalse,
				},
				{
					Name:    LoggerFlag,
					Usage:   "include an slog logger",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "core/domain",
					Out:  "internal/core/domain/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:    valobj.GenPort,
			Summary: "add a port interface to the ports module",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "interface name, ex. OrderRepository",
					Required: true,
				},
				{
					Name:    SideFlag,
					Usage:   "which ports file to add to: " + strings.Join(PortSides(), ", "),
					Default: SideSecondary,
				},
				{
					Name:  MethodFlag,
					Usage: "interface method, ex. 'Save(ctx context.Context, order *entity.Order) error'",
					Type:  entity.FlagList,
				},
				{
					Name:  ImportFlag,
					Usage: "import path required by a method signature",
					Type:  entity.FlagList,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "ports",
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "port/interface",
					Out:  "internal/ports/{{.Flags.side}}.go",
					Mode: entity.ModeAppend,
				},
			},
		},
		{
			Type:    valobj.GenServer,
			Summary: "generate an http server, a primary adapter",
			Flags: []entity.FlagSpec{
				{
					Name:    NameFlag,
					Usage:   "server name, ex. API",
					Default: "Server",
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "adapters",
				},
				{
					Name:    PortFlag,
					Usage:   "default listen port",
					Default: "8080",
				},
				{
					Name:    TracerFlag,
					Usage:   "include otel tracing",
					Type:    entity.FlagBool,
					Default: entity.BoolFalse,
				},
				{
					Name:    LoggerFlag,
					Usage:   "include an slog logger",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "server/http",
					Out:  "internal/adapters/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:    valobj.GenModule,
			Summary: "generate an internal module, ex. logs or observability",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "module name, ex. auth",
					Required: true,
				},
				{
					Name:    KindFlag,
					Usage:   "module kind: " + strings.Join(ModuleKinds(), ", "),
					Default: ModuleGeneric,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "module/{{.Kind}}",
					Out:  "internal/{{.Name.Snake}}/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:    valobj.GenMocks,
			Summary: "generate a hand written fake for a port interface",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "interface being faked, ex. OrderRepository",
					Required: true,
				},
				{
					Name:     MethodFlag,
					Usage:    "interface method, ex. 'Save(ctx context.Context, id int) error'",
					Type:     entity.FlagList,
					Required: true,
				},
				{
					Name:  ImportFlag,
					Usage: "import path required by a method signature",
					Type:  entity.FlagList,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "fake",
				},
				{
					Name:    DirFlag,
					Usage:   "directory the fake is written to",
					Default: "internal/adapters/fake",
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "mocks/fake",
					Out:  "{{.Flags.dir}}/{{.Name.Snake}}.go",
				},
			},
		},
		{
			Type:           valobj.GenInfra,
			Summary:        "generate an aws cdk stack in go",
			RequiresModule: true,
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "stack name, ex. Orders",
					Required: true,
				},
				{
					Name:    DirFlag,
					Usage:   "directory the cdk app is written to",
					Default: "infra",
				},
				{
					Name:    GoModFlag,
					Usage:   "generate a go.mod, the cdk app is its own module",
					Type:    entity.FlagBool,
					Default: entity.BoolTrue,
				},
			},
			Templates: []entity.TemplateRef{
				{Name: "infra/main", Out: "{{.Flags.dir}}/main.go"},
				{Name: "infra/stack", Out: "{{.Flags.dir}}/{{.Name.Snake}}_stack.go"},
				{Name: "infra/cdkjson", Out: "{{.Flags.dir}}/cdk.json"},
				{Name: "infra/gomod", Out: `{{if eq .Flags.gomod "true"}}{{.Flags.dir}}/go.mod{{end}}`},
			},
		},
		{
			Type:    valobj.GenTest,
			Summary: "generate a table driven test",
			Flags: []entity.FlagSpec{
				{
					Name:     NameFlag,
					Usage:    "the thing under test, ex. ParseOrder",
					Required: true,
				},
				{
					Name:    PkgFlag,
					Usage:   "package name",
					Default: "domain",
				},
				{
					Name:    DirFlag,
					Usage:   "directory the test is written to",
					Default: "internal/core/domain",
				},
				{
					Name:  CaseFlag,
					Usage: "test case name, ex. 'rejects an empty order'",
					Type:  entity.FlagList,
				},
			},
			Templates: []entity.TemplateRef{
				{
					Name: "test/table",
					Out:  "{{.Flags.dir}}/{{.Name.Snake}}_test.go",
				},
			},
		},
	}
}

type RegistryOpt func(r *Registry)

// WithSpecs replaces the specs the registry resolves against
func WithSpecs(specs []*entity.GenSpec) RegistryOpt {
	return func(r *Registry) {
		r.specs = specs
	}
}

type Registry struct {
	specs []*entity.GenSpec
}

// NewRegistry creates the registry of generation types. Every registry owns its
// own specs, so a caller holding a *entity.GenSpec cannot reach another one
func NewRegistry(opts ...RegistryOpt) *Registry {
	registry := &Registry{
		specs: defaultSpecs(),
	}

	for _, opt := range opts {
		opt(registry)
	}

	return registry
}

// Spec returns the spec for the supplied type
func (r *Registry) Spec(genType valobj.GenType) (*entity.GenSpec, error) {
	for i := range r.specs {
		if r.specs[i].Type == genType {
			return r.specs[i], nil
		}
	}

	return nil, &ErrNoSpec{genType}
}

// Specs returns every registered spec
func (r *Registry) Specs() []*entity.GenSpec {
	return r.specs
}
