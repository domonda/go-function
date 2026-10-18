package gen

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ungerik/go-astvisit"
	"golang.org/x/mod/module"
)

// packageFuncs holds function declarations for a package along with package location info.
type packageFuncs struct {
	Location *astvisit.PackageLocation // Package location and metadata
	Funcs    map[string]funcDeclInFile // Map of function name to declaration
}

// localAndImportedFunctions builds a complete map of all functions available to a file.
// This includes both local package functions and exported functions from imported packages.
//
// Parameters:
//   - fset: Token file set for parsing
//   - pkgName: Name of the package containing the file
//   - pkgFiles: All files of the package by path, including file
//   - file: The specific file to analyze
//   - pkgDir: Directory path of the package
//   - importNames: Import names that the wrappers of the file reference
//
// Returns:
//   - Map with package names as keys:
//   - "" (empty string): Functions from the same package
//   - "pkg": Functions from imported package "pkg"
//   - error if package parsing or import resolution fails
//
// The function:
//  1. Collects all functions from the local package (key: "")
//  2. For each import in the file that can have one of the importNames:
//     a. Locates the imported package's source
//     b. Parses the package, also of the standard library
//     c. Collects all exported functions, see packageFunctions
//     d. Adds to map with import name as key
//
// Only imports that can be referenced are located, so that imports
// which can't be located, like cgo's "C" or packages for other platforms,
// don't fail the generation of wrappers that don't reference them.
// The name of an import without explicit name is the package name,
// which is only known after locating the package.
// Imports whose package name guessed from the path is not referenced
// are only located if a referenced name was not found in the other imports,
// and only parsed if their located package name is referenced.
//
// This is used by the code generator to find the wrapped function's declaration
// when it's referenced as "pkg.FuncName" or just "FuncName".
func localAndImportedFunctions(fset *token.FileSet, pkgName string, pkgFiles map[string]*ast.File, file *ast.File, pkgDir string, importNames map[string]struct{}) (map[string]packageFuncs, error) {
	functions := map[string]packageFuncs{
		"": {
			Location: &astvisit.PackageLocation{
				Name: pkgName,
				Dir:  pkgDir,
			},
			Funcs: packageFunctions(pkgFiles, false),
		},
	}

	var referenced, others []astvisit.Import
	for _, spec := range file.Imports {
		imp, err := astvisit.ImportOf(spec)
		if err != nil {
			return nil, err
		}
		switch {
		case imp.Name != "":
			if _, ok := importNames[imp.Name]; ok {
				referenced = append(referenced, imp)
			}
		case imp.Path == "C":
			// Pseudo package of cgo without Go functions
		default:
			guessedName, err := guessPackageNameFromPath(imp.Path)
			_, isReferenced := importNames[guessedName]
			if err == nil && isReferenced {
				referenced = append(referenced, imp)
			} else {
				others = append(others, imp)
			}
		}
	}

	err := addImportedFunctions(fset, functions, pkgDir, referenced, importNames)
	if err != nil {
		return nil, err
	}
	for name := range importNames {
		if _, ok := functions[name]; !ok {
			// The package name of an import without explicit name
			// can differ from the name guessed from its path
			err = addImportedFunctions(fset, functions, pkgDir, others, importNames)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	return functions, nil
}

// addImportedFunctions locates the packages of imps from pkgDir
// and adds the exported functions of the packages whose import name
// is one of importNames to functions with the import name as key.
// The other packages are only located but not parsed,
// because their import name is only known after locating them.
func addImportedFunctions(fset *token.FileSet, functions map[string]packageFuncs, pkgDir string, imps []astvisit.Import, importNames map[string]struct{}) error {
	if len(imps) == 0 {
		return nil
	}
	importPaths := make([]string, len(imps))
	for i, imp := range imps {
		importPaths[i] = imp.Path
	}
	pkgLocations, err := astvisit.LocatePackages(pkgDir, importPaths...)
	if err != nil {
		return err
	}

	for _, imp := range imps {
		pkgLocation := pkgLocations[imp.Path]
		importName := cmp.Or(imp.Name, pkgLocation.Name)
		if _, ok := importNames[importName]; !ok {
			continue
		}
		impPkg, err := astvisit.ParsePackage(fset, pkgLocation.Dir, filterOutTests)
		if err != nil {
			return err
		}
		functions[importName] = packageFuncs{
			Location: pkgLocation,
			Funcs:    packageFunctions(impPkg.Files, true),
		}
	}
	return nil
}

// packageFunctions returns the functions without receiver
// declared in the files of a package by name, only exported ones if exportedOnly.
//
// astvisit.ParsePackage parses the files of all platforms,
// so a function can be declared in several files, like f_linux.go and f_windows.go.
// Then the declaration of a file that is part of the build
// for the current platform is used, else the one of the first file by path,
// so that the generated code doesn't depend on the map iteration order.
func packageFunctions(files map[string]*ast.File, exportedOnly bool) map[string]funcDeclInFile {
	funcs := make(map[string]funcDeclInFile)
	inBuild := make(map[string]bool) // Function name to if its declaration is from a file of the build
	for _, filePath := range slices.Sorted(maps.Keys(files)) {
		file := files[filePath]
		fileInBuild := fileMatchesBuildContext(filePath, file)
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv != nil || exportedOnly && !funcDecl.Name.IsExported() {
				continue
			}
			name := funcDecl.Name.Name
			if _, exists := funcs[name]; exists && (inBuild[name] || !fileInBuild) {
				continue
			}
			funcs[name] = funcDeclInFile{Decl: funcDecl, File: file}
			inBuild[name] = fileInBuild
		}
	}
	return funcs
}

// fileMatchesBuildContext reports if the file is part of the build
// for the current platform of build.Default by its name and build constraints.
// The file header is reconstructed from the parsed file,
// because the file may only exist in memory.
func fileMatchesBuildContext(filePath string, file *ast.File) bool {
	var header strings.Builder
	for _, group := range file.Comments {
		if group.End() >= file.Package {
			break
		}
		for _, comment := range group.List {
			header.WriteString(comment.Text)
			header.WriteByte('\n')
		}
		if group != file.Doc {
			// Keep build constraints separated from the package documentation
			header.WriteByte('\n')
		}
	}
	header.WriteString("package ")
	header.WriteString(file.Name.Name)
	header.WriteByte('\n')
	for _, imp := range file.Imports {
		if imp.Path.Value == `"C"` {
			// cgo files are excluded without cgo
			header.WriteString("import \"C\"\n")
		}
	}
	ctxt := build.Default
	ctxt.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(header.String())), nil
	}
	match, err := ctxt.MatchFile(filepath.Dir(filePath), filepath.Base(filePath))
	return err == nil && match
}

