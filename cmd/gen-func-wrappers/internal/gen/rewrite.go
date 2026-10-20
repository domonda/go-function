package gen

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ungerik/go-astvisit"
	"golang.org/x/tools/go/ast/astutil"
)

// ErrWrappersOutdated is wrapped by the error that the rewrite functions
// return in validate mode for a file whose wrappers are missing or outdated,
// to tell it apart from errors that rewriting the file wouldn't fix.
var ErrWrappersOutdated = errors.New("function wrappers are missing or outdated")

// RewriteDir processes a directory (and optionally subdirectories) to rewrite wrapper declarations.
// This is the main entry point for the code generator when invoked on directories.
//
// Parameters:
//   - path: Directory path to process; append "..." for recursive processing
//   - verbose: If true, print detailed information about processing
//   - printOnly: If not nil, print generated code to this writer instead of modifying files
//   - validate: If true, check for outdated wrappers without modifying files
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON unmarshalling
//   - localImportPrefixes: Import path prefixes to treat as "local" for import grouping
//
// Returns:
//   - error if any file processing fails or validation detects issues
//
// Validation continues after files with missing or outdated wrappers
// and returns an error joining all of them that matches ErrWrappersOutdated,
// unless processing another file fails, which returns that error.
//
// The function:
//  1. Checks if path ends with "..." for recursive processing
//  2. Processes all Go files in the directory in sorted order
//  3. If recursive, processes all subdirectories except the ones the go command
//     ignores for ./... patterns (starting with . or _, testdata, vendor,
//     and nested modules with their own go.mod file) and node_modules
//  4. Skips test files (_test.go)
//
// Like the go command, nested modules are only skipped within a module,
// the modules below a directory that is not within a module are entered.
//
// Example:
//
//	err := RewriteDir("./pkg/mypackage/...", true, nil, false, nil, []string{"github.com/myorg/"})
func RewriteDir(path string, verbose bool, printOnly io.Writer, validate bool, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	recursive := strings.HasSuffix(path, "...")
	if recursive {
		path = filepath.Clean(strings.TrimSuffix(path, "..."))
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		return RewriteFile(path, verbose, printOnly, validate, jsonTypeReplacements, localImportPrefixes)
	}
	inModule := false
	if recursive {
		inModule, err = isWithinModule(path)
		if err != nil {
			return err
		}
	}
	return rewriteDir(path, recursive, inModule, verbose, printOnly, validate, jsonTypeReplacements, localImportPrefixes)
}

// rewriteDir implements RewriteDir for the directory dir
// that is within a module if inModule is true.
func rewriteDir(dir string, recursive, inModule, verbose bool, printOnly io.Writer, validate bool, jsonTypeReplacements map[string]string, localImportPrefixes []string) error {
	fset := token.NewFileSet()
	pkg, err := astvisit.ParsePackage(fset, dir, filterOutTests)
	if err != nil && (!recursive || !errors.Is(err, astvisit.ErrPackageNotFound)) {
		return err
	}
	// outdated joins the errors of all files with missing or outdated wrappers,
	// other errors are returned immediately
	var outdated error
	if err == nil {
		// Sorted for deterministic output and errors
		for _, fileName := range slices.Sorted(maps.Keys(pkg.Files)) {
			err = RewriteAstFile(fset, pkg.Name, pkg.Files, pkg.Files[fileName], fileName, verbose, printOnly, validate, jsonTypeReplacements, localImportPrefixes)
			if errors.Is(err, ErrWrappersOutdated) {
				outdated = errors.Join(outdated, err)
				continue
			}
			if err != nil {
				return err
			}
		}
	} else if verbose {
		fmt.Println(err)
	}
	if !recursive {
		return outdated
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		fileName := file.Name()
		// Skip directories ignored by the go command for ./... patterns and node_modules
		if !file.IsDir() || fileName[0] == '.' || fileName[0] == '_' || fileName == "testdata" || fileName == "vendor" || fileName == "node_modules" {
			continue
		}
		subDir := filepath.Join(dir, fileName)
		subInModule := inModule
		if isModuleRoot(subDir) {
			if inModule {
				// Skip nested modules like the go command
				continue
			}
			subInModule = true
		}
		err = rewriteDir(subDir, true, subInModule, verbose, printOnly, validate, jsonTypeReplacements, localImportPrefixes)
		if errors.Is(err, ErrWrappersOutdated) {
			outdated = errors.Join(outdated, err)
			continue
		}
		if err != nil {
			return err
		}
	}
	return outdated
}

