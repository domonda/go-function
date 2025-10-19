package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		false, // verbose
		&output,
		nil, // no JSON type replacements
		nil, // no local import prefixes
	)
	require.NoError(t, err)

	// Verify the output contains generated wrapper code
	result := output.String()
	assert.Contains(t, result, "// simpleAddWrapper wraps SimpleAdd as function.Wrapper (generated code)")
	assert.Contains(t, result, "var simpleAddWrapper simpleAddWrapperT")
	assert.Contains(t, result, "type simpleAddWrapperT struct{}")
	assert.Contains(t, result, "func (simpleAddWrapperT) Name() string")
	assert.Contains(t, result, "func (simpleAddWrapperT) Call")
	assert.Contains(t, result, "results[0] = SimpleAdd(args[0].(int), args[1].(int))")
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
		fset,              // Token file set
		"example",         // Package name
		pkgFiles,          // All package files
		astFile,           // The AST file to process
		"example.go",      // File path (for error messages only)
		source,            // Original source code
		false,             // verbose
		&output,           // Write output here instead of to disk
		nil,               // No JSON type replacements
		nil,               // No local import prefixes
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
	// 	return "Add(a, b int) int"
	// }
	//
	// func (addWrapperT) Name() string {
	// 	return "Add"
	// }
	//
	// func (addWrapperT) NumArgs() int      { return 2 }
	// func (addWrapperT) ContextArg() bool  { return false }
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
	// func (addWrapperT) Call(_ context.Context, args []any) (results []any, err error) {
	// 	results = make([]any, 1)
	// 	results[0] = Add(args[0].(int), args[1].(int)) // wrapped call
	// 	return results, err
	// }
	//
	// func (f addWrapperT) CallWithStrings(_ context.Context, strs ...string) (results []any, err error) {
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
	// 	var a struct {
	// 		A int
	// 		B int
	// 	}
	// 	err = json.Unmarshal(argsJSON, &a)
	// 	if err != nil {
	// 		return nil, function.NewErrParseArgsJSON(err, f, argsJSON)
	// 	}
	// 	results = make([]any, 1)
	// 	results[0] = Add(a.A, a.B) // wrapped call
	// 	return results, err
	// }
}
