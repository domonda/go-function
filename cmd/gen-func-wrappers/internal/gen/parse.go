package gen

import (
	"go/ast"
	"os"
	"strings"
)

// funcDeclInFile associates a function declaration with its containing file.
// This is needed because imports and other context are file-specific.
type funcDeclInFile struct {
	Decl *ast.FuncDecl
	File *ast.File
}

// filterOutTests creates a file filter that excludes all test files.
// This is used when parsing packages for function declarations.
//
// Returns:
//   - true if the file should be included (not a test file)
//   - false if the file should be excluded (is a test file)
func filterOutTests(info os.FileInfo) bool {
	if strings.HasSuffix(info.Name(), "_test.go") {
		return false
	}
	return true
}