// isModuleRoot reports if dir contains a go.mod file.
func isModuleRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil && !info.IsDir()
}

// isWithinModule reports if dir or one of its parent directories contains a go.mod file.
func isWithinModule(dir string) (bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	for {
		if isModuleRoot(dir) {
			return true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false, nil
		}
		dir = parent
	}
}

// RewriteFile processes a single Go source file to rewrite wrapper declarations.
//
// Parameters:
//   - filePath: Path to the Go source file to process
//   - verbose: If true, print detailed information about processing
//   - printOnly: If not nil, print generated code to this writer instead of modifying file
//   - validate: If true, check for outdated wrappers without modifying files
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON unmarshalling
//   - localImportPrefixes: Import path prefixes to treat as "local" for import grouping
//
// Returns:
//   - error if file doesn't exist, is a directory, or processing fails
//
// The function:
//  1. Parses the package containing the file
//  2. Calls RewriteAstFile to perform the actual rewriting
//  3. Writes the modified content back to the file (or prints it)
func RewriteFile(filePath string, verbose bool, printOnly io.Writer, validate bool, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	filePath = filepath.Clean(filePath)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("file path is a directory: %s", filePath)
	}
	fset := token.NewFileSet()
	pkg, err := astvisit.ParsePackage(fset, filepath.Dir(filePath), filterOutTests)
	if err != nil {
		return err
	}
	astFile, ok := pkg.Files[filePath]
	if !ok {
		return fmt.Errorf("file is not part of the package build, it's a test file or excluded by build constraints: %s", filePath)
	}
	return RewriteAstFile(fset, pkg.Name, pkg.Files, astFile, filePath, verbose, printOnly, validate, jsonTypeReplacements, localImportPrefixes)
}

// RewriteAstFile is the core rewriting logic that processes an AST file.
// This function performs the actual wrapper code generation and replacement.
//
// Parameters:
//   - fset: Token file set for position information
//   - pkgName: The package name
//   - pkgFiles: All files in the package
//   - astFile: The parsed AST of the file to process
//   - filePath: Original file path (for error messages and writing)
//   - verbose: If true, print detailed information
//   - printTo: If not nil, print to this writer instead of modifying file
//   - validate: If true, check for outdated wrappers without modifying files
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON
//   - localImportPrefixes: Import path prefixes to treat as "local"
//
// Returns:
//   - error if wrapper generation or file writing fails
//
// The function:
//  1. Scans the AST for wrapper declarations (TODO calls or implementation comments)
//  2. Finds the wrapped function's declaration in the package or imports
//  3. Generates wrapper implementation code using WriteFunctionWrapper
//  4. Replaces old declarations with generated code using AST node replacements
//  5. Adds missing imports automatically
//  6. Formats the result and writes it back
//
// Wrapper declarations are found by:
//   - Variable assignments with TODO calls: var x = function.WrapperTODO(F)
//   - Implementation comments: // myWrapper wraps F as function.Wrapper (generated code)
func RewriteAstFile(fset *token.FileSet, pkgName string, pkgFiles map[string]*ast.File, astFile *ast.File, filePath string, verbose bool, printTo io.Writer, validate bool, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	filePath = filepath.Clean(filePath)

	source, err := os.ReadFile(filePath) //#nosec G304
	if err != nil {
		return err
	}
	return RewriteAstFileSource(fset, pkgName, pkgFiles, astFile, filePath, source, verbose, printTo, validate, jsonTypeReplacements, localImportPrefixes)
}

