package adapters

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/nxdir-s/gopher/internal/core/valobj"
)

type ErrParseSource struct {
	err error
}

func (e *ErrParseSource) Error() string {
	return "failed to parse go source: " + e.err.Error()
}

type ErrPackageMismatch struct {
	dst string
	src string
}

func (e *ErrPackageMismatch) Error() string {
	return "cannot merge package '" + e.src + "' into package '" + e.dst + "'"
}

// GoSourceAdapter merges generated declarations into existing go source. It exists
// so generators that add to a shared file, like ports, stay idempotent
type GoSourceAdapter struct{}

// NewGoSourceAdapter creates an adapter for merging go source
func NewGoSourceAdapter() *GoSourceAdapter {
	return &GoSourceAdapter{}
}

// Declares reports whether the supplied source already declares the name at the
// top level, so a second run can be reported as a no op
func (a *GoSourceAdapter) Declares(src []byte, name string) (bool, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		return false, &ErrParseSource{err}
	}

	for _, decl := range file.Decls {
		if declares(decl, name) {
			return true, nil
		}
	}

	return false, nil
}

// Merge appends the declarations in src to dst, unioning the two import
// blocks — unless dst already declares name at the top level, in which case
// dst comes back unchanged with declared true, so running the same generator
// twice never duplicates a declaration. One parse of dst answers both
// questions. The result is unformatted, callers pass it through the formatter
func (a *GoSourceAdapter) Merge(dst []byte, src []byte, name string) ([]byte, bool, error) {
	fset := token.NewFileSet()

	dstFile, err := parser.ParseFile(fset, "dst.go", dst, parser.ParseComments)
	if err != nil {
		return nil, false, &ErrParseSource{err}
	}

	for _, decl := range dstFile.Decls {
		if declares(decl, name) {
			return dst, true, nil
		}
	}

	srcFile, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		return nil, false, &ErrParseSource{err}
	}

	if dstFile.Name.Name != srcFile.Name.Name {
		return nil, false, &ErrPackageMismatch{dstFile.Name.Name, srcFile.Name.Name}
	}

	imports := make(map[string]struct{}, len(dstFile.Imports)+len(srcFile.Imports))

	for _, file := range []*ast.File{dstFile, srcFile} {
		for _, spec := range file.Imports {
			imports[importLine(spec)] = struct{}{}
		}
	}

	var merged strings.Builder

	// everything up to and including the package clause, so build tags and
	// license headers survive the merge
	merged.WriteString(strings.TrimRight(string(dst[:offset(fset, dstFile.Name.End())]), " \t\n"))
	merged.WriteString("\n\n")

	if len(imports) > 0 {
		merged.WriteString(importBlock(imports))
		merged.WriteString("\n")
	}

	dstBody := body(fset, dstFile, dst)
	if len(dstBody) > 0 {
		merged.WriteString(dstBody)
		merged.WriteString("\n\n")
	}

	merged.WriteString(body(fset, srcFile, src))
	merged.WriteString("\n")

	return []byte(merged.String()), false, nil
}

// Methods parses interface method declarations into a form templates can
// re-emit and forward calls to. Unnamed parameters are given generated names so
// a fake can pass them through
func (a *GoSourceAdapter) Methods(decls []string) ([]valobj.Method, error) {
	if len(decls) == 0 {
		return nil, nil
	}

	src := "package p\n\ntype iface interface {\n\t" + strings.Join(decls, "\n\t") + "\n}\n"

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "iface.go", src, 0)
	if err != nil {
		return nil, &ErrParseSource{err}
	}

	fields := interfaceFields(file)
	if fields == nil {
		return nil, &ErrParseSource{errors.New("no interface methods found")}
	}

	methods := make([]valobj.Method, 0, len(fields))

	for _, field := range fields {
		signature, ok := field.Type.(*ast.FuncType)
		if !ok || len(field.Names) == 0 {
			continue
		}

		params, args := parameters(fset, signature, src)

		methods = append(methods, valobj.Method{
			Name:       valobj.NewNaming(field.Names[0].Name),
			Params:     params,
			Results:    results(fset, signature, src),
			Args:       strings.Join(args, ", "),
			HasResults: signature.Results != nil && len(signature.Results.List) > 0,
		})
	}

	return methods, nil
}

// interfaceFields returns the method list of the first interface in the file
func interfaceFields(file *ast.File) []*ast.Field {
	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}

		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}

			return iface.Methods.List
		}
	}

	return nil
}

