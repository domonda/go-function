package gen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ungerik/go-astvisit"
)

func RewriteDir(path string, verbose bool, printOnly io.Writer) (err error) {
	recursive := strings.HasSuffix(path, "...")
	if recursive {
		path = filepath.Clean(strings.TrimSuffix(path, "..."))
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		return RewriteFile(path, verbose, printOnly)
	}

	fset := token.NewFileSet()
	pkg, err := astvisit.ParsePackage(fset, path, filterOutTests)
	if err != nil && (!recursive || !errors.Is(err, astvisit.ErrPackageNotFound)) {
		return err
	}
	if err == nil {
		for fileName, file := range pkg.Files {
			err = RewriteAstFile(fset, pkg, file, fileName, verbose, printOnly)
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

	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	for _, file := range files {
		fileName := file.Name()
		if !file.IsDir() || fileName[0] == '.' || fileName == "node_modules" {
			continue
		}
		err = RewriteDir(filepath.Join(path, fileName, "..."), verbose, printOnly)
		if err != nil {
			return err
		}
	}
	return nil
}

func RewriteFile(filePath string, verbose bool, printOnly io.Writer) (err error) {
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
	return RewriteAstFile(fset, pkg, pkg.Files[filePath], filePath, verbose, printOnly)
}

func RewriteAstFile(fset *token.FileSet, filePkg *ast.Package, file *ast.File, filePath string, verbose bool, printTo io.Writer) (err error) {
	// ast.Print(fset, file)

	wrappers := findWrappers(fset, file)
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
	functions, err := localAndImportedFunctions(fset, filePkg, file, pkgDir)
	if err != nil {
		return err
	}

	importLines := map[string]struct{}{
		// `"reflect"`: {},
		// `"context"`: {},
		// `function "github.com/domonda/go-function"`: {},
	}

	var replacements astvisit.NodeReplacements
	for _, wrapper := range wrappers {
		importName, funcName := wrapper.WrappedFuncPkgAndFuncName()
		referencedPkg, ok := functions[importName]
		if !ok {
			return fmt.Errorf("can't find package %s in imports of file %s", importName, filePath)
		}
		wrappedFunc, ok := referencedPkg.Funcs[funcName]
		if !ok {
			return fmt.Errorf("can't find function %s in package %s", funcName, importName)
		}

		err = gatherFunctionImports(file, wrappedFunc.Decl.Type, importLines)
		if err != nil {
			return err
		}

		var repl strings.Builder
		// fmt.Fprintf(&newSrc, "////////////////////////////////////////\n")
		// fmt.Fprintf(&newSrc, "// %s\n\n", impl.WrappedFunc)
		fmt.Fprintf(&repl, "// %s wraps %s as %s (generated code)\n", wrapper.VarName, wrapper.WrappedFunc, wrapper.Impl)
		fmt.Fprintf(&repl, "var %[1]s %[1]sT\n\n", wrapper.VarName)
		err = wrapper.Impl.WriteFunctionWrapper(&repl, file, wrappedFunc.Decl, wrapper.VarName+"T", importName, importLines)
		if err != nil {
			return err
		}

		var implReplacements astvisit.NodeReplacements
		for i, node := range wrapper.Nodes {
			if i == 0 {
				implReplacements.AddReplacement(node, repl.String())
			} else {
				implReplacements.AddRemoval(node)
			}
		}
		replacements.Add(implReplacements)
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	rewritten, err := replacements.Apply(fset, source)
	if err != nil {
		return err
	}
	rewritten, err = format.Source(rewritten)
	if err != nil {
		return err
	}

	// imports.LocalPrefix = "github.com/domonda/"
	// rewritten, err = imports.Process(filePath, rewritten, &imports.Options{Comments: true, FormatOnly: true})
	// if err != nil {
	// 	return err
	// }

	// if verbose {
	// 	var sortedImportLines []string
	// 	for l := range importLines {
	// 		sortedImportLines = append(sortedImportLines, l)
	// 	}
	// 	sort.Strings(sortedImportLines)
	// 	fmt.Println("imports:", sortedImportLines)
	// }

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
	return ioutil.WriteFile(filePath, rewritten, 0660)
}

type wrapper struct {
	VarName     string
	WrappedFunc string
	Type        string
	Nodes       []ast.Node
	Impl        Impl
}

func (impl *wrapper) WrappedFuncPkgAndFuncName() (pkgName, funcName string) {
	dot := strings.IndexByte(impl.WrappedFunc, '.')
	if dot == -1 {
		return "", impl.WrappedFunc
	}
	return impl.WrappedFunc[:dot], impl.WrappedFunc[dot+1:]
}

func findWrappers(fset *token.FileSet, file *ast.File) []*wrapper {
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

// parseImplementsComment parses a comment that indicates the wrapped function
// and what interface is implemented
//
// Example:
//   // documentCanUserRead wraps document.CanUserRead as function.Wrapper (generated code)
//   var documentCanUserRead documentCanUserReadT
// or:
//   // documentCanUserReadT wraps document.CanUserRead as function.Wrapper (generated code)
//   type documentCanUserReadT struct{}
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

type packageFuncs struct {
	Location *astvisit.PackageLocation
	Funcs    map[string]funcDeclInFile
}

func localAndImportedFunctions(fset *token.FileSet, filePkg *ast.Package, file *ast.File, pkgDir string) (map[string]packageFuncs, error) {
	pkgFuncs := make(map[string]funcDeclInFile)
	for _, f := range filePkg.Files {
		for _, decl := range f.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				pkgFuncs[funcDecl.Name.Name] = funcDeclInFile{
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
			Funcs: pkgFuncs,
		},
	}

	for _, imp := range file.Imports {
		importName, pkgLocation, err := astvisit.ImportSpecInfo(pkgDir, imp)
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
				if ok && funcDecl.Name.IsExported() {
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

func gatherFunctionImports(file *ast.File, funcType *ast.FuncType, outImportLines map[string]struct{}) error {
	funcSelectors := make(map[string]struct{})
	recursiveExprSelectors(funcType, funcSelectors)
	// fmt.Println(funcSelectors)
	for _, imp := range file.Imports {
		if imp.Name != nil {
			if _, ok := funcSelectors[imp.Name.Name]; ok {
				delete(outImportLines, imp.Path.Value)
				outImportLines[imp.Name.Name+" "+imp.Path.Value] = struct{}{}
			}
			continue
		}
		guessedName, err := guessPackageNameFromPath(imp.Path.Value)
		if err != nil {
			return err
		}
		if _, ok := funcSelectors[guessedName]; ok {
			if _, ok = outImportLines[guessedName+" "+imp.Path.Value]; !ok {
				outImportLines[imp.Path.Value] = struct{}{}
			}
		}
	}
	return nil
}

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
