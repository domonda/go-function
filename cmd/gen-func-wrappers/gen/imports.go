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

// gatherFieldListImports collects import statements needed for types in a field list
// and returns a mapping to translate package qualifiers from source context to target context.
//
// Parameters:
//   - funcFile: The file containing the function (source of import information)
//   - fieldList: AST field list containing parameters or results
//   - setImportLines: Map to add required import lines to (modified in-place)
//   - targetFileImports: Optional slice of imports from the target file to check for conflicts
//
// Returns:
//   - packageRemap: Map of source package name -> target package name (for type translation)
//   - error if package name cannot be guessed from import path
//
// The function:
//  1. Extracts all package qualifiers used in the field types (e.g., "context" from "context.Context")
//  2. Matches qualifiers against the file's imports
//  3. Checks for alias conflicts with targetFileImports (if provided)
//  4. Builds a translation map (source pkg name -> target pkg name) for type rewriting
//  5. Adds matching imports to setImportLines in proper format:
//     - `"path/to/package"` for imports without aliases
//     - `alias "path/to/package"` for aliased imports
//     - Uses alternative alias if there's a conflict
//
// Example:
//
//	Source file: import gmail "google.golang.org/api/gmail/v1"
//	Target file: import gmailapi "google.golang.org/api/gmail/v1"
//	Result: packageRemap["gmail"] = "gmailapi"
func gatherFieldListImports(funcFile *ast.File, fieldList *ast.FieldList, setImportLines map[string]struct{}, targetFileImports []*ast.ImportSpec) (packageRemap map[string]string, err error) {
	if fieldList == nil {
		return nil, nil
	}

	packageRemap = make(map[string]string) // source name -> target name

	// Build maps of the target file's imports
	// to detect conflicts and reuse existing aliases
	targetImportsByName := make(map[string]string) // name -> path
	targetImportsByPath := make(map[string]string) // path -> name
	if targetFileImports != nil {
		for _, imp := range targetFileImports {
			var name string
			if imp.Name != nil {
				name = imp.Name.Name
			} else {
				var err error
				name, err = guessPackageNameFromPath(imp.Path.Value)
				if err != nil {
					// Skip imports we can't parse
					continue
				}
			}
			targetImportsByName[name] = imp.Path.Value
			targetImportsByPath[imp.Path.Value] = name
		}
	}

	packageNames := make(map[string]struct{})
	for _, field := range fieldList.List {
		astvisit.TypeExprNameQualifyers(field.Type, packageNames)
	}

	for _, imp := range funcFile.Imports {
		var importName string
		var importPath string

		if imp.Name != nil {
			importName = imp.Name.Name
			importPath = imp.Path.Value
		} else {
			var err error
			importName, err = guessPackageNameFromPath(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			importPath = imp.Path.Value
		}

		// Check if this package name is used in the field types
		if _, needed := packageNames[importName]; !needed {
			continue
		}

		// First, check if the target file already imports this path (possibly with different alias)
		if targetName, alreadyImported := targetImportsByPath[importPath]; alreadyImported {
			// Target file already has this package imported
			// Record the name mapping for type translation
			if importName != targetName {
				packageRemap[importName] = targetName
			}
			// Don't add import - it's already there with whatever alias/name target uses
			continue
		}

		// Check if the import name we want to use conflicts with target file's imports
		if _, nameUsed := targetImportsByName[importName]; nameUsed {
			// The target file uses this name for a DIFFERENT package
			// We need to use an alternative alias to avoid conflict
			altAlias := importName + "2"
			for i := 2; ; i++ {
				if _, conflict := targetImportsByName[altAlias]; !conflict {
					break
				}
				altAlias = importName + fmt.Sprint(i+1)
			}
			delete(setImportLines, importPath)
			setImportLines[altAlias+" "+importPath] = struct{}{}
			// Record the remapping
			packageRemap[importName] = altAlias
		} else {
			// No conflict - add with or without alias
			if imp.Name != nil {
				// Source uses an alias, preserve it
				delete(setImportLines, importPath)
				setImportLines[importName+" "+importPath] = struct{}{}
			} else {
				// No alias in source, add without alias
				if _, ok := setImportLines[importName+" "+importPath]; !ok {
					setImportLines[importPath] = struct{}{}
				}
			}
			// No remapping needed - source and target use same name
		}
	}
	return packageRemap, nil
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
