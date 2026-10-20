package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ungerik/go-astvisit"
)

func Test_parseImplementsComment(t *testing.T) {
	type args struct {
		implementor string
		comment     string
	}
	tests := []struct {
		name            string
		args            args
		wantWrappedFunc string
		wantImpl        Impl
		wantErr         bool
	}{
		{
			name:            "function.Wrapper (generated code)",
			args:            args{implementor: "myFunction", comment: "myFunction wraps my.Function as function.Wrapper (generated code)"},
			wantWrappedFunc: "my.Function",
			wantImpl:        ImplWrapper,
		},
		{
			name:            "function.Wrapper",
			args:            args{implementor: "myFunction", comment: " myFunction wraps my.Function as function.Wrapper "},
			wantWrappedFunc: "my.Function",
			wantImpl:        ImplWrapper,
		},
		{
			name:            "function.Description",
			args:            args{implementor: "myFunction", comment: "myFunction wraps MyFunction as function.Description (generated code)"},
			wantWrappedFunc: "MyFunction",
			wantImpl:        ImplDescription,
		},

		// Invalid:
		{
			name:    "empty",
			args:    args{implementor: "", comment: ""},
			wantErr: true,
		},
		{
			name:    "missing wrapped func",
			args:    args{implementor: "myFunction", comment: "myFunction wraps as function.Wrapper"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWrappedFunc, gotImplements, err := parseImplementsComment(tt.args.implementor, tt.args.comment)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseImplementsComment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotWrappedFunc != tt.wantWrappedFunc {
				t.Errorf("parseImplementsComment() gotWrappedFunc = %v, want %v", gotWrappedFunc, tt.wantWrappedFunc)
			}
			if gotImplements != tt.wantImpl {
				t.Errorf("parseImplementsComment() gotImplements = %v, want %v", gotImplements, tt.wantImpl)
			}
		})
	}
}

// TestRewriteAstFileSource_InMemory demonstrates using RewriteAstFileSource for in-memory testing
func TestRewriteAstFileSource_InMemory(t *testing.T) {
	// Create in-memory source code with a wrapper TODO
	source := []byte(`package testpkg

import "github.com/domonda/go-function"

// SimpleAdd adds two integers.
//   a: First number
//   b: Second number
func SimpleAdd(a, b int) int {
	return a + b
}

var simpleAddWrapper = function.WrapperTODO(SimpleAdd)
`)

	// Parse the source into an AST
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	// Create package files map (single file in this case)
	pkgFiles := map[string]*ast.File{
		"test.go": astFile,
	}

	// Buffer to capture the rewritten output
	var output bytes.Buffer

	// Call RewriteAstFileSource with in-memory source
	err = RewriteAstFileSource(
		fset,
		"testpkg",
		pkgFiles,
		astFile,
		"test.go", // path is only used for error messages
		source,
		false,   // verbose
		&output, // print output
		false,   // not validating
		nil,     // no JSON type replacements
		nil,     // no local import prefixes
	)
	require.NoError(t, err)

	// Verify the output contains generated wrapper code
	result := output.String()
	assert.Contains(t, result, "// simpleAddWrapper wraps SimpleAdd as function.Wrapper (generated code)")
	assert.Contains(t, result, "var simpleAddWrapper simpleAddWrapperT")
	assert.Contains(t, result, "type simpleAddWrapperT struct{}")
	assert.Contains(t, result, "func (simpleAddWrapperT) Name() string")
	assert.Contains(t, result, "func (f simpleAddWrapperT) Call")
	assert.Contains(t, result, "v, ok := args[0].(int)")
	assert.Contains(t, result, "results[0] = SimpleAdd(a.a, a.b) // wrapped call")
}

// TestRewriteAstFileSource_NoWrappers tests handling of files without wrappers
func TestRewriteAstFileSource_NoWrappers(t *testing.T) {
	source := []byte(`package testpkg

// SimpleAdd adds two integers.
func SimpleAdd(a, b int) int {
	return a + b
}
`)

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	pkgFiles := map[string]*ast.File{
		"test.go": astFile,
	}

	var output bytes.Buffer

	err = RewriteAstFileSource(
		fset,
		"testpkg",
		pkgFiles,
		astFile,
		"test.go",
		source,
		false,
		&output,
		false, // not validating
		nil,
		nil,
	)
	require.NoError(t, err)

	// Should not generate anything if there are no wrappers
	assert.Empty(t, output.String())
}

// ExampleRewriteAstFileSource demonstrates in-memory wrapper generation without disk I/O.
// This is useful for testing or integrating wrapper generation into other tools.
func ExampleRewriteAstFileSource() {
	// Define source code with a wrapper TODO in memory
	source := []byte(`package example

import "github.com/domonda/go-function"

// Add adds two integers.
//   a: First number
//   b: Second number
func Add(a, b int) int {
	return a + b
}

var addWrapper = function.WrapperTODO(Add)
`)

	// Parse the source code into an AST
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "example.go", source, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	// Create package files map (single file in this example)
	pkgFiles := map[string]*ast.File{
		"example.go": astFile,
	}

	// Buffer to capture the generated output
	var output bytes.Buffer

	// Generate wrapper code in memory
	err = RewriteAstFileSource(
		fset,         // Token file set
		"example",    // Package name
		pkgFiles,     // All package files
		astFile,      // The AST file to process
		"example.go", // File path (for error messages only)
		source,       // Original source code
		false,        // verbose
		&output,      // Write output here instead of to disk
		false,        // not validating
		nil,          // No JSON type replacements
		nil,          // No local import prefixes
	)
	if err != nil {
		panic(err)
	}

	// Print the generated wrapper code
	fmt.Println(output.String())

	// Output:
	// package example
	//
	// import (
	// 	"context"
	// 	"encoding/json"
	// 	"fmt"
	// 	"reflect"
	//
	// 	"github.com/domonda/go-function"
	// )
	//
	// // Add adds two integers.
	// //
	// //	a: First number
	// //	b: Second number
	// func Add(a, b int) int {
	// 	return a + b
	// }
	//
	// // addWrapper wraps Add as function.Wrapper (generated code)
	// var addWrapper addWrapperT
	//
	// // addWrapperT wraps Add as function.Wrapper (generated code)
	// type addWrapperT struct{}
	//
	// func (addWrapperT) String() string {
	// 	return "example.Add(a int, b int) int"
	// }
	//
	// func (addWrapperT) Name() string {
	// 	return "example.Add"
	// }
	//
	// func (addWrapperT) NumArgs() int      { return 2 }
	// func (addWrapperT) ContextArg() bool  { return false }
	// func (addWrapperT) Variadic() bool    { return false }
	// func (addWrapperT) NumResults() int   { return 1 }
	// func (addWrapperT) ErrorResult() bool { return false }
	//
	// func (addWrapperT) ArgNames() []string {
	// 	return []string{"a", "b"}
	// }
	//
	// func (addWrapperT) ArgDescriptions() []string {
	// 	return []string{"First number", "Second number"}
	// }
	//
	// func (addWrapperT) ArgTypes() []reflect.Type {
	// 	return []reflect.Type{
	// 		reflect.TypeFor[int](),
	// 		reflect.TypeFor[int](),
	// 	}
	// }
	//
	// func (addWrapperT) ResultTypes() []reflect.Type {
	// 	return []reflect.Type{
	// 		reflect.TypeFor[int](),
	// 	}
	// }
	//
	// func (f addWrapperT) Call(_ context.Context, args []any) (results []any, err error) {
	// 	if len(args) > 2 {
	// 		return nil, fmt.Errorf("%d arguments passed, but function %s takes 2", len(args), f)
	// 	}
	// 	var a struct {
	// 		a int
	// 		b int
	// 	}
	// 	if 0 < len(args) && args[0] != nil {
	// 		v, ok := args[0].(int)
	// 		if !ok {
	// 			return nil, fmt.Errorf("can't use %T as type int for argument 'a' of function %s", args[0], f)
	// 		}
	// 		a.a = v
	// 	}
	// 	if 1 < len(args) && args[1] != nil {
	// 		v, ok := args[1].(int)
	// 		if !ok {
	// 			return nil, fmt.Errorf("can't use %T as type int for argument 'b' of function %s", args[1], f)
	// 		}
	// 		a.b = v
	// 	}
	// 	results = make([]any, 1)
	// 	results[0] = Add(a.a, a.b) // wrapped call
	// 	return results, err
	// }
	//
	// func (f addWrapperT) CallWithStrings(_ context.Context, strs ...string) (results []any, err error) {
	// 	if len(strs) > 2 {
	// 		return nil, fmt.Errorf("%d arguments passed, but function %s takes 2", len(strs), f)
	// 	}
	// 	var a struct {
	// 		a int
	// 		b int
	// 	}
	// 	if 0 < len(strs) {
	// 		err := function.ScanString(strs[0], &a.a)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgString(err, f, "a", strs[0])
	// 		}
	// 	}
	// 	if 1 < len(strs) {
	// 		err := function.ScanString(strs[1], &a.b)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgString(err, f, "b", strs[1])
	// 		}
	// 	}
	// 	results = make([]any, 1)
	// 	results[0] = Add(a.a, a.b) // wrapped call
	// 	return results, err
	// }
	//
	// func (f addWrapperT) CallWithNamedStrings(_ context.Context, strs map[string]string) (results []any, err error) {
	// 	var a struct {
	// 		a int
	// 		b int
	// 	}
	// 	if str, ok := strs["a"]; ok {
	// 		err := function.ScanString(str, &a.a)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgString(err, f, "a", str)
	// 		}
	// 	}
	// 	if str, ok := strs["b"]; ok {
	// 		err := function.ScanString(str, &a.b)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgString(err, f, "b", str)
	// 		}
	// 	}
	// 	results = make([]any, 1)
	// 	results[0] = Add(a.a, a.b) // wrapped call
	// 	return results, err
	// }
	//
	// func (f addWrapperT) CallWithJSON(_ context.Context, argsJSON []byte) (results []any, err error) {
	// 	var args map[string]json.RawMessage
	// 	err = json.Unmarshal(argsJSON, &args)
	// 	if err != nil {
	// 		return nil, function.NewErrParseArgsJSON(err, f)
	// 	}
	// 	var a struct {
	// 		a int
	// 		b int
	// 	}
	// 	if raw, ok := args["a"]; ok {
	// 		err = json.Unmarshal(raw, &a.a)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgJSON(err, f, "a")
	// 		}
	// 	}
	// 	if raw, ok := args["b"]; ok {
	// 		err = json.Unmarshal(raw, &a.b)
	// 		if err != nil {
	// 			return nil, function.NewErrParseArgJSON(err, f, "b")
	// 		}
	// 	}
	// 	results = make([]any, 1)
	// 	results[0] = Add(a.a, a.b) // wrapped call
	// 	return results, err
	// }
}