// RewriteAstFileSource is like RewriteAstFile but accepts the source content directly.
// This allows for in-memory testing without requiring files on disk.
//
// Parameters:
//   - fset: Token file set for position information
//   - pkgName: The package name
//   - pkgFiles: All files in the package (for finding function declarations)
//   - astFile: The parsed AST of the file to process
//   - filePath: File path for error messages (doesn't need to exist)
//   - source: The original source code as bytes
//   - verbose: If true, print detailed information
//   - printTo: If not nil, print to this writer instead of modifying file
//   - validate: If true, check for outdated wrappers without modifying files
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON
//   - localImportPrefixes: Import path prefixes to treat as "local"
//
// Returns:
//   - error if wrapper generation or file writing fails,
//     an error wrapping ErrWrappersOutdated if validate finds
//     that the wrappers of the file are missing or outdated
//
// This function is useful for testing as it doesn't require actual files on disk.
func RewriteAstFileSource(fset *token.FileSet, pkgName string, pkgFiles map[string]*ast.File, astFile *ast.File, filePath string, source []byte, verbose bool, printTo io.Writer, validate bool, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	filePath = filepath.Clean(filePath)
	pkgDir := filepath.Dir(filePath)

	target, err := newTargetFile(astFile, pkgDir, make(packageNames))
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	// ast.Print(fset, file)
	wrappers, err := findFunctionWrappers(fset, astFile, target)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}
	if len(wrappers) == 0 {
		if verbose {
			fmt.Println("no wrappers found to rewrite in", filePath)
		}
		return nil
	}

	// Gather imported packages of file
	// and parse packages for function declarations
	// that could be referenced by function.Wrapper implementations
	// Also parse all functions of the file's package
	// because they could als be referenced with an empty import name.
	// Added with empty string as package/import name.
	importNames := make(map[string]struct{})
	for _, wrapper := range wrappers {
		wrappedFuncPackage, _ := wrapper.WrappedFuncPkgAndFuncName()
		importNames[wrappedFuncPackage] = struct{}{}
	}
	functions, err := localAndImportedFunctions(fset, pkgName, pkgFiles, astFile, pkgDir, importNames)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	neededImports := make(astvisit.Imports)

	var replacements astvisit.NodeReplacements
	for _, wrapper := range wrappers {
		wrappedFuncPackage, wrappedFuncName := wrapper.WrappedFuncPkgAndFuncName()
		referencedPkg, ok := functions[wrappedFuncPackage]
		if !ok {
			return fmt.Errorf("%s: can't find package %s of %s in the imports of the file", filePath, wrappedFuncPackage, wrapper.WrappedFunc)
		}
		wrappedFunc, ok := referencedPkg.Funcs[wrappedFuncName]
		if !ok {
			return fmt.Errorf("%s: can't find function %s in package %s", filePath, wrappedFuncName, wrappedFuncPackage)
		}

		var repl strings.Builder
		implType := wrapper.Type
		if wrapper.VarName != "" {
			implType = wrapper.VarName + "T"
			fmt.Fprintf(&repl, "// %s wraps %s as %s (generated code)\n", wrapper.VarName, wrapper.WrappedFunc, wrapper.Impl)
			fmt.Fprintf(&repl, "var %[1]s %[2]s\n\n", wrapper.VarName, implType)
		}
		err = wrapper.Impl.WriteFunctionWrapper(&repl, wrappedFunc.File, referencedPkg.Location.Dir, wrappedFunc.Decl, implType, wrappedFuncPackage, neededImports, jsonTypeReplacements, target)
		if err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}

		var implReplacements astvisit.NodeReplacements
		debugID := "Wrapper for " + wrapper.WrappedFunc
		if wrapper.InsertAfter != nil {
			implReplacements.AddInsertAfter(wrapper.InsertAfter, "\n\n"+repl.String(), debugID)
		}
		for i, node := range wrapper.Nodes {
			if i == 0 && wrapper.InsertAfter == nil {
				implReplacements.AddReplacement(node, repl.String(), debugID)
			} else {
				implReplacements.AddRemoval(node, debugID)
			}
		}
		replacements.Add(implReplacements)
	}

	rewritten, err := replacements.Apply(fset, source)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	// Parse rewritten again to add missing imports
	// to the ast.File and pretty print the result
	rewritten, err = astvisit.FormatSource(rewritten, neededImports, localImportPrefixes...)
	if err != nil {
		return fmt.Errorf("%s: invalid generated code: %w", filePath, err)
	}
	// Remove the imports that the previously generated code
	// or the TODO calls needed but the generated code doesn't
	rewritten, err = removeUnusedImports(rewritten, target)
	if err != nil {
		return fmt.Errorf("%s: invalid generated code: %w", filePath, err)
	}

	// In validate mode, check if the generated code differs from the current source
	if validate {
		if !bytes.Equal(source, rewritten) {
			return fmt.Errorf("%s: %w", filePath, ErrWrappersOutdated)
		}
		if verbose {
			fmt.Println(filePath, "is up to date")
		}
		return nil
	}

	if printTo != nil {
		if verbose {
			fmt.Println(filePath, "would be rewritten as:")
		}
		_, err = printTo.Write(rewritten)
		return err
	}
	if verbose {
		fmt.Println("rewriting", filePath)
	}
	return os.WriteFile(filePath, rewritten, 0640) //#nosec G306,G703 -- developer tool: filePath comes from CLI args, writing back the rewritten source is the intended behavior
}