// gatherFieldListImports collects import statements needed for types in a field list
// and returns a mapping to translate package qualifiers from source context to target context.
//
// Parameters:
//   - funcFile: The file containing the function (source of import information)
//   - funcDir: The directory of funcFile that resolves its imports
//   - fieldList: AST field list containing parameters or results
//   - setImports: Set to add required imports to (modified in-place), must not be nil
//   - target: The file the wrappers are generated into, to check for conflicts
//
// Returns:
//   - packageRemap: Map of source package name -> target package name (for type translation)
//   - error if a package qualifier of the types is not imported by funcFile
//     or the name of an imported package can't be located
//
// The function:
//  1. Extracts all package qualifiers used in the field types (e.g., "context" from "context.Context")
//  2. Matches qualifiers against the imports of funcFile
//  3. Checks for alias conflicts with the imports of target
//  4. Builds a translation map (source pkg name -> target pkg name) for type rewriting
//  5. Adds matching imports to setImports:
//     - "path/to/package" for imports without aliases
//     - alias "path/to/package" for aliased imports
//     - Uses alternative alias if there's a conflict
//
// Example:
//
//	Source file: import gmail "google.golang.org/api/gmail/v1"
//	Target file: import gmailapi "google.golang.org/api/gmail/v1"
//	Result: packageRemap["gmail"] = "gmailapi"
func gatherFieldListImports(funcFile *ast.File, funcDir string, fieldList *ast.FieldList, setImports astvisit.Imports, target *targetFile) (packageRemap map[string]string, err error) {
	if fieldList == nil {
		return nil, nil
	}
	qualifiers := make(map[string]struct{})
	for _, field := range fieldList.List {
		astvisit.TypeExprNameQualifyers(field.Type, qualifiers)
	}
	return target.addImportsOfQualifiers(funcFile, funcDir, qualifiers, setImports)
}

