package domain

import (
	"strings"

	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
)

// AdapterKinds are the adapter templates that ship with gopher. Any other kind
// resolves against the template directories, so users can add their own
var AdapterKinds = []string{
	"generic",
	"kafka",
	"postgres",
	"http",
	"aws",
	"google",
	"cmd",
	"tmux",
	"toml",
	"zip",
}

// ValobjKinds are the shapes a value object can take
var ValobjKinds = []string{
	"struct",
	"enum",
}

// ModuleKinds are the internal modules that ship with a real implementation
var ModuleKinds = []string{
	"generic",
	"logs",
	"config",
	"observability",
}

// PortSides are the files the ports module is split into
var PortSides = []string{
	"core",
	"primary",
	"secondary",
}

type ErrNoSpec struct {
	genType valobj.GenType
}

func (e *ErrNoSpec) Error() string {
	return "no spec registered for type: " + e.genType.String()
}

// specs declares every generation type gopher supports. A spec drives cli flag
// registration, the describe output, and template selection, so adding a type
// means adding an entry here and a template, nothing else
var specs = []*entity.GenSpec{
	{
		Type:           valobj.GenSetup,
		Summary:        "scaffold a hexagonal go project",
		RequiresModule: true,
		Flags: []entity.FlagSpec{
			{
				Name:  "name",
				Usage: "application name, defaults to the last segment of the module path",
			},
			{
				Name:    "gomod",
				Usage:   "generate a go.mod",
				Type:    entity.FlagBool,
				Default: "true",
			},
			{
				Name:    "makefile",
				Usage:   "generate a Makefile and .gitignore",
				Type:    entity.FlagBool,
				Default: "true",
			},
			{
				Name:    "claude",
				Usage:   "generate a CLAUDE.md describing the layout and conventions",
				Type:    entity.FlagBool,
				Default: "true",
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
				Name:     "name",
				Usage:    "adapter name, ex. Events",
				Required: true,
			},
			{
				Name:    "kind",
				Usage:   "adapter kind: " + strings.Join(AdapterKinds, ", "),
				Default: "generic",
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "adapters",
			},
			{
				Name:    "tracer",
				Usage:   "include otel tracing",
				Type:    entity.FlagBool,
				Default: "true",
			},
			{
				Name:    "logger",
				Usage:   "include an slog logger",
				Type:    entity.FlagBool,
				Default: "true",
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
				Name:     "name",
				Usage:    "entity name, ex. Order",
				Required: true,
			},
			{
				Name:  "field",
				Usage: "struct field as Name:Type[:tag], ex. Total:float64",
				Type:  entity.FlagList,
			},
			{
				Name:  "import",
				Usage: "import path required by a field type, ex. time",
				Type:  entity.FlagList,
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "entity",
			},
			{
				Name:    "json",
				Usage:   "add json tags to untagged fields",
				Type:    entity.FlagBool,
				Default: "false",
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
				Name:     "name",
				Usage:    "value object name, ex. Status",
				Required: true,
			},
			{
				Name:    "kind",
				Usage:   "value object kind: " + strings.Join(ValobjKinds, ", "),
				Default: "struct",
			},
			{
				Name:  "field",
				Usage: "struct field as Name:Type[:tag], for the struct kind",
				Type:  entity.FlagList,
			},
			{
				Name:  "value",
				Usage: "enum member, ex. Pending, for the enum kind",
				Type:  entity.FlagList,
			},
			{
				Name:  "import",
				Usage: "import path required by a field type, ex. time",
				Type:  entity.FlagList,
			},
			{
				Name:    "pkg",
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
				Name:     "name",
				Usage:    "domain name, ex. Orders",
				Required: true,
			},
			{
				Name:  "port",
				Usage: "port the domain drives as Field:ports.Interface",
				Type:  entity.FlagList,
			},
			{
				Name:  "import",
				Usage: "additional import path, ex. time",
				Type:  entity.FlagList,
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "domain",
			},
			{
				Name:    "tracer",
				Usage:   "include otel tracing",
				Type:    entity.FlagBool,
				Default: "false",
			},
			{
				Name:    "logger",
				Usage:   "include an slog logger",
				Type:    entity.FlagBool,
				Default: "true",
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
				Name:     "name",
				Usage:    "interface name, ex. OrderRepository",
				Required: true,
			},
			{
				Name:    "side",
				Usage:   "which ports file to add to: " + strings.Join(PortSides, ", "),
				Default: "secondary",
			},
			{
				Name:  "method",
				Usage: "interface method, ex. 'Save(ctx context.Context, order *entity.Order) error'",
				Type:  entity.FlagList,
			},
			{
				Name:  "import",
				Usage: "import path required by a method signature",
				Type:  entity.FlagList,
			},
			{
				Name:    "pkg",
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
				Name:    "name",
				Usage:   "server name, ex. API",
				Default: "Server",
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "adapters",
			},
			{
				Name:    "port",
				Usage:   "default listen port",
				Default: "8080",
			},
			{
				Name:    "tracer",
				Usage:   "include otel tracing",
				Type:    entity.FlagBool,
				Default: "false",
			},
			{
				Name:    "logger",
				Usage:   "include an slog logger",
				Type:    entity.FlagBool,
				Default: "true",
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
				Name:     "name",
				Usage:    "module name, ex. auth",
				Required: true,
			},
			{
				Name:    "kind",
				Usage:   "module kind: " + strings.Join(ModuleKinds, ", "),
				Default: "generic",
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
				Name:     "name",
				Usage:    "interface being faked, ex. OrderRepository",
				Required: true,
			},
			{
				Name:     "method",
				Usage:    "interface method, ex. 'Save(ctx context.Context, id int) error'",
				Type:     entity.FlagList,
				Required: true,
			},
			{
				Name:  "import",
				Usage: "import path required by a method signature",
				Type:  entity.FlagList,
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "fake",
			},
			{
				Name:    "dir",
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
				Name:     "name",
				Usage:    "stack name, ex. Orders",
				Required: true,
			},
			{
				Name:    "dir",
				Usage:   "directory the cdk app is written to",
				Default: "infra",
			},
			{
				Name:    "gomod",
				Usage:   "generate a go.mod, the cdk app is its own module",
				Type:    entity.FlagBool,
				Default: "true",
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
				Name:     "name",
				Usage:    "the thing under test, ex. ParseOrder",
				Required: true,
			},
			{
				Name:    "pkg",
				Usage:   "package name",
				Default: "domain",
			},
			{
				Name:    "dir",
				Usage:   "directory the test is written to",
				Default: "internal/core/domain",
			},
			{
				Name:  "case",
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

type Registry struct {
	specs []*entity.GenSpec
}

// NewRegistry creates the registry of generation types
func NewRegistry() *Registry {
	return &Registry{
		specs: specs,
	}
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