// removeUnusedImports removes the imports that source doesn't use,
// except blank imports, dot imports, and the pseudo package "C" of cgo,
// because the regenerated wrappers may not use all imports anymore that
// the previously generated code or the TODO calls needed.
//
// The name of an import without explicit name is its package name,
// which is located with the go command, unless the name guessed from
// the import path is used by source. An import is not removed
// if its package can't be located.
func removeUnusedImports(source []byte, target *targetFile) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	// The package qualifiers that source uses are the unresolved
	// identifiers of selector expressions, a resolved identifier
	// is a variable or another declaration of the file
	used := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Obj == nil {
				used[x.Name] = true
			}
		}
		return true
	})

	var unused, unknown []astvisit.Import
	for _, spec := range file.Imports {
		imp, err := astvisit.ImportOf(spec)
		if err != nil {
			return nil, err
		}
		if imp.Name == "_" || imp.Name == "." || imp.Path == "C" {
			continue
		}
		name, known := imp.Name, imp.Name != ""
		if !known {
			name, known = target.names[imp.Path]
		}
		if !known {
			if guessed, err := guessPackageNameFromPath(imp.Path); err == nil && used[guessed] {
				continue
			}
			unknown = append(unknown, imp)
			continue
		}
		if !used[name] {
			unused = append(unused, imp)
		}
	}
	if len(unknown) > 0 {
		paths := make([]string, len(unknown))
		for i, imp := range unknown {
			paths[i] = imp.Path
		}
		if target.names.locate(target.dir, paths...) != nil {
			// Locate one by one, so that a package that can't be located
			// doesn't prevent the removal of the others
			for _, path := range paths {
				_ = target.names.locate(target.dir, path)
			}
		}
		for _, imp := range unknown {
			if name, ok := target.names[imp.Path]; ok && !used[name] {
				unused = append(unused, imp)
			}
		}
	}
	if len(unused) == 0 {
		return source, nil
	}

	for _, imp := range unused {
		astutil.DeleteNamedImport(fset, file, imp.Name, imp.Path)
	}
	var formatted bytes.Buffer
	err = format.Node(&formatted, fset, file)
	if err != nil {
		return nil, err
	}
	return formatted.Bytes(), nil
}

