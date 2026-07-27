package valobj

import "strings"

type ErrUnknownType struct {
	name string
}

func (e *ErrUnknownType) Error() string {
	return "unknown generation type: " + e.name
}

// GenType identifies the kind of code gopher generates
type GenType int

const (
	GenUnknown GenType = iota
	GenSetup
	GenAdapter
	GenPort
	GenValobj
	GenEntity
	GenDomain
	GenModule
	GenServer
	GenInfra
	GenTest
	GenMocks
)

var genTypeNames = map[GenType]string{
	GenSetup:   "setup",
	GenAdapter: "adapter",
	GenPort:    "port",
	GenValobj:  "valobj",
	GenEntity:  "entity",
	GenDomain:  "domain",
	GenModule:  "module",
	GenServer:  "server",
	GenInfra:   "infra",
	GenTest:    "test",
	GenMocks:   "mocks",
}

// genTypeAliases maps alternate spellings onto the canonical type
var genTypeAliases = map[string]GenType{
	"api":       GenServer,
	"webserver": GenServer,
	"cdk":       GenInfra,
	"vo":        GenValobj,
	"mock":      GenMocks,
}

// String returns the canonical name of the type
func (t GenType) String() string {
	name, ok := genTypeNames[t]
	if !ok {
		return "unknown"
	}

	return name
}

// MarshalJSON encodes the type as its canonical name
func (t GenType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// ParseGenType resolves a name or alias to its GenType
func ParseGenType(name string) (GenType, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))

	for genType, genName := range genTypeNames {
		if genName == normalized {
			return genType, nil
		}
	}

	if genType, ok := genTypeAliases[normalized]; ok {
		return genType, nil
	}

	return GenUnknown, &ErrUnknownType{name}
}
