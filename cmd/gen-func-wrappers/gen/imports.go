package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// packageFuncs holds function declarations for a package along with package location info.
type packageFuncs struct {
	Location *astvisit.PackageLocation  // Package location and metadata
	Funcs    map[string]funcDeclInFile  // Map of function name to declaration
}

// localAndImportedFunctions builds a complete map of all functions available to a file.
// This includes both local package functions and exported functions from imported packages.
//
// Parameters:
//   - fset: Token file set for parsing
//   - filePkg: The package containing the file
//   - file: The specific file to analyze
//   - pkgDir: Directory path of the package
//
// Returns:
//   - Map with package names as keys:
//   - "" (empty string): Functions from the same package
//   - "pkg": Functions from imported package "pkg"
//   - error if package parsing or import resolution fails
//
// The function:
//  1. Collects all functions from the local package (key: "")
//  2. For each import in the file:
//     a. Locates the imported package's source
//     b. Parses the package (skipping standard library)
//     c. Collects all exported functions
//     d. Adds to map with import name as key
//
// This is used by the code generator to find the wrapped function's declaration
// when it's referenced as "pkg.FuncName" or just "FuncName".
func localAndImportedFunctions(fset *token.FileSet, filePkg *ast.Package, file *ast.File, pkgDir string) (map[string]packageFuncs, error) {
	localFuncs := make(map[string]funcDeclInFile)
	for _, f := range filePkg.Files {
		for _, decl := range f.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if ok && funcDecl.Recv == nil {
				localFuncs[funcDecl.Name.Name] = funcDeclInFile{
					Decl: funcDecl,
					File: f,
				}
			}
		}
	}
	functions := map[string]packageFuncs{
		"": {
			Location: &astvisit.PackageLocation{
				PkgName:    filePkg.Name,
				SourcePath: pkgDir,
			},
			Funcs: localFuncs,
		},
	}

	for _, imp := range file.Imports {
		importName, pkgLocation, err := astvisit.LocatePackageOfImportSpec(pkgDir, imp)
		if err != nil {
			return nil, err
		}
		if pkgLocation.Std {
			continue
		}
		impPkg, err := astvisit.ParsePackage(fset, pkgLocation.SourcePath, filterOutTests)
		if err != nil {
			return nil, err
		}
		exportedFuncs := make(map[string]funcDeclInFile)
		for _, f := range impPkg.Files {
			for _, decl := range f.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if ok && funcDecl.Recv == nil && funcDecl.Name.IsExported() {
					exportedFuncs[funcDecl.Name.Name] = funcDeclInFile{
						Decl: funcDecl,
						File: f,
					}
				}
			}
		}
		functions[importName] = packageFuncs{
			Location: pkgLocation,
			Funcs:    exportedFuncs,
		}
	}

	return functions, nil
}

// gatherFieldListImports collects import statements needed for types in a field list.
// This is used to ensure the generated wrapper code has all necessary imports.
//
// Parameters:
//   - funcFile: The file containing the function (source of import information)
//   - fieldList: AST field list containing parameters or results
//   - setImportLines: Map to add required import lines to (modified in-place)
//
// Returns:
//   - error if package name cannot be guessed from import path
//
// The function:
//  1. Extracts all package qualifiers used in the field types (e.g., "context" from "context.Context")
//  2. Matches qualifiers against the file's imports
//  3. Adds matching imports to setImportLines in proper format:
//     - `"path/to/package"` for imports without aliases
//     - `alias "path/to/package"` for aliased imports
//
// Example:
//
//	Field list: (ctx context.Context, r io.Reader)
//	funcFile imports: import "context"; import "io"
//	Result: setImportLines gets `"context"` and `"io"` added
func gatherFieldListImports(funcFile *ast.File, fieldList *ast.FieldList, setImportLines map[string]struct{}) error {
	if fieldList == nil {
		return nil
	}
	packageNames := make(map[string]struct{})
	for _, field := range fieldList.List {
		astvisit.TypeExprNameQualifyers(field.Type, packageNames)
	}
	for _, imp := range funcFile.Imports {
		if imp.Name != nil {
			if _, ok := packageNames[imp.Name.Name]; ok {
				delete(setImportLines, imp.Path.Value)
				setImportLines[imp.Name.Name+" "+imp.Path.Value] = struct{}{}
			}
			continue
		}
		guessedName, err := guessPackageNameFromPath(imp.Path.Value)
		if err != nil {
			return err
		}
		if _, ok := packageNames[guessedName]; ok {
			if _, ok = setImportLines[guessedName+" "+imp.Path.Value]; !ok {
				setImportLines[imp.Path.Value] = struct{}{}
			}
		}
	}
	return nil
}

// guessPackageNameFromPath attempts to guess a package's name from its import path.
// This is needed when imports don't have explicit aliases.
//
// Parameters:
//   - path: Import path (may include surrounding quotes)
//
// Returns:
//   - Guessed package name (last path component with common prefixes/suffixes removed)
//   - error if package name cannot be determined
//
// The heuristics:
//  1. Remove surrounding quotes if present
//  2. Take the last path component after "/"
//  3. Remove "go-" prefix (common Go package naming convention)
//  4. Remove ".go" suffix (edge case)
//  5. Validate result doesn't contain "." or "-"
//
// Examples:
//   - `"github.com/user/mypackage"` -> "mypackage"
//   - `"github.com/user/go-types"` -> "types"
//   - `"io"` -> "io"
//   - `"github.com/user/my-pkg"` -> error (contains dash)
func guessPackageNameFromPath(path string) (string, error) {
	pkg := path
	if len(pkg) >= 2 && pkg[0] == '"' && pkg[len(pkg)-1] == '"' {
		pkg = pkg[1 : len(pkg)-1]
	}
	pkg = pkg[strings.LastIndex(pkg, "/")+1:]
	pkg = strings.TrimPrefix(pkg, "go-")
	pkg = strings.TrimSuffix(pkg, ".go")
	if pkg == "" || strings.ContainsAny(pkg, ".-") {
		return "", fmt.Errorf("could not guess package name from import path %s", path)
	}
	return pkg, nil
}