// wrapper represents a function wrapper declaration found in source code.
// It contains all information needed to generate the wrapper implementation.
type wrapper struct {
	VarName     string     // Name of the wrapper variable (e.g., "myWrapper"), empty for a type without var
	WrappedFunc string     // Full name of the wrapped function (e.g., "pkg.MyFunc" or "MyFunc")
	Type        string     // Name of the wrapper type (e.g., "myWrapperT")
	Nodes       []ast.Node // All AST nodes to be replaced (comments, var, type, methods)
	InsertAfter ast.Node   // If not nil, the generated code is inserted after it and all Nodes are removed
	Impl        Impl       // Which wrapper interfaces to implement
}

// WrappedFuncPkgAndFuncName splits the WrappedFunc into package and function name.
// Returns empty package name if the function is in the same package (no dot).
//
// Examples:
//   - "MyFunc" -> ("", "MyFunc")
//   - "pkg.MyFunc" -> ("pkg", "MyFunc")
func (impl *wrapper) WrappedFuncPkgAndFuncName() (pkgName, funcName string) {
	dot := strings.IndexByte(impl.WrappedFunc, '.')
	if dot == -1 {
		return "", impl.WrappedFunc
	}
	return impl.WrappedFunc[:dot], impl.WrappedFunc[dot+1:]
}