// TestRewriteAstFileSource_ValidateUpToDate tests that validation passes for up-to-date wrappers
func TestRewriteAstFileSource_ValidateUpToDate(t *testing.T) {
	// In real-world usage, developers run gen-func-wrappers to generate code,
	// commit it, and CI validates it hasn't changed. However, the formatting
	// might differ slightly between runs due to import ordering or whitespace.
	//
	// The validation works by:
	// 1. Finding wrapper declarations (TODO or generated code comments)
	// 2. Regenerating the wrapper code
	// 3. Comparing the regenerated code with the current source
	//
	// Since the tool always normalizes formatting during generation, validation
	// will fail if the source differs from what would be generated, even if only
	// by whitespace or import order.
	//
	// To properly test validation, we should test that:
	// - Validation passes when source has NO wrappers to generate (empty case)
	// - Validation fails when source has TODOs that need generation (tested separately)
	// - Validation fails when generated code is manually modified (tested separately)

	// Test case: No wrappers at all (validation should pass - nothing to validate)
	sourceNoWrappers := []byte(`package testpkg

// SimpleAdd adds two integers.
func SimpleAdd(a, b int) int {
	return a + b
}
`)

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", sourceNoWrappers, parser.ParseComments)
	require.NoError(t, err)

	pkgFiles := map[string]*ast.File{
		"test.go": astFile,
	}

	// Validation should pass - no wrappers means nothing out of date
	err = RewriteAstFileSource(
		fset,
		"testpkg",
		pkgFiles,
		astFile,
		"test.go",
		sourceNoWrappers,
		false, // verbose
		nil,   // no print output
		true,  // validate mode
		nil,
		nil,
	)
	require.NoError(t, err, "validation should pass when there are no wrappers")
}

// TestRewriteAstFileSource_ValidateOutdated tests that validation fails for outdated wrappers
func TestRewriteAstFileSource_ValidateOutdated(t *testing.T) {
	// Create source with TODO that needs generation
	source := []byte(`package testpkg

import "github.com/domonda/go-function"

// SimpleAdd adds two integers.
//   a: First number
//   b: Second number
func SimpleAdd(a, b int) int {
	return a + b
}

var simpleAddWrapper = function.WrapperTODO(SimpleAdd)
`)

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	pkgFiles := map[string]*ast.File{
		"test.go": astFile,
	}

	// Run validation - should fail because wrapper needs generation
	err = RewriteAstFileSource(
		fset,
		"testpkg",
		pkgFiles,
		astFile,
		"test.go",
		source,
		false, // verbose
		nil,   // no print output
		true,  // validate mode
		nil,
		nil,
	)
	require.Error(t, err, "validation should fail for outdated wrappers")
	assert.Contains(t, err.Error(), "function wrappers are missing or outdated")
}

// TestRewriteAstFileSource_ValidateModified tests that validation fails when wrapper code is modified
func TestRewriteAstFileSource_ValidateModified(t *testing.T) {
	// Create source with wrapper that has been manually modified
	source := []byte(`package testpkg

import (
	"context"
	"reflect"

	"github.com/domonda/go-function"
)

// SimpleAdd adds two integers.
//   a: First number
//   b: Second number
func SimpleAdd(a, b int) int {
	return a + b
}

// simpleAddWrapper wraps SimpleAdd as function.Wrapper (generated code)
var simpleAddWrapper simpleAddWrapperT

// simpleAddWrapperT wraps SimpleAdd as function.Wrapper (generated code)
type simpleAddWrapperT struct{}

// This is a manually modified implementation that differs from generated code
func (simpleAddWrapperT) Call(_ context.Context, args []any) (results []any, err error) {
	// Custom implementation
	return []any{42}, nil
}
`)

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	pkgFiles := map[string]*ast.File{
		"test.go": astFile,
	}

	// Run validation - should fail because wrapper was modified
	err = RewriteAstFileSource(
		fset,
		"testpkg",
		pkgFiles,
		astFile,
		"test.go",
		source,
		false, // verbose
		nil,   // no print output
		true,  // validate mode
		nil,
		nil,
	)
	require.Error(t, err, "validation should fail for modified wrappers")
	assert.Contains(t, err.Error(), "function wrappers are missing or outdated")
}

// TestRewriteAstFileSource_ValidateGenerated tests that the generated output
// passes validation, because CI runs gen-func-wrappers -validate
// on the files that the generator has written.
// The first run adds imports and groups them like goimports,
// the validation run finds all imports present and only formats like gofmt.
func TestRewriteAstFileSource_ValidateGenerated(t *testing.T) {
	// Wraps a function of an imported package with an alias
	// whose parameter type needs the import "go/ast"
	source := []byte(`package testpkg

import (
	"github.com/domonda/go-function"
	av "github.com/ungerik/go-astvisit"
)

var exprStringWrapper = function.WrapperTODO(av.ExprString)
`)
	localImportPrefixes := []string{"github.com/domonda/"}

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	var generated bytes.Buffer
	err = RewriteAstFileSource(
		fset,
		"testpkg",
		map[string]*ast.File{"test.go": astFile},
		astFile,
		"test.go", // its directory resolves the imports like the go command, here through go.work
		source,
		false,      // verbose
		&generated, // print output
		false,      // not validating
		nil,        // no JSON type replacements
		localImportPrefixes,
	)
	require.NoError(t, err)
	assert.Contains(t, generated.String(), "v, ok := args[0].(ast.Expr)")
	assert.Contains(t, generated.String(), "results[0] = av.ExprString(a.expr) // wrapped call")
	assert.Contains(t, generated.String(), "\t\"go/ast\"\n", "import of the parameter type should be added")

	fset = token.NewFileSet()
	astFile, err = parser.ParseFile(fset, "test.go", generated.Bytes(), parser.ParseComments)
	require.NoError(t, err)

	err = RewriteAstFileSource(
		fset,
		"testpkg",
		map[string]*ast.File{"test.go": astFile},
		astFile,
		"test.go",
		generated.Bytes(),
		false, // verbose
		nil,   // no print output
		true,  // validate mode
		nil,   // no JSON type replacements
		localImportPrefixes,
	)
	require.NoError(t, err, "validation should pass for the output of the generator")
}

// TestRewriteFile_ExcludedFile tests that a file named on the command line
// that is not part of the package build is reported as an error,
// because the file can't be rewritten without the package it belongs to.
func TestRewriteFile_ExcludedFile(t *testing.T) {
	dir := t.TempDir()
	funcsFile := filepath.Join(dir, "funcs.go")
	err := os.WriteFile(funcsFile, []byte("package p\n\nfunc Add(a, b int) int { return a + b }\n"), 0600)
	require.NoError(t, err)
	// Files with build constraints that need custom build tags
	// are excluded from the package like by the go command
	taggedSource := []byte("//go:build integration\n\npackage p\n\nimport \"github.com/domonda/go-function\"\n\nvar addWrapper = function.WrapperTODO(Add)\n")
	taggedFile := filepath.Join(dir, "wrappers_integration.go")
	err = os.WriteFile(taggedFile, taggedSource, 0600)
	require.NoError(t, err)

	err = RewriteFile(taggedFile, false, nil, false, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), taggedFile, "error should name the file")

	source, err := os.ReadFile(taggedFile)
	require.NoError(t, err)
	assert.Equal(t, string(taggedSource), string(source), "file should be unchanged")
}

// sourceImporter type checks imported packages from their source,
// located like the go command does in the directory of this test package,
// here through go.work, so the generated code is checked
// against the go-function package of this repository.
var sourceImporter = importer.ForCompiler(token.NewFileSet(), "source", nil)

