package entity

import "github.com/nxdir-s/gopher/internal/core/valobj"

// FlagType identifies how the cli should parse a flag
type FlagType int

const (
	FlagString FlagType = iota
	FlagBool
	FlagList
)

var flagTypeNames = map[FlagType]string{
	FlagString: "string",
	FlagBool:   "bool",
	FlagList:   "list",
}

// String returns the name of the flag type
func (t FlagType) String() string {
	name, ok := flagTypeNames[t]
	if !ok {
		return "string"
	}

	return name
}

// MarshalJSON encodes the flag type as its name
func (t FlagType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// FlagSpec declares a single flag accepted by a generation type
type FlagSpec struct {
	Name     string   `json:"name"`
	Usage    string   `json:"usage"`
	Default  string   `json:"default,omitempty"`
	Type     FlagType `json:"type"`
	Required bool     `json:"required"`
}

// TemplateRef points at a template and the path its output is written to. Both
// fields are themselves templates, so they can be resolved against the request
type TemplateRef struct {
	Name string       `json:"name"`
	Out  string       `json:"out"`
	Mode ArtifactMode `json:"mode"`
}

// GenSpec is the single declaration of a generation type. It drives cli flag
// registration, the describe output, and template selection during render
type GenSpec struct {
	Type           valobj.GenType `json:"type"`
	Summary        string         `json:"summary"`
	RequiresModule bool           `json:"requires_module"`
	Flags          []FlagSpec     `json:"flags"`
	Templates      []TemplateRef  `json:"templates"`
}

// Flag returns the named flag spec
func (s *GenSpec) Flag(name string) (FlagSpec, bool) {
	for i := range s.Flags {
		if s.Flags[i].Name == name {
			return s.Flags[i], true
		}
	}

	return FlagSpec{}, false
}