// findFunctionWrappers scans an AST file for wrapper declarations.
// It recognizes two declaration patterns:
//
// Pattern 1: TODO function call, also in a var block
//
//	var myWrapper = function.WrapperTODO(MyFunction)
//
// The TODO functions are recognized with the name that the file uses for go-function,
// for example fn.WrapperTODO for an import with the name fn,
// or WrapperTODO for a dot import.
//
// Pattern 2: Implementation comment with var and type
//
//	// myWrapper wraps MyFunction as function.Wrapper (generated code)
//	var myWrapper myWrapperT
//
//	// myWrapperT wraps MyFunction as function.Wrapper (generated code)
//	type myWrapperT struct{}
//
// A type with implementation comment without var is a wrapper of its own.
//
// The vars are collected before the types, and the types before the methods,
// so that the order of the declarations in the file doesn't matter.
// The function also collects all method declarations that belong to wrapper types
// so they can be replaced along with the type declaration.
//
// Returns:
//   - A slice of wrapper structs in declaration order, each containing all AST nodes
//     that need to be replaced during code generation
//   - An error for a call of a TODO function that can't be rewritten
func findFunctionWrappers(_ *token.FileSet, file *ast.File, target *targetFile) ([]*wrapper, error) {
	// The names of the go-function package for the TODO functions
	functionNames := make(map[string]struct{})
	for _, imp := range target.imports {
		if imp.Path == functionImportPath && imp.Name != "_" {
			functionNames[cmp.Or(imp.Name, "function")] = struct{}{}
		}
	}
	if len(functionNames) == 0 {
		// The import of go-function is added if needed
		functionNames["function"] = struct{}{}
	}

	ordered := make([]*wrapper, 0)
	named := make(map[string]*wrapper)
	typed := make(map[string]*wrapper)

	addNamed := func(varName string) (*wrapper, error) {
		if named[varName] != nil {
			return nil, fmt.Errorf("function wrapper %s is declared twice", varName)
		}
		w := &wrapper{VarName: varName}
		ordered = append(ordered, w)
		named[varName] = w
		return w, nil
	}

	// First pass: vars
	for _, decl := range file.Decls {
		decl, ok := decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR {
			continue
		}

		if len(decl.Specs) == 1 {
			// Example:
			//   // documentCanUserRead wraps document.CanUserRead as function.Wrapper (generated code)
			//   var documentCanUserRead documentCanUserReadT
			valueSpec := decl.Specs[0].(*ast.ValueSpec)
			if len(valueSpec.Names) == 1 && len(valueSpec.Values) == 0 {
				implVarName := valueSpec.Names[0].Name
				wrappedFunc, implements, err := parseImplementsComment(implVarName, decl.Doc.Text())
				if err != nil {
					continue
				}
				w, err := addNamed(implVarName)
				if err != nil {
					return nil, err
				}
				w.WrappedFunc = wrappedFunc
				w.Impl = implements
				w.Type = astvisit.ExprString(valueSpec.Type)
				if decl.Doc != nil {
					w.Nodes = append(w.Nodes, decl.Doc)
				}
				w.Nodes = append(w.Nodes, decl)
				typed[w.Type] = w
				continue
			}
		}

		// TODO calls like:
		//   var documentCanUserRead = function.WrapperTODO(document.CanUserRead)
		var declWrappers []*wrapper
		for _, spec := range decl.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			var specWrapper *wrapper
			for _, value := range valueSpec.Values {
				implements, wrappedFunc, isTODO, err := todoCall(value, functionNames)
				if err != nil {
					return nil, err
				}
				if !isTODO {
					continue
				}
				if len(valueSpec.Names) != 1 || len(valueSpec.Values) != 1 {
					names := make([]string, len(valueSpec.Names))
					for i, name := range valueSpec.Names {
						names[i] = name.Name
					}
					return nil, fmt.Errorf("can't generate function wrappers for the declaration of %s, declare every wrapper variable separately", strings.Join(names, ", "))
				}
				specWrapper, err = addNamed(valueSpec.Names[0].Name)
				if err != nil {
					return nil, err
				}
				specWrapper.WrappedFunc = wrappedFunc
				specWrapper.Impl = implements
				// A previously generated type is replaced
				specWrapper.Type = specWrapper.VarName + "T"
				typed[specWrapper.Type] = specWrapper
			}
			if specWrapper == nil {
				continue
			}
			declWrappers = append(declWrappers, specWrapper)
			if len(decl.Specs) > 1 {
				// Remove the spec from the var block,
				// the generated code is inserted after the block
				specNodes := astvisit.NodeRange{spec}
				if valueSpec.Doc != nil {
					specNodes = append(specNodes, valueSpec.Doc)
				}
				if valueSpec.Comment != nil {
					specNodes = append(specNodes, valueSpec.Comment)
				}
				specWrapper.Nodes = append(specWrapper.Nodes, specNodes)
				specWrapper.InsertAfter = decl
			}
		}
		switch {
		case len(declWrappers) == 0:
			continue
		case len(declWrappers) == len(decl.Specs):
			// Replace the whole declaration
			w := declWrappers[0]
			if len(decl.Specs) > 1 {
				for _, w := range declWrappers {
					w.Nodes = nil
				}
			}
			if decl.Doc != nil {
				w.Nodes = append(w.Nodes, decl.Doc)
			}
			w.Nodes = append(w.Nodes, decl)
		}
	}

	// Second pass: types
	claimedTypes := make(map[string]*wrapper)
	for _, decl := range file.Decls {
		decl, ok := decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.TYPE || len(decl.Specs) != 1 {
			continue
		}
		// ast.Print(fset, decl)
		typeSpec, ok := decl.Specs[0].(*ast.TypeSpec)
		if !ok || typeSpec.TypeParams != nil || astvisit.ExprString(typeSpec.Type) != "struct{}" {
			continue
		}
		implTypeName := typeSpec.Name.Name
		// Example:
		//   // documentCanUserReadT wraps document.CanUserRead as function.Wrapper (generated code)
		//   type documentCanUserReadT struct{}
		wrappedFunc, implements, err := parseImplementsComment(implTypeName, decl.Doc.Text())
		if err != nil {
			continue
		}
		w := typed[implTypeName]
		if w == nil {
			// No var with that type declared,
			// the type is instantiated by the user with {}
			w = &wrapper{
				WrappedFunc: wrappedFunc,
				Type:        implTypeName,
				Impl:        implements,
			}
			ordered = append(ordered, w)
			typed[implTypeName] = w
		}
		// The var of the type defines the wrapped function and interfaces
		if decl.Doc != nil {
			w.Nodes = append(w.Nodes, decl.Doc)
		}
		w.Nodes = append(w.Nodes, decl)
		claimedTypes[implTypeName] = w
	}

	// Third pass: methods of the wrapper types
	for _, decl := range file.Decls {
		decl, ok := decl.(*ast.FuncDecl)
		if !ok || decl.Recv.NumFields() != 1 {
			continue
		}
		recvType := astvisit.ExprString(decl.Recv.List[0].Type)
		w := claimedTypes[recvType]
		if w == nil {
			continue
		}
		if decl.Doc != nil {
			w.Nodes = append(w.Nodes, decl.Doc)
		}
		w.Nodes = append(w.Nodes, decl)
	}

	return ordered, nil
}

