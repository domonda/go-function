package gen

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_funcDeclArgDescriptions tests that the argument descriptions
// documented as "name: description" lines are found in all documented
// formats, also after gofmt reformatted the doc comment,
// because the generator gofmts the files it writes and a second run
// or -validate must find the same descriptions again.
func Test_funcDeclArgDescriptions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "indented lines",
			source: "// F does something.\n//   a: First number\n//   b: Second number\nfunc F(a, b int) {}\n",
			want:   []string{"First number", "Second number"},
		},
		{
			// gofmt since Go 1.19 formats indented lines as code block with a tab
			name:   "gofmt formatted indented lines",
			source: "// F does something.\n//\n//\ta: First number\n//\tb: Second number\nfunc F(a, b int) {}\n",
			want:   []string{"First number", "Second number"},
		},
		{
			name:   "list items",
			source: "// F does something.\n//\n// Arguments:\n//   - a:  First number\n//   - b: Second number\nfunc F(a, b int) {}\n",
			want:   []string{"First number", "Second number"},
		},
		{
			// Numbered and other Markdown list items describe arguments too
			name:   "numbered and starred list items",
			source: "// F does something.\n//\n// Arguments:\n//  1. a: First number\n//  2) b: Second number\n//   * c: Third number\nfunc F(a, b, c int) {}\n",
			want:   []string{"First number", "Second number", "Third number"},
		},
		{
			// The description ends with its line, not with the comment
			name:   "block comment",
			source: "/*\nF does something.\n  a: First number\n  b: Second number\nMore text.\n*/\nfunc F(a, b int) {}\n",
			want:   []string{"First number", "Second number"},
		},
		{
			// Only a line starting with the name describes the argument
			name:   "prose mentioning name label",
			source: "// F does something with the values a: and b: together.\n//   a: First number\n//   b: Second number\nfunc F(a, b int) {}\n",
			want:   []string{"First number", "Second number"},
		},
		{
			name:   "undocumented argument",
			source: "// F does something.\n//   b: Second number\nfunc F(ctx context.Context, a, b int) {}\n",
			want:   []string{"", "", "Second number"},
		},
		{
			name:   "no doc comment",
			source: "func F(a int) {}\n",
			want:   []string{""},
		},
		{
			// Unnamed arguments get the names arg0, arg1, ... like in ArgNames
			name:   "unnamed arguments",
			source: "// F does something.\n//   arg1: Second number\nfunc F(int, int) {}\n",
			want:   []string{"", "Second number"},
		},
		{
			name:   "no arguments",
			source: "// F does something.\nfunc F() {}\n",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "package p\n\n" + tt.source
			funcDecl := parseFuncDecl(t, source)
			assert.Equal(t, tt.want, funcDeclArgDescriptions(funcDecl))

			// The generator writes the file formatted by gofmt,
			// so the next run parses the reformatted doc comment
			formatted, err := format.Source([]byte(source))
			require.NoError(t, err)
			funcDecl = parseFuncDecl(t, string(formatted))
			assert.Equal(t, tt.want, funcDeclArgDescriptions(funcDecl), "after gofmt")
		})
	}
}

// Test_funcTypeArgs tests that unnamed arguments get synthesized
// names and that every argument has a type, because the generated
// wrappers have to pass every argument to the wrapped function.
func Test_funcTypeArgs(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantNames []string
		wantTypes []string
	}{
		{
			name:      "named",
			source:    "func F(ctx context.Context, a, b int, s ...string) {}",
			wantNames: []string{"ctx", "a", "b", "s"},
			wantTypes: []string{"context.Context", "int", "int", "...string"},
		},
		{
			name:      "unnamed",
			source:    "func F(int, string, ...bool) {}",
			wantNames: []string{"arg0", "arg1", "arg2"},
			wantTypes: []string{"int", "string", "...bool"},
		},
		{
			name:      "blank names",
			source:    "func F(_ int, _ string) {}",
			wantNames: []string{"_", "_"},
			wantTypes: []string{"int", "string"},
		},
		{
			name:   "no arguments",
			source: "func F() {}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcDecl := parseFuncDecl(t, "package p\n\n"+tt.source+"\n")
			assert.Equal(t, tt.wantNames, funcTypeArgNames(funcDecl.Type), "names")
			assert.Equal(t, tt.wantTypes, funcTypeArgTypes(funcDecl.Type, ""), "types")
		})
	}
}

// parseFuncDecl returns the last declaration of source that must be a function.
func parseFuncDecl(t *testing.T, source string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "test.go", source, parser.ParseComments)
	require.NoError(t, err)
	funcDecl, ok := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	require.True(t, ok, "last declaration must be a function")
	return funcDecl
}

// Test_unqualifiedIdents tests that the identifiers a type expression
// refers to without package qualifier are found, because the declarations
// of the generated code would shadow them, but not the names of
// struct fields, parameters, results and methods, which they can't shadow.
func Test_unqualifiedIdents(t *testing.T) {
	tests := []struct {
		typeExpr string
		want     []string
	}{
		{typeExpr: "v", want: []string{"v"}},
		{typeExpr: "*pkg.T"},
		{typeExpr: "map[a][N]pkg.T", want: []string{"N", "a"}},
		{typeExpr: "List[v]", want: []string{"List", "v"}},
		{typeExpr: "struct{ v int; pkg.T `json:\"a\"` }", want: []string{"int"}},
		{typeExpr: "func(ok bool) (err error)", want: []string{"bool", "error"}},
		{typeExpr: "interface{ f(x i) }", want: []string{"i"}},
	}
	for _, tt := range tests {
		t.Run(tt.typeExpr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.typeExpr)
			require.NoError(t, err)
			idents := make(map[string]struct{})
			unqualifiedIdents(expr, idents)
			assert.Equal(t, tt.want, slices.Sorted(maps.Keys(idents)))
		})
	}
}
