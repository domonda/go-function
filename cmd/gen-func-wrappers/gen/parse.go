package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// funcDeclInFile associates a function declaration with its containing file.
// This is needed because imports and other context are file-specific.
type funcDeclInFile struct {
	Decl *ast.FuncDecl
	File *ast.File
}

// parsePackage parses all Go files in a directory and extracts function declarations.
//
// Parameters:
//   - pkgDir: Directory containing the package source files
//   - excludeFilename: Name of file to skip (typically the generated output file)
//   - onlyFuncs: Optional list of specific function names to parse; if empty, all exported functions are parsed
//
// Returns:
//   - pkg: The parsed package
//   - funcs: Map of function names to their declarations
//   - err: Error if directory contains multiple packages or parsing fails
//
// The function filters out test files (_test.go) and the main package automatically.
// Only exported functions are included unless specific names are requested via onlyFuncs.
func parsePackage(pkgDir, excludeFilename string, onlyFuncs ...string) (pkg *ast.Package, funcs map[string]funcDeclInFile, err error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, filterGoFiles(excludeFilename), 0)
	if err != nil {
		return nil, nil, err
	}
	delete(pkgs, "main") // ignore main package
	if len(pkgs) != 1 {
		var pkgNames []string
		for _, pkg := range pkgs {
			pkgNames = append(pkgNames, pkg.Name)
		}
		return nil, nil, fmt.Errorf("%d packages found in %s: %s", len(pkgs), pkgDir, strings.Join(pkgNames, ", "))
	}
	for _, p := range pkgs {
		pkg = p
	}

	// // typesInfo.Uses allows to lookup import paths for identifiers.
	// typesInfo := &types.Info{Uses: make(map[*ast.Ident]types.Object)}
	// // Type check the parsed code using the default importer.
	// // Use golang.org/x/tools/go/loader to check a program
	// // consisting of multiple packages.
	// conf := types.Config{Importer: importer.Default()}
	// _, err = conf.Check(pkgDir, fileSet, files, typesInfo)
	// if err != nil {
	// 	return nil, err
	// }

	funcs = make(map[string]funcDeclInFile)
	for _, file := range pkg.Files {
		// ast.Print(fileSet, file.Imports)
		for _, obj := range file.Scope.Objects {
			if obj.Kind != ast.Fun {
				// ast.Print(fileSet, obj)
				continue
			}
			funcDecl, ok := obj.Decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv != nil {
				continue
			}
			if len(onlyFuncs) > 0 {
				for _, name := range onlyFuncs {
					if funcDecl.Name.Name == name {
						funcs[name] = funcDeclInFile{Decl: funcDecl, File: file}
						break
					}
				}
			} else if funcDecl.Name.IsExported() {
				funcs[funcDecl.Name.Name] = funcDeclInFile{Decl: funcDecl, File: file}
			}
		}
	}
	return pkg, funcs, nil
}

// filterGoFiles creates a file filter function that excludes specified files and test files.
// This is used by the Go parser to determine which files to parse in a directory.
//
// Parameters:
//   - excludeFilenames: Names of files to exclude from parsing (e.g., previously generated files)
//
// Returns:
//   - A filter function suitable for parser.ParseDir that returns true for files to include
//
// The filter automatically excludes:
//   - Files in the excludeFilenames list
//   - All test files ending with _test.go
func filterGoFiles(excludeFilenames ...string) func(info os.FileInfo) bool {
	return func(info os.FileInfo) bool {
		name := info.Name()
		for _, exclude := range excludeFilenames {
			if name == exclude {
				return false
			}
		}
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return true
	}
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

// func parsePackage2(pkgDir, genFilename string, onlyFuncs ...string) (pkgName string, funcs map[*ast.FuncDecl]*ast.File, err error) {
// 	config := &packages.Config{
// 		Mode: packages.NeedName + packages.NeedImports + packages.NeedTypes + packages.NeedSyntax + packages.NeedTypesInfo,
// 		Dir:  pkgDir,
// 	}
// 	pkgs, err := packages.Load(config, pkgDir)
// 	if err != nil {
// 		return "", nil, err
// 	}
// 	if err != nil {
// 		return "", nil, err
// 	}
// 	if len(pkgs) != 1 {
// 		return "", nil, fmt.Errorf("%d packages found in %s", len(pkgs), pkgDir)
// 	}
// 	pkgName = pkgs[0].Name
// 	files := pkgs[0].Syntax

// 	funcs = make(map[*ast.FuncDecl]*ast.File)
// 	for _, file := range files {
// 		// ast.Print(fileSet, file.Imports)
// 		for _, obj := range file.Scope.Objects {
// 			if obj.Kind != ast.Fun {
// 				// ast.Print(fileSet, obj)
// 				continue
// 			}
// 			funcDecl := obj.Decl.(*ast.FuncDecl)
// 			if len(onlyFuncs) > 0 {
// 				for _, name := range onlyFuncs {
// 					if funcDecl.Name.Name == name {
// 						funcs[funcDecl] = file
// 						break
// 					}
// 				}
// 			} else if funcDecl.Name.IsExported() {
// 				funcs[funcDecl] = file
// 			}
// 		}
// 	}
// 	return pkgName, funcs, nil
// }