// addImportsOfQualifiers adds the imports of funcFile with the package qualifiers
// to setImports that target needs for code using them,
// and returns a mapping from the qualifiers to the names that target uses for them.
// See gatherFieldListImports.
func (target *targetFile) addImportsOfQualifiers(funcFile *ast.File, funcDir string, qualifiers map[string]struct{}, setImports astvisit.Imports) (packageRemap map[string]string, err error) {
	packageRemap = make(map[string]string) // source name -> target name
	sourceImports, err := target.names.importsOfQualifiers(funcFile, funcDir, qualifiers)
	if err != nil {
		return nil, err
	}
	for _, importName := range slices.Sorted(maps.Keys(sourceImports)) {
		imp := sourceImports[importName]

		// First, check if the target file already imports this path (possibly with different alias)
		targetName, alreadyImported, err := target.nameOfPath(imp, importName)
		if err != nil {
			return nil, err
		}
		if alreadyImported && !generatedIdents[targetName] {
			// Record the name mapping for type translation,
			// don't add the import because it's already there
			if importName != targetName {
				packageRemap[importName] = targetName
			}
			continue
		}

		// Check if the import name we want to use conflicts with the imports
		// of the target file or the generated code, or would be shadowed
		// by an identifier of the generated code
		if target.nameConflicts(importName, imp.Path) {
			// The name is used for a DIFFERENT package
			// We need to use an alternative alias to avoid conflict
			altAlias := importName + "2"
			for i := 3; target.nameConflicts(altAlias, imp.Path); i++ {
				altAlias = importName + strconv.Itoa(i)
			}
			delete(setImports, astvisit.Import{Path: imp.Path})
			setImports.AddNamed(altAlias, imp.Path)
			target.setAdded(altAlias, imp.Path)
			packageRemap[importName] = altAlias
			continue
		}
		target.setAdded(importName, imp.Path)

		// No conflict - add with or without alias
		if imp.Name != "" {
			// Source uses an alias, preserve it.
			// An import of the path without alias that declares the same name
			// is replaced, with another name it's still needed by other wrappers.
			if pkgName, err := guessPackageNameFromPath(imp.Path); err == nil && pkgName == importName {
				delete(setImports, astvisit.Import{Path: imp.Path})
			}
			setImports.AddNamed(importName, imp.Path)
		} else if _, ok := setImports[astvisit.Import{Name: importName, Path: imp.Path}]; !ok {
			// No alias in source, add without alias
			setImports.Add(imp.Path)
		}
	}
	return packageRemap, nil
}

// packageNames caches the names of packages by import path
// that were located with the go command,
// because the name of a package can differ from its import path.
type packageNames map[string]string

