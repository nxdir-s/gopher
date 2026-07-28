package domain

// The flag names a spec can declare. A spec declares a flag by one of these
// names in registry.go and the generator reads the value back by the same name
// in generator.go, so the two files have to agree. Naming them is what makes
// that agreement checkable — an undeclared flag is an empty string, not an error
const (
	NameFlag     string = "name"
	KindFlag     string = "kind"
	PkgFlag      string = "pkg"
	TracerFlag   string = "tracer"
	LoggerFlag   string = "logger"
	FieldFlag    string = "field"
	ImportFlag   string = "import"
	ValueFlag    string = "value"
	PortFlag     string = "port"
	MethodFlag   string = "method"
	SideFlag     string = "side"
	DirFlag      string = "dir"
	GoModFlag    string = "gomod"
	MakefileFlag string = "makefile"
	ClaudeFlag   string = "claude"
	CaseFlag     string = "case"
	// JSONTagFlag adds json tags to generated struct fields. It is not the
	// cli's -json output flag, which is a different flag in a different package
	JSONTagFlag string = "json"
)