// generateWrappers rewrites the wrappers of the file target of the package
// with the in-memory files and returns the rewritten target file.
// The files are in the directory of this test package,
// so their imports are resolved like the go command does there.
func generateWrappers(t *testing.T, files map[string]string, target string, jsonTypeReplacements map[string]string) (string, error) {
	t.Helper()
	fset := token.NewFileSet()
	pkgFiles := make(map[string]*ast.File, len(files))
	for name, source := range files {
		file, err := parser.ParseFile(fset, name, source, parser.ParseComments)
		require.NoError(t, err)
		pkgFiles[name] = file
	}
	var generated bytes.Buffer
	err := RewriteAstFileSource(fset, pkgFiles[target].Name.Name, pkgFiles, pkgFiles[target], target, []byte(files[target]), false, &generated, false, jsonTypeReplacements, nil)
	return generated.String(), err
}

// requireValidGenerated requires that the package with the generated
// target file compiles, including that it has no unused imports,
// and that the generator doesn't change the generated file again,
// so that -validate passes for the files written by the generator.
func requireValidGenerated(t *testing.T, files map[string]string, target, generated string, jsonTypeReplacements map[string]string) {
	t.Helper()
	files = maps.Clone(files)
	files[target] = generated

	fset := token.NewFileSet()
	var astFiles []*ast.File
	for _, name := range slices.Sorted(maps.Keys(files)) {
		file, err := parser.ParseFile(fset, name, files[name], 0)
		require.NoError(t, err, "generated code should parse:\n%s", generated)
		astFiles = append(astFiles, file)
	}
	conf := types.Config{Importer: sourceImporter}
	_, err := conf.Check(astFiles[0].Name.Name, fset, astFiles, nil)
	require.NoError(t, err, "generated code should compile:\n%s", generated)

	regenerated, err := generateWrappers(t, files, target, jsonTypeReplacements)
	require.NoError(t, err)
	require.Equal(t, generated, regenerated, "a second run should not change the generated code")
}

