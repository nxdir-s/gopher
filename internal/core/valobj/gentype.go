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

// String returns the canonical name of the type
func (t GenType) String() string {
	switch t {
	case GenSetup:
		return "setup"
	case GenAdapter:
		return "adapter"
	case GenPort:
		return "port"
	case GenValobj:
		return "valobj"
	case GenEntity:
		return "entity"
	case GenDomain:
		return "domain"
	case GenModule:
		return "module"
	case GenServer:
		return "server"
	case GenInfra:
		return "infra"
	case GenTest:
		return "test"
	case GenMocks:
		return "mocks"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the type as its canonical name
func (t GenType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// ParseGenType resolves a name or alias to its GenType. The second name in a
// case is an accepted alternate spelling of the canonical one
func ParseGenType(name string) (GenType, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "setup":
		return GenSetup, nil
	case "adapter":
		return GenAdapter, nil
	case "port":
		return GenPort, nil
	case "valobj", "vo":
		return GenValobj, nil
	case "entity":
		return GenEntity, nil
	case "domain":
		return GenDomain, nil
	case "module":
		return GenModule, nil
	case "server", "api", "webserver":
		return GenServer, nil
	case "infra", "cdk":
		return GenInfra, nil
	case "test":
		return GenTest, nil
	case "mocks", "mock":
		return GenMocks, nil
	default:
		return GenUnknown, &ErrUnknownType{name}
	}
}
