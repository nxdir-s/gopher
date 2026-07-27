package adapters

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/nxdir-s/gopher/internal/core/valobj"
)

type ErrParseTemplate struct {
	name string
	err  error
}

func (e *ErrParseTemplate) Error() string {
	return "failed to parse template '" + e.name + "': " + e.err.Error()
}

type ErrExecTemplate struct {
	name string
	err  error
}

func (e *ErrExecTemplate) Error() string {
	return "failed to render template '" + e.name + "': " + e.err.Error()
}

type TemplateAdapter struct {
	funcs template.FuncMap
}

// NewTemplateAdapter creates an adapter for rendering templates
func NewTemplateAdapter() *TemplateAdapter {
	return &TemplateAdapter{
		funcs: template.FuncMap{
			"pascal":   func(s string) string { return valobj.NewNaming(s).Pascal },
			"camel":    func(s string) string { return valobj.NewNaming(s).Camel },
			"snake":    func(s string) string { return valobj.NewNaming(s).Snake },
			"plural":   func(s string) string { return valobj.NewNaming(s).Plural },
			"lower":    strings.ToLower,
			"upper":    strings.ToUpper,
			"contains": strings.Contains,
			"join":     strings.Join,
		},
	}
}

// Render parses and executes the supplied template against the data. Templates
// error on missing keys so a typo fails loudly instead of rendering "<no value>"
func (a *TemplateAdapter) Render(name string, tmpl []byte, data any) ([]byte, error) {
	parsed, err := template.New(name).Funcs(a.funcs).Option("missingkey=error").Parse(string(tmpl))
	if err != nil {
		return nil, &ErrParseTemplate{name, err}
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return nil, &ErrExecTemplate{name, err}
	}

	return buf.Bytes(), nil
}
