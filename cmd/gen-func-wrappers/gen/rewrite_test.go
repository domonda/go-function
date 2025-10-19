package gen

import (
	"bytes"
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