// parameters renders the parameter list and the argument names to forward
func parameters(fset *token.FileSet, signature *ast.FuncType, src string) (string, []string) {
	if signature.Params == nil || len(signature.Params.List) == 0 {
		return "", nil
	}

	params := make([]string, 0, len(signature.Params.List))
	args := make([]string, 0, len(signature.Params.List))

	unnamed := 0

	for _, field := range signature.Params.List {
		typeName := expr(fset, field.Type, src)

		if len(field.Names) == 0 {
			unnamed++
			name := "arg" + strconv.Itoa(unnamed)

			params = append(params, name+" "+typeName)
			args = append(args, name)

			continue
		}

		names := make([]string, 0, len(field.Names))
		for _, ident := range field.Names {
			names = append(names, ident.Name)
			args = append(args, spreadArg(ident.Name, field.Type))
		}

		params = append(params, strings.Join(names, ", ")+" "+typeName)
	}

	return strings.Join(params, ", "), args
}

// spreadArg appends the variadic marker when forwarding a variadic parameter
func spreadArg(name string, fieldType ast.Expr) string {
	if _, ok := fieldType.(*ast.Ellipsis); ok {
		return name + "..."
	}

	return name
}

// results renders the result list, parenthesized when there is more than one
func results(fset *token.FileSet, signature *ast.FuncType, src string) string {
	if signature.Results == nil || len(signature.Results.List) == 0 {
		return ""
	}

	values := make([]string, 0, len(signature.Results.List))

	for _, field := range signature.Results.List {
		typeName := expr(fset, field.Type, src)

		if len(field.Names) == 0 {
			values = append(values, typeName)

			continue
		}

		names := make([]string, 0, len(field.Names))
		for _, ident := range field.Names {
			names = append(names, ident.Name)
		}

		values = append(values, strings.Join(names, ", ")+" "+typeName)
	}

	if len(values) == 1 && len(signature.Results.List[0].Names) == 0 {
		return values[0]
	}

	return "(" + strings.Join(values, ", ") + ")"
}

// expr returns the source text of an expression
func expr(fset *token.FileSet, node ast.Expr, src string) string {
	start := offset(fset, node.Pos())
	end := offset(fset, node.End())

	if start < 0 || end > len(src) || start > end {
		return ""
	}

	return src[start:end]
}

// declares reports whether the declaration introduces the supplied name
func declares(decl ast.Decl, name string) bool {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return typed.Name.Name == name
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if s.Name.Name == name {
					return true
				}
			case *ast.ValueSpec:
				for _, ident := range s.Names {
					if ident.Name == name {
						return true
					}
				}
			}
		}
	}

	return false
}

// body returns the source following the package clause and import declarations
func body(fset *token.FileSet, file *ast.File, src []byte) string {
	end := file.Name.End()

	for _, decl := range file.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.IMPORT {
			continue
		}

		if generic.End() > end {
			end = generic.End()
		}
	}

	return strings.TrimSpace(string(src[offset(fset, end):]))
}

// importBlock renders an import declaration with the standard library grouped
// ahead of everything else, matching the layout gofmt preserves
func importBlock(imports map[string]struct{}) string {
	stdlib := make([]string, 0, len(imports))
	external := make([]string, 0, len(imports))

	for line := range imports {
		if isStdlib(line) {
			stdlib = append(stdlib, line)

			continue
		}

		external = append(external, line)
	}

	sort.Strings(stdlib)
	sort.Strings(external)

	var block strings.Builder

	block.WriteString("import (\n")

	for i := range stdlib {
		block.WriteString("\t" + stdlib[i] + "\n")
	}

	if len(stdlib) > 0 && len(external) > 0 {
		block.WriteString("\n")
	}

	for i := range external {
		block.WriteString("\t" + external[i] + "\n")
	}

	block.WriteString(")\n")

	return block.String()
}

// isStdlib reports whether an import line refers to the standard library. A
// standard library path never has a dot in its first segment
func isStdlib(line string) bool {
	start := strings.Index(line, `"`)
	if start < 0 {
		return false
	}

	path := strings.Trim(line[start:], `"`)

	segment, _, _ := strings.Cut(path, "/")

	return !strings.Contains(segment, ".")
}

// importLine renders an import spec as it appears inside an import block
func importLine(spec *ast.ImportSpec) string {
	if spec.Name == nil {
		return spec.Path.Value
	}

	return spec.Name.Name + " " + spec.Path.Value
}

// offset converts a position into a byte offset in its file
func offset(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Offset
}