// TestRewriteAstFileSource_GeneratedCodeCompiles tests that the generated
// wrappers compile for function signatures, imports and wrapper declarations
// that the generator got wrong, and that a second run doesn't change them,
// because CI runs -validate on the generated files.
func TestRewriteAstFileSource_GeneratedCodeCompiles(t *testing.T) {
	const funcs = `package testpkg

import "context"

// Add adds two integers.
//   a: First number
//   b: Second number
func Add(a, b int) int { return a + b }

func Join(a, b string) string { return a + b }

func Pair(a, b any) [2]any { return [2]any{a, b} }

func Now() {}

func WithCtx(ctx context.Context) error { return ctx.Err() }
`
	tests := []struct {
		name                 string
		files                map[string]string // package files besides funcs
		jsonTypeReplacements map[string]string
		wantContains         []string
		wantNotContains      []string
	}{
		{
			// gofmt reformats "//   a: First number" of a function in the
			// rewritten file to "//\ta: First number", the second run
			// must still find the descriptions
			name: "argument descriptions of function in rewritten file",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

// Sub subtracts two integers.
//   a: First number
//   b: Second number
func Sub(a, b int) int { return a - b }

var subWrapper = function.WrapperTODO(Sub)
`},
			wantContains: []string{`return []string{"First number", "Second number"}`},
		},
		{
			// Like ReflectWrapper, CallWithJSON returns an error for JSON
			// that is no object also without arguments, so it unmarshals
			// the JSON object without keeping it
			name: "functions without arguments",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

var (
	nowWrapper     = function.WrapperTODO(Now)
	withCtxWrapper = function.WrapperTODO(WithCtx)
)
`},
			wantContains: []string{"err = json.Unmarshal(argsJSON, new(map[string]json.RawMessage))", "function.NewErrParseArgsJSON(err, f)"},
		},
		{
			// Like ReflectWrapper, String writes empty interfaces as any,
			// also in results and composite types, so that the generated
			// and the reflected wrapper of a function describe it the same way
			name: "empty interfaces in signature",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

func Loose(m map[string]interface{}, xs ...interface{}) (interface{}, []interface{}, error) {
	return nil, nil, nil
}

func One() interface{} { return nil }

var (
	looseWrapper = function.WrapperTODO(Loose)
	oneWrapper   = function.WrapperTODO(One)
)
`},
			wantContains: []string{
				`return "testpkg.Loose(m map[string]any, xs ...any) (any, []any, error)"`,
				`return "testpkg.One() any"`,
			},
		},
		{
			name: "CallWrapper",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

var addCall = function.CallWrapperTODO(Add)
`},
			wantNotContains: []string{"github.com/domonda/go-function"},
		},
		{
			// Only the CallWrapper needs the import of reflect
			// to accept a value assignable to the named type,
			// the value of a basic type has to have its type,
			// so its argument doesn't get the reflection code
			name: "CallWrapper with argument of named type",
			files: map[string]string{
				"types.go": "package testpkg\n\ntype MyInts []int\n\nfunc Sum(xs MyInts, n int) int { return n }\n",
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var sumCall = function.CallWrapperTODO(Sum)
`},
			wantContains:    []string{"\t\"reflect\"\n", "reflect.TypeOf(args[0]).AssignableTo(reflect.TypeFor[MyInts]())"},
			wantNotContains: []string{"reflect.TypeFor[int]()"},
		},
		{
			// Arguments of type any get the unparsed string,
			// so go-function is not used and its import is removed
			name: "any arguments",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

var pairStrings = function.CallWithStringsWrapperTODO(Pair)

var pairNamedStrings = function.CallWithNamedStringsWrapperTODO(Pair)
`},
			wantNotContains: []string{"github.com/domonda/go-function"},
		},
		{
			// Like ReflectWrapper, strings are scanned,
			// so that scanners registered for strings apply
			name: "string arguments",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

var joinStrings = function.CallWithStringsWrapperTODO(Join)

var joinNamedStrings = function.CallWithNamedStringsWrapperTODO(Join)
`},
			wantContains: []string{"function.ScanString(strs[0], &a.a)", "function.ScanString(str, &a.a)"},
		},
		{
			// CallWithJSON was only generated together with CallWithNamedStrings
			// and function.CallWithJSONWrapperTODO was skipped
			name: "CallWithJSONWrapper",
			files: map[string]string{
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var addJSON = function.CallWithJSONWrapperTODO(Add)
`,
				"check.go": `package testpkg

import "github.com/domonda/go-function"

var _ function.CallWithJSONWrapper = addJSONT{}
`,
			},
			wantContains: []string{"func (f addJSONT) CallWithJSON("},
		},
		{
			// The file uses the name token for go/scanner, so go/token gets
			// another name that every generated method has to use,
			// but not for the package of mytoken
			name: "parameter type package renamed in target file",
			files: map[string]string{
				"types.go": `package testpkg

import (
	"go/token"
	mytoken "go/build/constraint"
)

func Pos(pos token.Pos, expr mytoken.Expr, more ...token.Pos) token.Pos { return pos }
`,
				"test.go": `package testpkg

import (
	token "go/scanner"

	"github.com/domonda/go-function"
)

var _ token.Mode

var posWrapper = function.WrapperTODO(Pos)
`},
			wantContains:    []string{`token2 "go/token"`, "token2.Pos"},
			wantNotContains: []string{"mytoken2"},
		},
		{
			// The generated code has to use the name of the target file for go/token
			name: "parameter type package with alias in function file",
			files: map[string]string{
				"types.go": `package testpkg

import gtk "go/token"

func IsValid(pos gtk.Pos) bool { return pos.IsValid() }
`,
				"test.go": `package testpkg

import (
	"go/token"

	"github.com/domonda/go-function"
)

var _ token.Pos

var isValidWrapper = function.WrapperTODO(IsValid)
`},
			// The String method documents the signature of the wrapped function
			wantContains:    []string{"reflect.TypeFor[token.Pos]()", "v, ok := args[0].(token.Pos)"},
			wantNotContains: []string{"gtk.Pos]", ".(gtk.Pos)", "gtk.Pos\n"},
		},
		{
			// Only the variadic parameter is passed as slice,
			// not the variadic parameter of a function type
			name: "variadic parameters",
			files: map[string]string{
				"types.go": `package testpkg

func Variadic(sum func(...int) int, strs ...string) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var variadicWrapper = function.WrapperTODO(Variadic)
`},
			wantContains: []string{"reflect.TypeFor[func(...int) int]()", "reflect.TypeFor[[]string]()"},
		},
		{
			// Every argument has to be passed to the wrapped function
			name: "unnamed and blank parameters",
			files: map[string]string{
				"types.go": `package testpkg

func Unnamed(int, string) bool { return true }

func Blank(_ int, _ string) bool { return true }
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var unnamedWrapper = function.WrapperTODO(Unnamed)

var blankWrapper = function.WrapperTODO(Blank)
`},
			wantContains: []string{`return []string{"arg0", "arg1"}`, `return []string{"_", "_"}`},
		},
		{
			// A type without var is used like func(T{}) and must stay a type
			name: "type without var",
			files: map[string]string{
				"test.go": `package testpkg

// AddWrapper wraps Add as function.CallWrapper (generated code)
type AddWrapper struct{}
`,
				"check.go": `package testpkg

import "github.com/domonda/go-function"

var _ function.CallWrapper = AddWrapper{}
`},
			wantContains:    []string{"type AddWrapper struct{}", "func (f AddWrapper) Call("},
			wantNotContains: []string{"var AddWrapper", "AddWrapperT"},
		},
		{
			// The var declared after its type and methods
			// must not make the type a wrapper of its own
			name: "var after type and methods",
			files: map[string]string{"test.go": `package testpkg

import "context"

// addWrapperT wraps Add as function.CallWrapper (generated code)
type addWrapperT struct{}

func (addWrapperT) Call(_ context.Context, args []any) ([]any, error) { return nil, nil }

// addWrapper wraps Add as function.CallWrapper (generated code)
var addWrapper addWrapperT
`},
			wantContains:    []string{"var addWrapper addWrapperT"},
			wantNotContains: []string{"addWrapperTT"},
		},
		{
			// Without the import of strings for the replacement type
			// the generated code doesn't compile, and a variadic
			// parameter has to be replaced by its element type
			name: "JSON type replacement of other package",
			files: map[string]string{
				"types.go": `package testpkg

import (
	"io"
	"strings"
)

var _ = strings.NewReader

func Read(r io.Reader, more ...io.Reader) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var readWrapper = function.WrapperTODO(Read)
`},
			jsonTypeReplacements: map[string]string{"io.Reader": "*strings.Reader"},
			// The variadic argument unmarshals its elements one by one
			// into the replacement type
			wantContains: []string{"\"strings\"\n", "\t\tvar v *strings.Reader\n", "\t\t\tvar v *strings.Reader\n"},
		},
		{
			// The generated code has to use the import name of go-function
			name: "go-function imported with alias",
			files: map[string]string{"test.go": `package testpkg

import fn "github.com/domonda/go-function"

var addWrapper = fn.WrapperTODO(Add)
`},
			wantContains: []string{"fn.ScanString("},
		},
		{
			name: "go-function dot imported",
			files: map[string]string{"test.go": `package testpkg

import . "github.com/domonda/go-function"

var addWrapper = WrapperTODO(Add)
`},
			wantContains: []string{" ScanString("},
		},
		{
			// The wrappers are generated after the var block
			// that keeps its other declarations
			name: "TODO calls in var block",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

var (
	addWrapper = function.WrapperTODO(Add)
	other      = 1
	// joinWrapper wraps Join
	joinWrapper = function.CallWrapperTODO(Join)
)

var (
	add1 = function.CallWrapperTODO(Add)
	add2 = function.CallWrapperTODO(Add)
)
`},
			wantContains: []string{"var (\n\tother = 1\n)\n", "var addWrapper addWrapperT", "var joinWrapper joinWrapperT", "var add1 add1T", "var add2 add2T"},
		},
		{
			// The quotes of the struct tag have to be escaped in the String method
			name: "struct tag in parameter type",
			files: map[string]string{
				"types.go": "package testpkg\n\nfunc Tagged(s struct{ X int `json:\"x\"` }) {}\n",
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var taggedWrapper = function.WrapperTODO(Tagged)
`},
			wantContains: []string{"return \"testpkg.Tagged(s struct{ X int `json:\\\"x\\\"` })\""},
		},
		{
			// String formats the signature like function.ReflectWrapper,
			// so both wrappers of a function describe it the same way:
			// one argument per name, the types of the package qualified
			// with its name, and the results without names
			name: "signature format of String",
			files: map[string]string{
				"types.go": "package testpkg\n\nimport \"io/fs\"\n\ntype Color int\n\nfunc Paint(a, b Color, f fs.File, names ...string) (n int, err error) { return 0, nil }\n",
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var paintWrapper = function.WrapperTODO(Paint)
`},
			wantContains: []string{`return "testpkg.Paint(a testpkg.Color, b testpkg.Color, f fs.File, names ...string) (int, error)"`},
		},
		{
			// text/template and html/template are both named template,
			// the import of the later wrapper needs another name
			name: "parameter types of different packages with the same name",
			files: map[string]string{
				"text.go": `package testpkg

import "text/template"

func Text(t *template.Template) {}
`,
				"html.go": `package testpkg

import "html/template"

func HTML(t *template.Template) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var textWrapper = function.WrapperTODO(Text)

var htmlWrapper = function.WrapperTODO(HTML)
`},
			wantContains: []string{"\t\"text/template\"\n", "template2 \"html/template\"", "reflect.TypeFor[*template2.Template]()"},
		},
		{
			// The generated code imports the standard library package context,
			// a context that is not the first argument is a normal argument
			name: "parameter type package named like package of generated code",
			files: map[string]string{
				"types.go": `package testpkg

import "golang.org/x/net/context"

func Deprecated(n int, ctx context.Context) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var deprecatedWrapper = function.WrapperTODO(Deprecated)
`},
			wantContains: []string{"context2 \"golang.org/x/net/context\"", "reflect.TypeFor[context2.Context]()"},
		},
		{
			// The generated code replaces a TODO declaration with its doc comment
			// and a wrapper spec of a var block with its trailing comment,
			// a kept comment would be above the generated comment
			// that the second run recognizes the generated wrapper by
			name: "TODO var with doc comment and trailing comment in var block",
			files: map[string]string{"test.go": `package testpkg

import "github.com/domonda/go-function"

func Mul(a, b int) int { return a * b }

// mulWrapper is documented by the user
var mulWrapper = function.WrapperTODO(Mul)

var (
	x          = 1
	mul2Wrapper = function.WrapperTODO(Mul) // trailing
)
`},
			wantContains:    []string{"// mulWrapper wraps Mul as function.Wrapper (generated code)", "var mul2Wrapper mul2WrapperT", "x = 1"},
			wantNotContains: []string{"documented by the user", "// trailing"},
		},
		{
			// golang.org/x/net/context.Context is an alias of context.Context,
			// so like ReflectWrapper the generated wrapper passes the context
			// of the Call methods instead of a nil context
			name: "first argument of type golang.org/x/net/context.Context",
			files: map[string]string{
				"types.go": `package testpkg

import "golang.org/x/net/context"

func Legacy(ctx context.Context, n int) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var legacyWrapper = function.WrapperTODO(Legacy)
`},
			wantContains: []string{"ContextArg() bool  { return true }", "Legacy(ctx, a.n)"},
		},
		{
			// The receiver f and the local variable a of the generated
			// methods would shadow the packages of the parameter types
			name: "parameter type packages named like generated identifiers",
			files: map[string]string{
				"types.go": `package testpkg

import (
	a "go/token"
	f "go/build/constraint"
)

func Shadowed(pos a.Pos, expr f.Expr) a.Pos { return pos }
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var shadowedWrapper = function.WrapperTODO(Shadowed)
`},
			wantContains: []string{"a2 \"go/token\"", "f2 \"go/build/constraint\"", "v, ok := args[0].(a2.Pos)"},
		},
		{
			// The generated methods declare v, str, i, ok, and raw only
			// in blocks that don't call the wrapped function, a only with
			// arguments besides the context, and ctx only for a context argument,
			// so they must not shadow the functions of the package with these names.
			// The context, error, replaced interface and variadic arguments
			// make every block that declares them be generated.
			name: "functions of the package named like identifiers of the generated code",
			files: map[string]string{
				"types.go": `package testpkg

import "context"

type Named interface{ Name() string }

type Person struct{ First string }

func (p *Person) Name() string { return p.First }

func v(ctx context.Context, e error, n Named, more ...Named) int { return 0 }

func str(ctx context.Context, e error, n Named, more ...Named) int { return 0 }

func i(ctx context.Context, e error, n Named, more ...Named) int { return 0 }

func ok(ctx context.Context, e error, n Named, more ...Named) int { return 0 }

func raw(ctx context.Context, e error, n Named, more ...Named) int { return 0 }

func a() {}

func ctx(n int) {}
`,
				"test.go": `package testpkg

import "github.com/domonda/go-function"

var vWrapper = function.WrapperTODO(v)

var strWrapper = function.WrapperTODO(str)

var iWrapper = function.WrapperTODO(i)

var okWrapper = function.WrapperTODO(ok)

var rawWrapper = function.WrapperTODO(raw)

var aWrapper = function.WrapperTODO(a)

var ctxWrapper = function.WrapperTODO(ctx)
`},
			jsonTypeReplacements: map[string]string{"Named": "*Person"},
			wantContains: []string{
				"\t\tv, ok := args[1].(Named)\n",
				"\t\tfor i, str := range strs[2:] {\n",
				"\tif raw, ok := args[\"n\"]; ok && string(raw) != \"null\" {\n\t\tvar v *Person\n",
				"\t\tfor i, raw := range v {\n",
				"results[0] = v(ctx, a.e, a.n, a.more...) // wrapped call",
				"results[0] = raw(ctx, a.e, a.n, a.more...) // wrapped call",
				"\ta() // wrapped call",
				"\tctx(a.n) // wrapped call",
			},
		},
		{
			name: "function of standard library package",
			files: map[string]string{"test.go": `package testpkg

import (
	"strings"

	"github.com/domonda/go-function"
)

var toUpperWrapper = function.WrapperTODO(strings.ToUpper)
`},
			wantContains: []string{"strings.ToUpper(a.s)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := maps.Clone(tt.files)
			files["funcs.go"] = funcs
			generated, err := generateWrappers(t, files, "test.go", tt.jsonTypeReplacements)
			require.NoError(t, err)
			assert.NotContains(t, generated, "TODO(", "all TODO calls should be replaced")
			for _, want := range tt.wantContains {
				assert.Contains(t, generated, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, generated, notWant)
			}
			requireValidGenerated(t, files, "test.go", generated, tt.jsonTypeReplacements)
		})
	}
}

// TestRewriteAstFileSource_ChangedSignature tests that imports that
// a regenerated wrapper doesn't use anymore are removed,
// because the generated code doesn't compile with unused imports.
func TestRewriteAstFileSource_ChangedSignature(t *testing.T) {
	files := map[string]string{
		"funcs.go": `package testpkg

import "go/token"

func F(pos token.Pos, n int) error { return nil }
`,
		"test.go": `package testpkg

import "github.com/domonda/go-function"

var fWrapper = function.WrapperTODO(F)
`,
	}
	generated, err := generateWrappers(t, files, "test.go", nil)
	require.NoError(t, err)
	require.Contains(t, generated, "\"go/token\"\n")
	requireValidGenerated(t, files, "test.go", generated, nil)

	// The signature shrinks, the generated code doesn't need go/token anymore
	files["funcs.go"] = "package testpkg\n\nfunc F() error { return nil }\n"
	files["test.go"] = generated
	regenerated, err := generateWrappers(t, files, "test.go", nil)
	require.NoError(t, err)
	assert.NotContains(t, regenerated, "\"go/token\"")
	requireValidGenerated(t, files, "test.go", regenerated, nil)
}

// TestRewriteAstFileSource_UnhandledDeclarations tests that wrapper
// declarations the generator can't rewrite are errors,
// because skipping them leaves a TODO call that panics at runtime
// while the generator and -validate succeed.
func TestRewriteAstFileSource_UnhandledDeclarations(t *testing.T) {
	const funcs = "package testpkg\n\nfunc Add(a, b int) int { return a + b }\n"
	tests := []struct {
		name                 string
		source               string
		jsonTypeReplacements map[string]string
		wantErr              string
	}{
		{
			name:    "several variables in one declaration",
			source:  "var a, b = function.WrapperTODO(Add), function.WrapperTODO(Add)\n",
			wantErr: "a, b",
		},
		{
			name:    "unknown TODO function",
			source:  "var a = function.UnknownTODO(Add)\n",
			wantErr: "UnknownTODO",
		},
		{
			// The receiver f of the generated methods would shadow go-function
			name:    "go-function imported with name of generated identifier",
			source:  "import f \"github.com/domonda/go-function\"\n\nvar a = f.WrapperTODO(Add)\n",
			wantErr: "identifier f",
		},
		{
			// The generated code needs encoding/json with the name json
			name:    "file uses name of package of generated code for other package",
			source:  "import json \"go/token\"\n\nvar _ json.Pos\n\nvar a = function.WrapperTODO(Add)\n",
			wantErr: "encoding/json",
		},
		{
			// The receiver f of the generated methods would shadow the wrapped function
			name:    "wrapped function named like generated identifier",
			source:  "func f(x int) int { return x }\n\nvar a = function.WrapperTODO(f)\n",
			wantErr: "identifier f",
		},
		{
			// The function would be declared in the package block
			// and the import of encoding/json in the file block
			name:    "wrapped function named like package of generated code",
			source:  "func json(n int) {}\n\nvar w = function.WrapperTODO(json)\n",
			wantErr: "encoding/json",
		},
		{
			// The local variable a of the generated methods would shadow the package
			name:    "package of wrapped function named like generated identifier",
			source:  "import a \"strings\"\n\nvar w = function.WrapperTODO(a.ToUpper)\n",
			wantErr: "identifier a",
		},
		{
			// Call declares v, ok := args[0].(v.Reader) and then refers to
			// the argument type v.Reader in the same block, where v is the
			// local variable, so the generated code wouldn't compile
			name:    "package of wrapped function named v",
			source:  "import v \"io\"\n\nvar w = function.WrapperTODO(v.ReadAll)\n",
			wantErr: "identifier v",
		},
		{
			name:    "package of wrapped function named ok",
			source:  "import ok \"io\"\n\nvar w = function.WrapperTODO(ok.ReadAll)\n",
			wantErr: "identifier ok",
		},
		{
			// CallWithJSON declares raw, ok := args["r"] in the block
			// that unmarshals into a replacement type of the package
			name:                 "package of wrapped function named raw",
			source:               "import raw \"io\"\n\nvar w = function.WrapperTODO(raw.ReadAll)\n",
			jsonTypeReplacements: map[string]string{"raw.Reader": "*raw.SectionReader"},
			wantErr:              "identifier raw",
		},
		{
			// Call declares v, ok := args[0].(v) and then refers to
			// the argument type v of the package in the same block,
			// where v is the local variable
			name:    "argument type of the package named like generated identifier",
			source:  "type v int\n\nfunc G(x v) {}\n\nvar w = function.WrapperTODO(G)\n",
			wantErr: "identifier v",
		},
		{
			// The argument types are used after the declaration of a
			// that holds the arguments, also as element type
			name:    "element type of argument named like generated identifier",
			source:  "type a struct{}\n\nfunc G(xs ...a) {}\n\nvar w = function.WrapperTODO(G)\n",
			wantErr: "identifier a",
		},
		{
			name:                 "JSON type replacement of unknown package",
			source:               "var a = function.WrapperTODO(Add)\n",
			jsonTypeReplacements: map[string]string{"int": "unknownpkg.Int"},
			wantErr:              "unknownpkg",
		},
		{
			name:    "TODO call without function",
			source:  "var a = function.WrapperTODO()\n",
			wantErr: "single argument",
		},
		{
			name:    "TODO call with two functions",
			source:  "var a = function.WrapperTODO(Add, Add)\n",
			wantErr: "single argument",
		},
		{
			name:    "wrapper declared twice",
			source:  "var a = function.WrapperTODO(Add)\n\nvar a = function.WrapperTODO(Add)\n",
			wantErr: "declared twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"funcs.go": funcs,
				"test.go":  "package testpkg\n\nimport \"github.com/domonda/go-function\"\n\n" + tt.source,
			}
			if strings.Contains(tt.source, "github.com/domonda/go-function") {
				files["test.go"] = "package testpkg\n\n" + tt.source
			}
			_, err := generateWrappers(t, files, "test.go", tt.jsonTypeReplacements)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "test.go", "error should name the file")
		})
	}
}

// TestRewriteAstFileSource_CallsLikeTODOCalls tests that calls whose
// function name only looks like a TODO function of go-function are left
// alone, because they are code of the user that rewriting would delete:
// with a dot import of go-function, a function of the package
// whose name ends with TODO, and a method WrapperTODO of a variable
// that is not the go-function package.
// A TODO call of go-function in the same file is still rewritten,
// so the file is processed, and its output must compile.
func TestRewriteAstFileSource_CallsLikeTODOCalls(t *testing.T) {
	files := map[string]string{
		"funcs.go": `package testpkg

func Add(a, b int) int { return a + b }

func SetupTODO(x int) int { return x }

type todos struct{}

func (todos) WrapperTODO(f func(a, b int) int) int { return f(1, 2) }

var other todos
`,
		"test.go": `package testpkg

import . "github.com/domonda/go-function"

var y = SetupTODO(1)

var z = other.WrapperTODO(Add)

var addWrapper = WrapperTODO(Add)
`,
	}
	generated, err := generateWrappers(t, files, "test.go", nil)
	require.NoError(t, err)
	assert.Contains(t, generated, "\nvar y = SetupTODO(1)\n", "function of the package should be left alone")
	assert.Contains(t, generated, "\nvar z = other.WrapperTODO(Add)\n", "method of a variable should be left alone")
	assert.Contains(t, generated, "\nvar addWrapper addWrapperT\n", "TODO call of go-function should be rewritten")
	requireValidGenerated(t, files, "test.go", generated, nil)
}

// TestRewriteAstFileSource_CallSemantics tests by running the generated code
// that the generated wrappers handle arguments like ReflectWrapper,
// because the generated wrappers replace ReflectWrapper at runtime:
// missing and untyped nil arguments are zero values, too many arguments
// and arguments of the wrong type are errors instead of panics,
// Call accepts values assignable to the argument type like Go does,
// arguments of type any get the unparsed strings,
// every string from the variadic argument on is one of its elements
// like in a Go call, also a single string, so that a Printf like function
// gets "hello" and not a parse error of "hello" as slice,
// and CallWithJSON matches the argument names exactly like ReflectWrapper,
// so that a differently cased or duplicate key can't change an argument
// that a check before the call has seen with its exact name.
// Call rejects a value that is only convertible to the argument type,
// because Go doesn't assign it either. The parse error of a variadic
// element names the variadic argument, because httpfun answers it
// with 400 Bad Request naming the argument. An empty or null JSON
// error argument is a nil error like with ReflectWrapper, and CallWithJSON
// returns an ErrParseArgsJSON for malformed JSON also for a function
// without arguments, instead of calling it like valid JSON.
func TestRewriteAstFileSource_CallSemantics(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the generated code with the go command")
	}
	files := map[string]string{
		"funcs.go": `package main

import (
	"context"
	"fmt"
)

func Pointers(ctx context.Context, p *int, m map[string]int, e error, x any, y interface{}) string {
	return fmt.Sprintf("p=%v m=%v e=%v x=%v y=%v", p, m, e, x, y)
}

func Strings(ctx context.Context, x any, y interface{}, n int) string {
	return fmt.Sprintf("x=%#v y=%#v n=%d", x, y, n)
}

func Variadic(ctx context.Context, n int, nums ...int) string {
	return fmt.Sprint(n, nums)
}

func VariadicStrings(strs ...string) string {
	return fmt.Sprintf("%q", strs)
}

func Echo(s string, more ...string) string {
	return fmt.Sprint(s, more)
}

func Printf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

type MyInt int

type MyInts []int

type OtherInts []int

func NoArgs() string { return "no args" }

func CtxOnly(ctx context.Context) string { return "ctx only" }

func Sum(xs MyInts) int {
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum
}

func SumSlice(xs []int) int {
	return Sum(xs)
}

func Pay(amount int) string {
	return fmt.Sprint("pay ", amount)
}

type Named interface{ Name() string }

type Person struct{ First string }

func (p *Person) Name() string { return p.First }

func Greet(n Named, more ...Named) string {
	if n == nil {
		return "nil"
	}
	names := n.Name()
	for _, m := range more {
		if m == nil {
			names += " nil"
			continue
		}
		names += " " + m.Name()
	}
	return names
}
`,
		// The context argument is detected by the import path of its type,
		// not by the name the file imports it with
		"aliased.go": `package main

import (
	"fmt"
	stdctx "context"
)

func CtxAlias(ctx stdctx.Context, n int) string {
	return fmt.Sprint(ctx != nil, n)
}
`,
		"wrappers.go": `package main

import "github.com/domonda/go-function"

var pointersWrapper = function.WrapperTODO(Pointers)

var stringsWrapper = function.WrapperTODO(Strings)

var variadicWrapper = function.WrapperTODO(Variadic)

var variadicStringsWrapper = function.WrapperTODO(VariadicStrings)

var echoWrapper = function.WrapperTODO(Echo)

var printfWrapper = function.WrapperTODO(Printf)

var sumWrapper = function.WrapperTODO(Sum)

var sumSliceWrapper = function.WrapperTODO(SumSlice)

var payWrapper = function.WrapperTODO(Pay)

var greetWrapper = function.WrapperTODO(Greet)

var noArgsWrapper = function.WrapperTODO(NoArgs)

var ctxOnlyWrapper = function.WrapperTODO(CtxOnly)

var ctxAliasWrapper = function.WrapperTODO(CtxAlias)
`,
	}
	jsonTypeReplacements := map[string]string{"Named": "*Person"}
	generated, err := generateWrappers(t, files, "wrappers.go", jsonTypeReplacements)
	require.NoError(t, err)
	files["wrappers.go"] = generated
	files["main.go"] = `package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/domonda/go-function"
)

func main() {
	ctx := context.Background()
	call := func(name string, f func() ([]any, error)) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("%s: panic\n", name)
			}
		}()
		results, err := f()
		fmt.Printf("%s: %v error=%t\n", name, results, err != nil)
	}
	// callErr prints the type of the argument parsing error
	callErr := func(name string, f func() ([]any, error)) {
		_, err := f()
		var errString function.ErrParseArgString
		var errArg function.ErrParseArgJSON
		var errArgs function.ErrParseArgsJSON
		switch {
		case errors.As(err, &errString):
			fmt.Printf("%s: ErrParseArgString of argument %s\n", name, errString.ArgName)
		case errors.As(err, &errArg):
			fmt.Printf("%s: ErrParseArgJSON of argument %s\n", name, errArg.ArgName)
		case errors.As(err, &errArgs):
			fmt.Printf("%s: ErrParseArgsJSON\n", name)
		default:
			fmt.Printf("%s: %v\n", name, err)
		}
	}
	call("untyped nil", func() ([]any, error) { return pointersWrapper.Call(ctx, []any{nil, nil, nil, nil, nil}) })
	call("missing", func() ([]any, error) { return pointersWrapper.Call(ctx, nil) })
	call("too many", func() ([]any, error) { return pointersWrapper.Call(ctx, []any{nil, nil, nil, nil, nil, nil}) })
	call("wrong type", func() ([]any, error) { return pointersWrapper.Call(ctx, []any{"wrong"}) })
	call("assignable unnamed to named", func() ([]any, error) { return sumWrapper.Call(ctx, []any{[]int{1, 2}}) })
	call("assignable named to unnamed", func() ([]any, error) { return sumSliceWrapper.Call(ctx, []any{MyInts{1, 2}}) })
	call("not assignable to named", func() ([]any, error) { return sumWrapper.Call(ctx, []any{"wrong"}) })
	call("convertible but not assignable to named", func() ([]any, error) { return sumWrapper.Call(ctx, []any{OtherInts{1, 2}}) })
	call("not assignable to basic", func() ([]any, error) { return variadicWrapper.Call(ctx, []any{MyInt(1)}) })
	call("strings", func() ([]any, error) { return stringsWrapper.CallWithStrings(ctx, "raw", "raw2", "3") })
	call("named strings", func() ([]any, error) {
		return stringsWrapper.CallWithNamedStrings(ctx, map[string]string{"x": "raw", "y": "raw2", "n": "3"})
	})
	call("too many strings", func() ([]any, error) { return stringsWrapper.CallWithStrings(ctx, "raw", "raw2", "3", "extra") })
	call("variadic without strings", func() ([]any, error) { return variadicWrapper.CallWithStrings(ctx, "1") })
	call("variadic single string", func() ([]any, error) { return variadicWrapper.CallWithStrings(ctx, "1", "2") })
	call("variadic single string is no slice", func() ([]any, error) { return variadicWrapper.CallWithStrings(ctx, "1", "[2,3]") })
	call("variadic strings", func() ([]any, error) { return variadicWrapper.CallWithStrings(ctx, "1", "2", "3", "4") })
	callErr("variadic parse error", func() ([]any, error) { return variadicWrapper.CallWithStrings(ctx, "1", "2", "x") })
	call("variadic named string is slice", func() ([]any, error) {
		return variadicWrapper.CallWithNamedStrings(ctx, map[string]string{"n": "1", "nums": "[2,3]"})
	})
	call("variadic string elements", func() ([]any, error) { return variadicStringsWrapper.CallWithStrings(ctx, "a", "b") })
	call("variadic single string element", func() ([]any, error) { return variadicStringsWrapper.CallWithStrings(ctx, "[WARN]") })
	call("variadic any", func() ([]any, error) { return printfWrapper.CallWithStrings(ctx, "%v", "hello") })
	call("JSON exact key", func() ([]any, error) { return payWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"amount":7}` + "`" + `)) })
	call("JSON duplicate key in other case", func() ([]any, error) {
		return payWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"amount":1,"AMOUNT":1000000}` + "`" + `))
	})
	call("JSON key in other case", func() ([]any, error) { return payWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"Amount":7}` + "`" + `)) })
	call("JSON error argument", func() ([]any, error) { return pointersWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"e":"boom"}` + "`" + `)) })
	call("JSON empty error argument", func() ([]any, error) { return pointersWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"e":""}` + "`" + `)) })
	call("JSON null error argument", func() ([]any, error) { return pointersWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"e":null}` + "`" + `)) })
	call("JSON type replacement", func() ([]any, error) {
		return greetWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"n":{"First":"a"},"more":[{"First":"b"},{"First":"c"}]}` + "`" + `))
	})
	call("JSON type replacement missing", func() ([]any, error) { return greetWrapper.CallWithJSON(ctx, []byte(` + "`" + `{}` + "`" + `)) })
	// JSON null leaves the argument nil, not a nil *Person in the interface
	call("JSON type replacement null", func() ([]any, error) {
		return greetWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"n":null}` + "`" + `))
	})
	call("JSON type replacement null element", func() ([]any, error) {
		return greetWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"n":{"First":"a"},"more":[null,{"First":"c"}]}` + "`" + `))
	})
	call("aliased context", func() ([]any, error) { return ctxAliasWrapper.Call(ctx, []any{5}) })
	call("aliased context strings", func() ([]any, error) { return ctxAliasWrapper.CallWithStrings(ctx, "5") })
	callErr("JSON malformed", func() ([]any, error) { return payWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"amount":` + "`" + `)) })
	callErr("JSON bad argument", func() ([]any, error) { return payWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"amount":"x"}` + "`" + `)) })
	callErr("JSON bad error argument", func() ([]any, error) { return pointersWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"e":1}` + "`" + `)) })
	call("JSON object without arguments", func() ([]any, error) { return noArgsWrapper.CallWithJSON(ctx, []byte(` + "`" + `{}` + "`" + `)) })
	callErr("JSON malformed without arguments", func() ([]any, error) { return noArgsWrapper.CallWithJSON(ctx, []byte(` + "`" + `{` + "`" + `)) })
	callErr("JSON malformed with only context argument", func() ([]any, error) { return ctxOnlyWrapper.CallWithJSON(ctx, []byte(` + "`" + `{"ctx":` + "`" + `)) })
	// Variadic distinguishes ...int from []int, which have the same ArgTypes
	var variadic, slice, noArgs function.Wrapper = variadicWrapper, sumSliceWrapper, noArgsWrapper
	fmt.Printf("Variadic: %t %t %t\n", variadic.Variadic(), slice.Variadic(), noArgs.Variadic())
	// Name and String are the same as of ReflectWrapper,
	// also for empty interfaces written as interface{} in the source code
	reflected := function.MustReflectWrapper(Pointers, "ctx", "p", "m", "e", "x", "y")
	fmt.Printf("%s %t %t\n", pointersWrapper, pointersWrapper.Name() == reflected.Name(), pointersWrapper.String() == reflected.String())
	// Like ReflectWrapper, generated wrappers scan string arguments
	// and variadic elements, so that scanners registered for strings apply
	function.StringScanners = function.StringScanners.WithTypeScanner(
		reflect.TypeFor[string](),
		function.StringScannerFunc(func(sourceStr string, destPtr any) error {
			*destPtr.(*string) = strings.ToUpper(sourceStr)
			return nil
		}),
	)
	reflectedEcho := function.MustReflectWrapper(Echo, "s", "more")
	call("scanned strings", func() ([]any, error) { return echoWrapper.CallWithStrings(ctx, "a", "b") })
	call("reflected scanned strings", func() ([]any, error) { return reflectedEcho.CallWithStrings(ctx, "a", "b") })
	call("scanned named strings", func() ([]any, error) {
		return echoWrapper.CallWithNamedStrings(ctx, map[string]string{"s": "a", "more": "[b]"})
	})
	call("reflected scanned named strings", func() ([]any, error) {
		return reflectedEcho.CallWithNamedStrings(ctx, map[string]string{"s": "a", "more": "[b]"})
	})
}
`
	output := runPackage(t, files)
	assert.Equal(t, `untyped nil: [p=<nil> m=map[] e=<nil> x=<nil> y=<nil>] error=false
missing: [p=<nil> m=map[] e=<nil> x=<nil> y=<nil>] error=false
too many: [] error=true
wrong type: [] error=true
assignable unnamed to named: [3] error=false
assignable named to unnamed: [3] error=false
not assignable to named: [] error=true
convertible but not assignable to named: [] error=true
not assignable to basic: [] error=true
strings: [x="raw" y="raw2" n=3] error=false
named strings: [x="raw" y="raw2" n=3] error=false
too many strings: [] error=true
variadic without strings: [1 []] error=false
variadic single string: [1 [2]] error=false
variadic single string is no slice: [] error=true
variadic strings: [1 [2 3 4]] error=false
variadic parse error: ErrParseArgString of argument nums
variadic named string is slice: [1 [2 3]] error=false
variadic string elements: [["a" "b"]] error=false
variadic single string element: [["[WARN]"]] error=false
variadic any: [hello] error=false
JSON exact key: [pay 7] error=false
JSON duplicate key in other case: [pay 1] error=false
JSON key in other case: [pay 0] error=false
JSON error argument: [p=<nil> m=map[] e=boom x=<nil> y=<nil>] error=false
JSON empty error argument: [p=<nil> m=map[] e=<nil> x=<nil> y=<nil>] error=false
JSON null error argument: [p=<nil> m=map[] e=<nil> x=<nil> y=<nil>] error=false
JSON type replacement: [a b c] error=false
JSON type replacement missing: [nil] error=false
JSON type replacement null: [nil] error=false
JSON type replacement null element: [a nil c] error=false
aliased context: [true 5] error=false
aliased context strings: [true 5] error=false
JSON malformed: ErrParseArgsJSON
JSON bad argument: ErrParseArgJSON of argument amount
JSON bad error argument: ErrParseArgJSON of argument e
JSON object without arguments: [no args] error=false
JSON malformed without arguments: ErrParseArgsJSON
JSON malformed with only context argument: ErrParseArgsJSON
Variadic: true false false
main.Pointers(ctx context.Context, p *int, m map[string]int, e error, x any, y any) string true true
scanned strings: [A[B]] error=false
reflected scanned strings: [A[B]] error=false
scanned named strings: [A[B]] error=false
reflected scanned named strings: [A[B]] error=false
`, output)
}

// runPackage runs the main package with the in-memory files
// and returns its output. The files are overlaid into a directory
// of this module that doesn't exist on disk, so the go command
// resolves their imports through go.work without changing the repository.
func runPackage(t *testing.T, files map[string]string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	pkgDir := filepath.Join(cwd, "overlaid_main_package_of_test")
	overlay := struct{ Replace map[string]string }{Replace: make(map[string]string)}
	tempDir := t.TempDir()
	for name, source := range files {
		err = os.WriteFile(filepath.Join(tempDir, name), []byte(source), 0600)
		require.NoError(t, err)
		overlay.Replace[filepath.Join(pkgDir, name)] = filepath.Join(tempDir, name)
	}
	overlayJSON, err := json.Marshal(overlay)
	require.NoError(t, err)
	overlayFile := filepath.Join(tempDir, "overlay.json")
	err = os.WriteFile(overlayFile, overlayJSON, 0600)
	require.NoError(t, err)

	// The directory of the package doesn't exist, so run the go command in this one
	cmd := exec.Command("go", "run", "-overlay="+overlayFile, "./"+filepath.Base(pkgDir))
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go run failed:\n%s", output)
	return string(output)
}

// writeFiles writes the files with their contents by path relative to dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		err := os.MkdirAll(filepath.Dir(path), 0700)
		require.NoError(t, err)
		err = os.WriteFile(path, []byte(content), 0600)
		require.NoError(t, err)
	}
}

// TestRewriteDir_SortedFiles tests that the files of a package
// are rewritten in sorted order, so that the first error
// that stops the generator is reported deterministically.
func TestRewriteDir_SortedFiles(t *testing.T) {
	dir := t.TempDir()
	todo := "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar %[1]sWrapper = function.WrapperTODO(%[1]s)\n\nfunc %[1]s() {}\n"
	writeFiles(t, dir, map[string]string{
		"a.go": fmt.Sprintf(todo, "A"),
		"b.go": fmt.Sprintf(todo, "B"),
		"c.go": fmt.Sprintf(todo, "C"),
	})
	// Map iteration order is random, so validate several times
	for range 10 {
		err := RewriteDir(dir, false, nil, true, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join(dir, "a.go"), "the first file should be reported")
	}
}

// TestRewriteDir_SkipsDirsLikeGoCommand tests that recursive rewriting
// skips the directories that the go command ignores for ./... patterns,
// because their files are not part of the module's packages:
// vendored copies, test data, directories starting with _,
// and nested modules with their own go.mod file.
func TestRewriteDir_SkipsDirsLikeGoCommand(t *testing.T) {
	root := t.TempDir()
	outdated := "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n"
	writeFiles(t, root, map[string]string{
		"go.mod":               "module example.com/root\n\ngo 1.22\n",
		"root.go":              "package root\n",
		"vendor/v/v.go":        outdated,
		"testdata/td.go":       outdated,
		"_ignored/i.go":        outdated,
		"nested/go.mod":        "module example.com/nested\n\ngo 1.22\n",
		"nested/n.go":          outdated,
		"nested/sub/nested.go": outdated,
	})
	err := RewriteDir(filepath.Join(root, "..."), false, nil, true, nil, nil)
	require.NoError(t, err, "validation should not check skipped directories")

	// Other sub-directories are still rewritten recursively
	writeFiles(t, root, map[string]string{"sub/deeper/s.go": outdated})
	err = RewriteDir(filepath.Join(root, "..."), false, nil, true, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(root, "sub", "deeper", "s.go"))
}

// writeModule writes the files of a module example.com/m to a temporary
// directory, so that packages can be located with the go command
// that are not part of this repository.
// The module doesn't require go-function, so the generator must not
// locate the go-function package to rewrite its files.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files = maps.Clone(files)
	files["go.mod"] = "module example.com/m\n\ngo 1.22\n"
	writeFiles(t, dir, files)
	return dir
}

// TestRewriteFile_PackageNamedUnlikeDir tests that the package name
// of a parameter type is resolved instead of guessed from its import path,
// because the generated code refers to the package by its name.
func TestRewriteFile_PackageNamedUnlikeDir(t *testing.T) {
	const (
		utilsPkg = "package util\n\ntype T struct{}\n"
		funcs    = "package m\n\nimport \"example.com/m/utils\"\n\nfunc F(t util.T) {}\n"
	)
	t.Run("import is added", func(t *testing.T) {
		dir := writeModule(t, map[string]string{
			"utils/util.go": utilsPkg,
			"funcs.go":      funcs,
			"wrappers.go":   "package m\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n",
		})
		var generated bytes.Buffer
		err := RewriteFile(filepath.Join(dir, "wrappers.go"), false, &generated, false, nil, nil)
		require.NoError(t, err)
		assert.Contains(t, generated.String(), "\t\"example.com/m/utils\"\n")
		assert.Contains(t, generated.String(), "reflect.TypeFor[util.T]()")
	})
	t.Run("import of rewritten file is used", func(t *testing.T) {
		// The alias doesn't tell the package name that the rewritten file uses
		dir := writeModule(t, map[string]string{
			"utils/util.go": utilsPkg,
			"funcs.go":      "package m\n\nimport u \"example.com/m/utils\"\n\nfunc F(t u.T) {}\n",
			"wrappers.go":   "package m\n\nimport (\n\t\"example.com/m/utils\"\n\t\"github.com/domonda/go-function\"\n)\n\nvar _ util.T\n\nvar fWrapper = function.WrapperTODO(F)\n",
		})
		var generated bytes.Buffer
		err := RewriteFile(filepath.Join(dir, "wrappers.go"), false, &generated, false, nil, nil)
		require.NoError(t, err)
		assert.Contains(t, generated.String(), "reflect.TypeFor[util.T]()")
		assert.NotContains(t, generated.String(), "utils.T")
		assert.NotContains(t, generated.String(), ".(u.T)", "Call should use the name of the rewritten file")
		assert.NotContains(t, generated.String(), " u.T\n", "struct fields should use the name of the rewritten file")
	})
}

// importerFunc implements types.Importer with a function.
type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// TestRewriteFile_PackagesNamedUnlikeImportPath tests that the generated
// code refers to the packages of parameter types by their package names
// rand and yaml, not by the last elements v2 and yaml.v3 of their
// import paths, and imports them without alias, because otherwise
// the generated code doesn't compile, and that a second run doesn't
// change the generated code, because CI runs -validate on the generated files.
// The package yaml.v3 is part of the module that the test writes,
// so it doesn't depend on the modules that the go.work of this repository resolves.
func TestRewriteFile_PackagesNamedUnlikeImportPath(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"yaml.v3/yaml.go": "package yaml\n\ntype Node struct{}\n",
		"types.go": `package m

import (
	"math/rand/v2"

	"example.com/m/yaml.v3"
)

func Random(r *rand.Rand, n *yaml.Node) {}
`,
		"wrappers.go": "package m\n\nimport \"github.com/domonda/go-function\"\n\nvar randomWrapper = function.WrapperTODO(Random)\n",
	})
	wrappersFile := filepath.Join(dir, "wrappers.go")
	err := RewriteFile(wrappersFile, false, nil, false, nil, nil)
	require.NoError(t, err)
	generated, err := os.ReadFile(wrappersFile)
	require.NoError(t, err)
	assert.Contains(t, string(generated), "\t\"math/rand/v2\"\n")
	assert.Contains(t, string(generated), "\t\"example.com/m/yaml.v3\"\n")
	assert.Contains(t, string(generated), "reflect.TypeFor[*rand.Rand]()")
	assert.Contains(t, string(generated), "reflect.TypeFor[*yaml.Node]()")

	fset := token.NewFileSet()
	pkg, err := astvisit.ParsePackage(fset, dir, filterOutTests)
	require.NoError(t, err)
	// The source importer resolves imports with the go command
	// in the directory of this test package, which works for the standard
	// library and go-function, but not for the package of the module.
	// That package has no imports, so it's type checked without importer.
	const yamlPath = "example.com/m/yaml.v3"
	conf := types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		if path != yamlPath {
			return sourceImporter.Import(path)
		}
		yamlPkg, err := astvisit.ParsePackage(fset, filepath.Join(dir, "yaml.v3"), nil)
		if err != nil {
			return nil, err
		}
		return new(types.Config).Check(yamlPath, fset, slices.Collect(maps.Values(yamlPkg.Files)), nil)
	})}
	_, err = conf.Check(pkg.Name, fset, slices.Collect(maps.Values(pkg.Files)), nil)
	require.NoError(t, err, "generated code should compile:\n%s", generated)

	err = RewriteFile(wrappersFile, false, nil, true, nil, nil)
	assert.NoError(t, err, "a second run should not change the generated code")
}

// TestRewriteDir_EntersModulesBelowNonModuleDir tests that recursive
// rewriting enters the modules below a directory that is not within
// a module, like the go command does for the modules of a go.work
// checked out side by side, but still skips the modules nested
// in such a module, because they are not part of its packages.
func TestRewriteDir_EntersModulesBelowNonModuleDir(t *testing.T) {
	root := t.TempDir()
	inModule, err := isWithinModule(root)
	require.NoError(t, err)
	require.False(t, inModule, "the temporary directory %s must not be within a module for this test", root)
	outdated := "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n"
	writeFiles(t, root, map[string]string{
		"a/go.mod":        "module example.com/a\n\ngo 1.22\n",
		"a/a.go":          outdated,
		"a/nested/go.mod": "module example.com/a/nested\n\ngo 1.22\n",
		"a/nested/n.go":   outdated,
	})
	err = RewriteDir(filepath.Join(root, "..."), false, nil, true, nil, nil)
	require.Error(t, err, "the module below the directory outside of any module should be validated")
	assert.Contains(t, err.Error(), filepath.Join(root, "a", "a.go"))

	err = RewriteDir(filepath.Join(root, "..."), false, nil, false, nil, nil)
	require.NoError(t, err)
	source, err := os.ReadFile(filepath.Join(root, "a", "a.go"))
	require.NoError(t, err)
	assert.Contains(t, string(source), "var fWrapper fWrapperT", "file of the module should be rewritten")
	source, err = os.ReadFile(filepath.Join(root, "a", "nested", "n.go"))
	require.NoError(t, err)
	assert.Equal(t, outdated, string(source), "file of the module nested in a module should not be rewritten")

	err = RewriteDir(filepath.Join(root, "..."), false, nil, true, nil, nil)
	assert.NoError(t, err, "validation should not check the module nested in a module")
}

// TestRewriteAstFileSource_KeepsImportsOfOtherCode tests that removing
// the imports that the regenerated wrappers don't use anymore keeps
// a blank import and an import whose package can't be located,
// because the generator can't tell if other code of the file needs them:
// a blank import like embed for //go:embed is used without name,
// and the package of an import that go list can't resolve,
// like one for another platform, can be named unlike its import path.
func TestRewriteAstFileSource_KeepsImportsOfOtherCode(t *testing.T) {
	files := map[string]string{
		"funcs.go": "package testpkg\n\nimport \"go/token\"\n\nfunc F(pos token.Pos) {}\n",
		"test.go": `package testpkg

import (
	_ "embed"

	"example.com/does/not/exist/yaml.v3"
	"github.com/domonda/go-function"
)

//go:embed funcs.go
var funcsSource string

var _ yaml.Node

var fWrapper = function.WrapperTODO(F)
`,
	}
	generated, err := generateWrappers(t, files, "test.go", nil)
	require.NoError(t, err)
	require.Contains(t, generated, "\t\"go/token\"\n")

	// The signature shrinks, the generated code doesn't need go/token anymore
	files["funcs.go"] = "package testpkg\n\nfunc F() {}\n"
	files["test.go"] = generated
	regenerated, err := generateWrappers(t, files, "test.go", nil)
	require.NoError(t, err)
	assert.NotContains(t, regenerated, "\"go/token\"", "import of the previously generated code should be removed")
	assert.Contains(t, regenerated, "\t_ \"embed\"\n", "blank import should be kept")
	assert.Contains(t, regenerated, "\t\"example.com/does/not/exist/yaml.v3\"\n", "import that can't be located should be kept")
}

// TestRewriteFile_DotImportOfWrappedFunc tests that wrapping a function
// of another package whose file dot imports the types of its parameters
// is an error, because the generated code would qualify the types
// with the name of the wrapped function's package and not compile.
func TestRewriteFile_DotImportOfWrappedFunc(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"lib/lib.go":  "package lib\n\nimport . \"go/token\"\n\nfunc F(p Pos) {}\n",
		"wrappers.go": "package m\n\nimport (\n\t\"example.com/m/lib\"\n\t\"github.com/domonda/go-function\"\n)\n\nvar fWrapper = function.WrapperTODO(lib.F)\n",
	})
	err := RewriteFile(filepath.Join(dir, "wrappers.go"), false, io.Discard, false, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lib.F")
	assert.Contains(t, err.Error(), "dot import")
}

// TestREADMETestingExample tests that the call of RewriteAstFileSource
// in the Testing section of the README passes all its arguments,
// because the example is copied by readers.
func TestREADMETestingExample(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err)
	_, section, ok := strings.Cut(string(readme), "\n## Testing\n")
	require.True(t, ok, "README should have a Testing section")
	_, code, ok := strings.Cut(section, "```go\n")
	require.True(t, ok, "Testing section should have a Go code block")
	code, _, ok = strings.Cut(code, "```")
	require.True(t, ok)

	stmts, _, err := astvisit.ParseStatements(token.NewFileSet(), code)
	require.NoError(t, err, "example should be valid Go")
	var call *ast.CallExpr
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(node ast.Node) bool {
			if c, ok := node.(*ast.CallExpr); ok && astvisit.ExprString(c.Fun) == "gen.RewriteAstFileSource" {
				call = c
			}
			return true
		})
	}
	require.NotNil(t, call, "example should call gen.RewriteAstFileSource")
	assert.Len(t, call.Args, reflect.TypeOf(RewriteAstFileSource).NumIn(), "example should pass every argument")
}

// TestRewriteDir_FilesLikeGoCommand tests that a directory is rewritten
// with the files that the go command builds for the package,
// because files excluded by custom build tags or names starting with _
// may contain TODO calls that are not meant to be generated,
// and that the files of a main package are rewritten.
func TestRewriteDir_FilesLikeGoCommand(t *testing.T) {
	todo := "package main\n\nimport \"github.com/domonda/go-function\"\n\nvar %sWrapper = function.WrapperTODO(F)\n"
	excluded := map[string]string{
		"wrappers_integration.go": "//go:build integration\n\n" + fmt.Sprintf(todo, "integration"),
		"_scratch.go":             fmt.Sprintf(todo, "scratch"),
	}
	files := map[string]string{
		"main.go":     "package main\n\nfunc F() {}\n\nfunc main() {}\n",
		"wrappers.go": fmt.Sprintf(todo, "f"),
	}
	maps.Copy(files, excluded)
	dir := writeModule(t, files)

	err := RewriteDir(dir, false, nil, false, nil, nil)
	require.NoError(t, err)

	source, err := os.ReadFile(filepath.Join(dir, "wrappers.go"))
	require.NoError(t, err)
	assert.Contains(t, string(source), "var fWrapper fWrapperT", "file of main package should be rewritten")
	for name, want := range excluded {
		source, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, want, string(source), "%s is not part of the build and should be unchanged", name)
	}

	err = RewriteDir(dir, false, nil, true, nil, nil)
	assert.NoError(t, err, "validation should not check files that are not part of the build")
}
