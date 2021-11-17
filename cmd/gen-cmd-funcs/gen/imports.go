package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/ungerik/go-astvisit"
)

type packageFuncs struct {
	Location *astvisit.PackageLocation
	Funcs    map[string]funcDeclInFile
}

// localAndImportedFunctions returns a map of packageFuncs with the package
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
