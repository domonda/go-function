package gen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// RewriteDir processes a directory (and optionally subdirectories) to rewrite wrapper declarations.
// This is the main entry point for the code generator when invoked on directories.
//
// Parameters:
//   - path: Directory path to process; append "..." for recursive processing
//   - verbose: If true, print detailed information about processing
//   - printOnly: If not nil, print generated code to this writer instead of modifying files
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON unmarshalling
//   - localImportPrefixes: Import path prefixes to treat as "local" for import grouping
//
// Returns:
//   - error if any file processing fails
//
// The function:
//  1. Checks if path ends with "..." for recursive processing
//  2. Processes all Go files in the directory
//  3. If recursive, processes all subdirectories (excluding hidden dirs and node_modules)
//  4. Skips test files (_test.go)
//
// Example:
//
//	err := RewriteDir("./pkg/mypackage/...", true, nil, nil, []string{"github.com/myorg/"})
func RewriteDir(path string, verbose bool, printOnly io.Writer, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	recursive := strings.HasSuffix(path, "...")
	if recursive {
		path = filepath.Clean(strings.TrimSuffix(path, "..."))
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		return RewriteFile(path, verbose, printOnly, jsonTypeReplacements, localImportPrefixes)
	}

	fset := token.NewFileSet()
	pkg, err := astvisit.ParsePackage(fset, path, filterOutTests)
	if err != nil && (!recursive || !errors.Is(err, astvisit.ErrPackageNotFound)) {
		return err
	}
	if err == nil {
		for fileName, file := range pkg.Files {
			err = RewriteAstFile(fset, pkg, file, fileName, verbose, printOnly, jsonTypeReplacements, localImportPrefixes)
			if err != nil {
				return err
			}
		}
	} else if verbose {
		fmt.Println(err)
	}
	if !recursive {
		return nil
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, file := range files {
		fileName := file.Name()
		if !file.IsDir() || fileName[0] == '.' || fileName == "node_modules" {
			continue
		}
		err = RewriteDir(filepath.Join(path, fileName, "..."), verbose, printOnly, jsonTypeReplacements, localImportPrefixes)
		if err != nil {
			return err
		}
	}
	return nil
}

// RewriteFile processes a single Go source file to rewrite wrapper declarations.
//
// Parameters:
//   - filePath: Path to the Go source file to process
//   - verbose: If true, print detailed information about processing
//   - printOnly: If not nil, print generated code to this writer instead of modifying file
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
func RewriteFile(filePath string, verbose bool, printOnly io.Writer, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
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
	return RewriteAstFile(fset, pkg, pkg.Files[filePath], filePath, verbose, printOnly, jsonTypeReplacements, localImportPrefixes)
}

// RewriteAstFile is the core rewriting logic that processes an AST file.
// This function performs the actual wrapper code generation and replacement.
//
// Parameters:
//   - fset: Token file set for position information
//   - filePkg: The package containing the file
//   - astFile: The parsed AST of the file to process
//   - filePath: Original file path (for error messages and writing)
//   - verbose: If true, print detailed information
//   - printTo: If not nil, print to this writer instead of modifying file
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
func RewriteAstFile(fset *token.FileSet, filePkg *ast.Package, astFile *ast.File, filePath string, verbose bool, printTo io.Writer, jsonTypeReplacements map[string]string, localImportPrefixes []string) (err error) {
	filePath = filepath.Clean(filePath)

	// ast.Print(fset, file)
	wrappers := findFunctionWrappers(fset, astFile)
	if len(wrappers) == 0 {
		if verbose {
			fmt.Println("no wrappers found to rewrite in", filePath)
		}
		return nil
	}

	pkgDir := filepath.Dir(filePath)

	// Gather imported packages of file
	// and parse packages for function declarations
	// that could be referenced by function.Wrapper implementations
	// Also parse all functions of the file's package
	// because they could als be referenced with an empty import name.
	// Added with empty string as package/import name.
	functions, err := localAndImportedFunctions(fset, filePkg, astFile, pkgDir)
	if err != nil {
		return err
	}

	neededImportLines := make(map[string]struct{})

	var replacements astvisit.NodeReplacements
	for _, wrapper := range wrappers {
		wrappedFuncPackage, wrappedFuncName := wrapper.WrappedFuncPkgAndFuncName()
		referencedPkg, ok := functions[wrappedFuncPackage]
		if !ok {
			return fmt.Errorf("can't find package %s in imports of file %s", wrappedFuncPackage, filePath)
		}
		wrappedFunc, ok := referencedPkg.Funcs[wrappedFuncName]
		if !ok {
			return fmt.Errorf("can't find function %s in package %s", wrappedFuncName, wrappedFuncPackage)
		}

		var repl strings.Builder
		// fmt.Fprintf(&newSrc, "////////////////////////////////////////\n")
		// fmt.Fprintf(&newSrc, "// %s\n\n", impl.WrappedFunc)
		fmt.Fprintf(&repl, "// %s wraps %s as %s (generated code)\n", wrapper.VarName, wrapper.WrappedFunc, wrapper.Impl)
		fmt.Fprintf(&repl, "var %[1]s %[1]sT\n\n", wrapper.VarName)
		err = wrapper.Impl.WriteFunctionWrapper(&repl, wrappedFunc.File, wrappedFunc.Decl, wrapper.VarName+"T", wrappedFuncPackage, neededImportLines, jsonTypeReplacements)
		if err != nil {
			return err
		}

		var implReplacements astvisit.NodeReplacements
		debugID := "Wrapper for " + wrapper.WrappedFunc
		for i, node := range wrapper.Nodes {
			if i == 0 {
				implReplacements.AddReplacement(node, repl.String(), debugID)
			} else {
				implReplacements.AddRemoval(node, debugID)
			}
		}
		replacements.Add(implReplacements)
	}

	source, err := os.ReadFile(filePath) //#nosec G304
	if err != nil {
		return err
	}
	rewritten, err := replacements.Apply(fset, source)
	if err != nil {
		return err
	}
	// rewritten, err = format.Source(rewritten)
	// if err != nil {
	// 	return err
	// }

	// Parse rewritten again to add missing imports
	// to the ast.File and pretty print the result
	rewritten, err = astvisit.FormatFileWithImports(fset, rewritten, neededImportLines, localImportPrefixes...)
	if err != nil {
		return err
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
	return os.WriteFile(filePath, rewritten, 0644)
}

// wrapper represents a function wrapper declaration found in source code.
// It contains all information needed to generate the wrapper implementation.
type wrapper struct {
	VarName     string      // Name of the wrapper variable (e.g., "myWrapper")
	WrappedFunc string      // Full name of the wrapped function (e.g., "pkg.MyFunc" or "MyFunc")
	Type        string      // Name of the wrapper type (e.g., "myWrapperT")
	Nodes       []ast.Node  // All AST nodes to be replaced (comments, var, type, methods)
	Impl        Impl        // Which wrapper interfaces to implement
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
// Pattern 1: TODO function call
//
//	var myWrapper = function.WrapperTODO(MyFunction)
//
// Pattern 2: Implementation comment with var and type
//
//	// myWrapper wraps MyFunction as function.Wrapper (generated code)
//	var myWrapper myWrapperT
//
//	// myWrapperT wraps MyFunction as function.Wrapper (generated code)
//	type myWrapperT struct{}
//
// The function also collects all method declarations that belong to wrapper types
// so they can be replaced along with the type declaration.
//
// Returns:
//   - A slice of wrapper structs in declaration order, each containing all AST nodes
//     that need to be replaced during code generation
func findFunctionWrappers(_ *token.FileSet, file *ast.File) []*wrapper {
	ordered := make([]*wrapper, 0)
	named := make(map[string]*wrapper)
	typed := make(map[string]*wrapper)

	for _, decl := range file.Decls {
		// ast.Print(fset, decl)
		switch decl := decl.(type) {
		case *ast.GenDecl:
			if len(decl.Specs) != 1 {
				continue
			}
			switch decl.Tok {
			case token.VAR:
				valueSpec, ok := decl.Specs[0].(*ast.ValueSpec)
				if !ok || len(valueSpec.Names) != 1 {
					continue
				}
				implVarName := valueSpec.Names[0].Name

				if len(valueSpec.Values) == 0 {
					// Example:
					//   // documentCanUserRead wraps document.CanUserRead as function.Wrapper (generated code)
					//   var documentCanUserRead documentCanUserReadT
					wrappedFunc, implements, err := parseImplementsComment(implVarName, decl.Doc.Text())
					if err != nil {
						continue
					}
					impl := named[implVarName]
					if impl == nil {
						impl = new(wrapper)
						ordered = append(ordered, impl)
						named[implVarName] = impl
					}
					impl.VarName = implVarName
					impl.WrappedFunc = wrappedFunc
					impl.Impl |= implements
					impl.Type = astvisit.ExprString(valueSpec.Type)
					if decl.Doc != nil {
						impl.Nodes = append(impl.Nodes, decl.Doc)
					}
					impl.Nodes = append(impl.Nodes, decl)

					typed[impl.Type] = impl
					continue
				}

				if len(valueSpec.Values) != 1 {
					continue
				}
				callExpr, ok := valueSpec.Values[0].(*ast.CallExpr)
				if !ok || len(callExpr.Args) != 1 {
					continue
				}
				todoFunc := astvisit.ExprString(callExpr.Fun)
				if !strings.HasSuffix(todoFunc, "TODO") {
					continue
				}
				implements, err := ImplFromString(strings.TrimSuffix(todoFunc, "TODO"))
				if err != nil {
					continue
				}
				impl := named[implVarName]
				if impl == nil {
					impl = new(wrapper)
					ordered = append(ordered, impl)
					named[implVarName] = impl
				}
				impl.VarName = implVarName
				impl.WrappedFunc = astvisit.ExprString(callExpr.Args[0])
				impl.Impl |= implements
				if decl.Doc != nil {
					impl.Nodes = append(impl.Nodes, decl.Doc)
				}
				impl.Nodes = append(impl.Nodes, decl)

			case token.TYPE:
				// ast.Print(fset, decl)
				typeSpec, ok := decl.Specs[0].(*ast.TypeSpec)
				if !ok || astvisit.ExprString(typeSpec.Type) != "struct{}" {
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
				impl := typed[implTypeName]
				if impl == nil {
					impl = new(wrapper)
					ordered = append(ordered, impl)
					typed[implTypeName] = impl
					impl.Type = implTypeName
					// No var with that type declared
					// so also use the type like a var
					// and let the user instanciate the type with {}
					named[implTypeName] = impl
					impl.VarName = implTypeName
				}
				impl.WrappedFunc = wrappedFunc
				impl.Impl |= implements
				if decl.Doc != nil {
					impl.Nodes = append(impl.Nodes, decl.Doc)
				}
				impl.Nodes = append(impl.Nodes, decl)
			}

		case *ast.FuncDecl:
			if decl.Recv.NumFields() != 1 {
				continue
			}
			recvType := astvisit.ExprString(decl.Recv.List[0].Type)
			impl := typed[recvType]
			if impl == nil {
				continue
			}
			if decl.Doc != nil {
				impl.Nodes = append(impl.Nodes, decl.Doc)
			}
			impl.Nodes = append(impl.Nodes, decl)
		}
	}

	return ordered
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