// locate adds the names of the packages with the import paths
// that are not cached yet, located from dir with a single go list call.
func (names packageNames) locate(dir string, importPaths ...string) error {
	var missing []string
	for _, path := range importPaths {
		if _, ok := names[path]; !ok {
			missing = append(missing, path)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	locations, err := astvisit.LocatePackages(dir, missing...)
	if err != nil {
		return err
	}
	for path, location := range locations {
		names[path] = location.Name
	}
	return nil
}

// importsOfQualifiers returns the imports of file by the qualifiers
// that file uses for them, for all package qualifiers.
//
// The package names of imports without explicit name are guessed
// from their import path. Only if a qualifier doesn't match a name,
// the packages of the other imports without explicit name
// are located from dir, the directory of file.
// Returns an error if a qualifier doesn't match any import.
func (names packageNames) importsOfQualifiers(file *ast.File, dir string, qualifiers map[string]struct{}) (map[string]astvisit.Import, error) {
	imports := make(map[string]astvisit.Import, len(qualifiers))
	if len(qualifiers) == 0 {
		return imports, nil
	}
	var unnamed []astvisit.Import
	for _, spec := range file.Imports {
		imp, err := astvisit.ImportOf(spec)
		if err != nil {
			return nil, err
		}
		switch {
		case imp.Name == "_" || imp.Name == "." || imp.Path == "C":
			// Can't be referenced with a qualifier
		case imp.Name != "":
			if _, ok := qualifiers[imp.Name]; ok {
				imports[imp.Name] = imp
			}
		default:
			unnamed = append(unnamed, imp)
		}
	}

	// Match the qualifiers to located or guessed package names
	// of the imports without explicit name.
	// A guessed name is only used if no other import has the same guess.
	var guessedImports []astvisit.Import
	guessed := make(map[string][]astvisit.Import)
	for _, imp := range unnamed {
		if name, ok := names[imp.Path]; ok {
			if _, isQualifier := qualifiers[name]; isQualifier && imports[name] == (astvisit.Import{}) {
				imports[name] = imp
			}
			continue
		}
		guessedImports = append(guessedImports, imp)
		if name, err := guessPackageNameFromPath(imp.Path); err == nil {
			guessed[name] = append(guessed[name], imp)
		}
	}
	var unmatched []string
	for qualifier := range qualifiers {
		if _, ok := imports[qualifier]; ok {
			continue
		}
		if candidates := guessed[qualifier]; len(candidates) == 1 {
			imports[qualifier] = candidates[0]
			continue
		}
		unmatched = append(unmatched, qualifier)
	}
	if len(unmatched) == 0 {
		return imports, nil
	}

	// Locate the packages of the imports whose names were guessed
	var paths []string
	for _, imp := range guessedImports {
		paths = append(paths, imp.Path)
	}
	err := names.locate(dir, paths...)
	if err != nil {
		return nil, err
	}
	for _, imp := range guessedImports {
		name := names[imp.Path]
		if _, isQualifier := qualifiers[name]; isQualifier && slices.Contains(unmatched, name) {
			imports[name] = imp
		}
	}
	for _, qualifier := range unmatched {
		if _, ok := imports[qualifier]; !ok {
			return nil, fmt.Errorf("package %s is not imported by the file of the wrapped function in %s", qualifier, dir)
		}
	}
	return imports, nil
}

// targetFile is the file that the wrappers are generated into.
type targetFile struct {
	dir     string            // Directory of the file that resolves its imports
	imports []astvisit.Import // Imports of the file
	names   packageNames      // Located package names, shared with the wrapped functions' files
	added   map[string]string // Import paths by name that the generated code uses, see setAdded

	// guessedNamesLocated is set after the names of the imports
	// whose package name can't be guessed were located
	guessedNamesLocated bool
}

// newTargetFile returns the targetFile for the parsed file in dir.
// The names cache may be shared with other files.
func newTargetFile(file *ast.File, dir string, names packageNames) (*targetFile, error) {
	target := &targetFile{dir: dir, names: names}
	for _, spec := range file.Imports {
		imp, err := astvisit.ImportOf(spec)
		if err != nil {
			return nil, err
		}
		target.imports = append(target.imports, imp)
	}
	return target, nil
}

// functionImportPath is the import path of the go-function package
// that the generated code uses.
const functionImportPath = "github.com/domonda/go-function"

// functionImportName returns the name that the file uses
// for the go-function package, "." for a dot import,
// or "" if the file doesn't import it.
func (target *targetFile) functionImportName() string {
	name := ""
	for _, imp := range target.imports {
		switch {
		case imp.Path != functionImportPath || imp.Name == "_":
			continue
		case imp.Name == ".":
			// Prefer a qualified import
			name = "."
		default:
			return cmp.Or(imp.Name, "function")
		}
	}
	return name
}

// functionSel returns the selector that the generated code uses
// for the declarations of the go-function package, like "function."
// and adds the import of go-function to setImports if target doesn't import it.
// Returns an error if the import would conflict with another import.
func (target *targetFile) functionSel(setImports astvisit.Imports) (string, error) {
	switch name := target.functionImportName(); name {
	case "":
		return "function.", target.useImport(setImports, "function", functionImportPath)
	case ".":
		return "", nil
	default:
		if generatedIdents[name] {
			return "", fmt.Errorf("the generated code declares the identifier %s that is the import name of %q in the file, import it with another name", name, functionImportPath)
		}
		return name + ".", nil
	}
}

// generatedImports are the import paths by package name
// of the packages that the generated code uses besides go-function.
var generatedImports = map[string]string{
	"context": "context",
	"fmt":     "fmt",
	"json":    "encoding/json",
	"reflect": "reflect",
}

// generatedIdents are the identifiers that the generated methods declare:
// the receiver, parameters, named results, and local variables.
// They would shadow packages and functions with the same name.
var generatedIdents = map[string]bool{
	"a":        true,
	"args":     true,
	"argsJSON": true,
	"ctx":      true,
	"err":      true,
	"f":        true,
	"i":        true,
	"ok":       true,
	"raw":      true,
	"results":  true,
	"str":      true,
	"strs":     true,
	"v":        true,
}

// useImport adds the import of the package with the import path
// and the package name pkgName to setImports for generated code
// that refers to it by pkgName.
// Returns an error if the target file or the generated code
// uses pkgName for another package.
func (target *targetFile) useImport(setImports astvisit.Imports, pkgName, path string) error {
	if otherPath, ok := target.pathOfName(pkgName); ok && otherPath != path {
		return fmt.Errorf("the generated code imports %q with the name %s that the file uses for %q", path, pkgName, otherPath)
	}
	if otherPath, ok := target.added[pkgName]; ok && otherPath != path {
		return fmt.Errorf("the generated code imports %q and %q with the same name %s", otherPath, path, pkgName)
	}
	target.setAdded(pkgName, path)
	setImports.Add(path)
	return nil
}

// setAdded records that the generated code uses name for the import path.
func (target *targetFile) setAdded(name, path string) {
	if target.added == nil {
		target.added = make(map[string]string)
	}
	target.added[name] = path
}

// nameConflicts reports if code generated into the target file
// can't refer to the package with the import path by name, because the
// target file or the generated code uses name for another package,
// or because an identifier of the generated code would shadow it.
func (target *targetFile) nameConflicts(name, path string) bool {
	if generatedIdents[name] {
		return true
	}
	if generatedPath, ok := generatedImports[name]; ok && generatedPath != path {
		return true
	}
	if name == cmp.Or(target.functionImportName(), "function") && path != functionImportPath {
		return true
	}
	if addedPath, ok := target.added[name]; ok && addedPath != path {
		return true
	}
	if targetPath, ok := target.pathOfName(name); ok && targetPath != path {
		return true
	}
	return false
}

// nameOfPath returns the name that the target file uses
// for the package of srcImp that the file of a wrapped function
// imports with srcName, and if the target file imports the package
// with a name that code can use.
func (target *targetFile) nameOfPath(srcImp astvisit.Import, srcName string) (name string, imported bool, err error) {
	for _, imp := range target.imports {
		if imp.Path != srcImp.Path || imp.Name == "_" || imp.Name == "." {
			continue
		}
		switch {
		case imp.Name != "":
			return imp.Name, true, nil
		case srcImp.Name == "":
			// Both files import the package without explicit name,
			// so they use its package name
			return srcName, true, nil
		}
		err = target.names.locate(target.dir, imp.Path)
		if err != nil {
			return "", false, err
		}
		return target.names[imp.Path], true, nil
	}
	return "", false, nil
}

// pathOfName returns the import path of the package
// that the target file imports with the name.
// The names of packages imported without explicit name are guessed,
// or located if they can't be guessed.
func (target *targetFile) pathOfName(name string) (path string, ok bool) {
	if !target.guessedNamesLocated {
		target.guessedNamesLocated = true
		var unguessable []string
		for _, imp := range target.imports {
			if _, err := guessPackageNameFromPath(imp.Path); imp.Name == "" && imp.Path != "C" && err != nil {
				unguessable = append(unguessable, imp.Path)
			}
		}
		// A package that can't be located can't conflict
		// with a package of the generated code that compiles
		_ = target.names.locate(target.dir, unguessable...)
	}
	for _, imp := range target.imports {
		importName := imp.Name
		if importName == "" {
			var ok bool
			importName, ok = target.names[imp.Path]
			if !ok {
				importName, _ = guessPackageNameFromPath(imp.Path)
			}
		}
		if importName == name && imp.Name != "_" && imp.Name != "." {
			return imp.Path, true
		}
	}
	return "", false
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
//  2. Remove a major version suffix of a module path like "/v2" or the ".v3" of gopkg.in
//  3. Take the last path component after "/"
//  4. Remove "go-" prefix (common Go package naming convention)
//  5. Remove ".go" suffix (edge case)
//  6. Validate result doesn't contain "." or "-"
//
// Examples:
//   - `"github.com/user/mypackage"` -> "mypackage"
//   - `"github.com/user/go-types"` -> "types"
//   - `"github.com/user/mylib/v2"` -> "mylib"
//   - `"gopkg.in/yaml.v3"` -> "yaml"
//   - `"k8s.io/api/core/v1"` -> "v1" (v1 is no major version suffix)
//   - `"io"` -> "io"
//   - `"github.com/user/my-pkg"` -> error (contains dash)
//
// The guess can be wrong, the name of a package is only known
// after locating it with packageNames.locate.
func guessPackageNameFromPath(path string) (string, error) {
	pkg := path
	if len(pkg) >= 2 && pkg[0] == '"' && pkg[len(pkg)-1] == '"' {
		pkg = pkg[1 : len(pkg)-1]
	}
	// The prefix is pkg if it has no valid major version suffix
	pkg, _, _ = module.SplitPathVersion(pkg)
	pkg = pkg[strings.LastIndex(pkg, "/")+1:]
	pkg = strings.TrimPrefix(pkg, "go-")
	pkg = strings.TrimSuffix(pkg, ".go")
	if pkg == "" || strings.ContainsAny(pkg, ".-") {
		return "", fmt.Errorf("could not guess package name from import path %s", path)
	}
	return pkg, nil
}
