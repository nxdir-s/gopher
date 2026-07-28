package domain

import (
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

// flag usage strings naming the kinds above, joined at compile time rather
// than per registry. TestKindUsageStringsMatchKinds pins each to its list
const (
	adapterKindUsage string = "adapter kind: generic, kafka, postgres, http, aws, google, cmd, tmux, toml, zip"
	valobjKindUsage  string = "value object kind: struct, enum"
	moduleKindUsage  string = "module kind: generic, logs, config, observability"
	portSideUsage    string = "which ports file to add to: core, primary, secondary"
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

// The specs below declare every generation type gopher supports. A spec drives
// cli flag registration, the describe output, and template selection, so adding
// a type means adding a constructor here, a case in specFor, an entry in
// genTypes, and a template — nothing else. Each constructor builds one spec so
// resolving a type never pays for the other ten

func specSetup() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specAdapter() *entity.GenSpec {
	return &entity.GenSpec{
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
				Usage:   adapterKindUsage,
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
	}
}

func specEntity() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specValobj() *entity.GenSpec {
	return &entity.GenSpec{
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
				Usage:   valobjKindUsage,
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
	}
}

func specDomain() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specPort() *entity.GenSpec {
	return &entity.GenSpec{
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
				Usage:   portSideUsage,
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
	}
}

func specServer() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specModule() *entity.GenSpec {
	return &entity.GenSpec{
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
				Usage:   moduleKindUsage,
				Default: ModuleGeneric,
			},
		},
		Templates: []entity.TemplateRef{
			{
				Name: "module/{{.Kind}}",
				Out:  "internal/{{.Name.Snake}}/{{.Name.Snake}}.go",
			},
		},
	}
}

func specMocks() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specInfra() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

func specTest() *entity.GenSpec {
	return &entity.GenSpec{
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
	}
}

// genTypes is the canonical order specs are listed and described in
func genTypes() []valobj.GenType {
	return []valobj.GenType{
		valobj.GenSetup,
		valobj.GenAdapter,
		valobj.GenEntity,
		valobj.GenValobj,
		valobj.GenDomain,
		valobj.GenPort,
		valobj.GenServer,
		valobj.GenModule,
		valobj.GenMocks,
		valobj.GenInfra,
		valobj.GenTest,
	}
}

// specFor builds the spec for a single type. TestSpecsCanonicalOrder keeps
// this switch and genTypes in step
func specFor(genType valobj.GenType) *entity.GenSpec {
	switch genType {
	case valobj.GenSetup:
		return specSetup()
	case valobj.GenAdapter:
		return specAdapter()
	case valobj.GenEntity:
		return specEntity()
	case valobj.GenValobj:
		return specValobj()
	case valobj.GenDomain:
		return specDomain()
	case valobj.GenPort:
		return specPort()
	case valobj.GenServer:
		return specServer()
	case valobj.GenModule:
		return specModule()
	case valobj.GenMocks:
		return specMocks()
	case valobj.GenInfra:
		return specInfra()
	case valobj.GenTest:
		return specTest()
	default:
		return nil
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
	cache map[valobj.GenType]*entity.GenSpec
}

// NewRegistry creates the registry of generation types. Default specs are
// built one type at a time on first use, and every registry builds its own,
// so a caller holding a *entity.GenSpec cannot reach another registry's
func NewRegistry(opts ...RegistryOpt) *Registry {
	registry := &Registry{}

	for _, opt := range opts {
		opt(registry)
	}

	return registry
}

// Spec returns the spec for the supplied type. Repeated calls return the same
// pointer, whether or not Specs materialized the full table in between
func (r *Registry) Spec(genType valobj.GenType) (*entity.GenSpec, error) {
	// a registry given WithSpecs resolves against that slice alone
	if r.specs != nil {
		for i := range r.specs {
			if r.specs[i].Type == genType {
				return r.specs[i], nil
			}
		}

		return nil, &ErrNoSpec{genType}
	}

	if spec, ok := r.cache[genType]; ok {
		return spec, nil
	}

	spec := specFor(genType)
	if spec == nil {
		return nil, &ErrNoSpec{genType}
	}

	if r.cache == nil {
		r.cache = make(map[valobj.GenType]*entity.GenSpec, 1)
	}

	r.cache[genType] = spec

	return spec, nil
}

// Specs returns every registered spec in canonical order
func (r *Registry) Specs() []*entity.GenSpec {
	if r.specs == nil {
		types := genTypes()
		specs := make([]*entity.GenSpec, 0, len(types))

		for i := range types {
			// through Spec so a pointer handed out before this call is the
			// one that lands in the table
			spec, err := r.Spec(types[i])
			if err != nil {
				continue
			}

			specs = append(specs, spec)
		}

		r.specs = specs
	}

	return r.specs
}
