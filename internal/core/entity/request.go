package entity

import "github.com/nxdir-s/gopher/internal/core/valobj"

// the string form a bool flag takes between the cli and the templates. A bool
// flag is carried as a string so Flags stays one map, so the cli, the generator
// and the templates all have to agree on the spelling
const (
	BoolTrue  string = "true"
	BoolFalse string = "false"
)

// Request carries everything the cli collected for a single generation
type Request struct {
	Type   valobj.GenType
	Flags  map[string]string
	Lists  map[string][]string
	OutDir string
	Module string
	Force  bool
	DryRun bool
	Stdout bool
}

// Flag returns the value of the named flag
func (r *Request) Flag(name string) string {
	if r.Flags == nil {
		return ""
	}

	return r.Flags[name]
}

// Bool reports whether the named flag was set to true
func (r *Request) Bool(name string) bool {
	return r.Flag(name) == BoolTrue
}

// List returns the values collected for the named repeatable flag
func (r *Request) List(name string) []string {
	if r.Lists == nil {
		return nil
	}

	return r.Lists[name]
}