// todoCall returns the interfaces and the wrapped function of a call
// of a TODO function of go-function like function.WrapperTODO(MyFunction),
// and isTODO false if expr is no such call.
// functionNames are the names that the file uses for the go-function package,
// "." for a dot import.
// Returns an error for a call of go-function that ends with TODO but can't be rewritten.
func todoCall(expr ast.Expr, functionNames map[string]struct{}) (impl Impl, wrappedFunc string, isTODO bool, err error) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return 0, "", false, nil
	}
	var funcName string
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		if !ok {
			return 0, "", false, nil
		}
		if _, ok := functionNames[x.Name]; !ok {
			return 0, "", false, nil
		}
		funcName = fun.Sel.Name
	case *ast.Ident:
		if _, ok := functionNames["."]; !ok {
			return 0, "", false, nil
		}
		funcName = fun.Name
	default:
		return 0, "", false, nil
	}
	interfaceName, ok := strings.CutSuffix(funcName, "TODO")
	if !ok {
		return 0, "", false, nil
	}
	impl, err = ImplFromString("function." + interfaceName)
	if err != nil {
		if _, isIdent := call.Fun.(*ast.Ident); isIdent {
			// With a dot import it can be a function of the file's package
			return 0, "", false, nil
		}
		return 0, "", true, fmt.Errorf("%s is no TODO function of go-function", astvisit.ExprString(call.Fun))
	}
	if len(call.Args) != 1 {
		return 0, "", true, fmt.Errorf("%s needs the function to wrap as single argument", astvisit.ExprString(call.Fun))
	}
	return impl, astvisit.ExprString(call.Args[0]), true, nil
}

// parseImplementsComment extracts wrapper information from a generated code comment.
// The comment format is:
//
//	{implementor} wraps {function} as {interface} (generated code)
//
// Parameters:
//   - implementor: Expected name of the wrapper (var or type name)
//   - comment: The full comment text to parse
//
// Returns:
//   - wrappedFunc: Name of the function being wrapped (e.g., "pkg.MyFunc")
//   - impl: Which wrapper interfaces to implement
//   - err: Error if comment doesn't match expected format
//
// Examples:
//
//	// documentCanUserRead wraps document.CanUserRead as function.Wrapper (generated code)
//	var documentCanUserRead documentCanUserReadT
//
//	// documentCanUserReadT wraps document.CanUserRead as function.Wrapper (generated code)
//	type documentCanUserReadT struct{}
func parseImplementsComment(implementor, comment string) (wrappedFunc string, impl Impl, err error) {
	comment = strings.TrimSuffix(strings.TrimSpace(comment), " (generated code)")
	prefix := implementor + " wraps "
	asPos := strings.Index(comment, " as ")
	if !strings.HasPrefix(comment, prefix) || asPos <= len(prefix) {
		return "", 0, errors.New("no implementation comment")
	}
	wrappedFunc = comment[len(prefix):asPos]
	impl, err = ImplFromString(comment[asPos+len(" as "):])
	if err != nil {
		return "", 0, err
	}
	return wrappedFunc, impl, nil
}
