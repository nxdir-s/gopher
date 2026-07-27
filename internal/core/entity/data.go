package entity

import "github.com/nxdir-s/gopher/internal/core/valobj"

// TemplateData is the contract every template renders against. Templates
// supplied by users receive this exact structure. Every flag a spec declares is
// present in Flags or Lists, so a template can index them without a nil check
type TemplateData struct {
	Name      valobj.Naming
	Package   string
	Module    string
	Kind      string
	GoVersion string
	Fields    []valobj.Field
	Ports     []valobj.Field
	Methods   []valobj.Method
	Flags     map[string]string
	Lists     map[string][]string
	Tracer    bool
	Logger    bool
}
