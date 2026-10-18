package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ungerik/go-astvisit"
)

// Test_gatherFieldListImports tests which imports a wrapped function's
// parameter types add to the generated file and how their package
// qualifiers get renamed so the generated code compiles in the target file.
func Test_gatherFieldListImports(t *testing.T) {
	tests := []struct {
		name        string
		funcSource  string // file declaring func F whose params are gathered
		targetSrc   string // file the wrapper is generated into, empty for none
		initial     astvisit.Imports
		wantImports astvisit.Imports
		wantRemap   map[string]string
	}{
		{
			// The import path must be unquoted, a quoted path is no valid import for FormatSource.
			// Imports of the source file that the params don't use must not be added.
			name:        "unaliased import is added by unquoted path",
			funcSource:  "package p\n\nimport (\n\t\"fmt\"\n\t\"go/token\"\n)\n\nfunc F(pos token.Pos) { fmt.Println(pos) }\n",
			wantImports: astvisit.Imports{{Path: "go/token"}: {}},
			wantRemap:   map[string]string{},
		},
		{
			// The generated code refers to tok.Pos, so the alias has to be imported too.
			name:        "alias of source file is preserved",
			funcSource:  "package p\n\nimport tok \"go/token\"\n\nfunc F(pos tok.Pos) {}\n",
			wantImports: astvisit.Imports{{Name: "tok", Path: "go/token"}: {}},
			wantRemap:   map[string]string{},
		},
		{
			// The target file uses the name token for another package,
			// importing go/token as token would not compile.
			name:        "name used by target for another package gets alternative alias",
			funcSource:  "package p\n\nimport \"go/token\"\n\nfunc F(pos token.Pos) {}\n",
			targetSrc:   "package t\n\nimport token \"go/scanner\"\n",
			wantImports: astvisit.Imports{{Name: "token2", Path: "go/token"}: {}},
			wantRemap:   map[string]string{"token": "token2"},
		},
		{
			name:        "alternative alias skips names used by target",
			funcSource:  "package p\n\nimport \"go/token\"\n\nfunc F(pos token.Pos) {}\n",
			targetSrc:   "package t\n\nimport (\n\ttoken \"go/scanner\"\n\ttoken2 \"go/types\"\n)\n",
			wantImports: astvisit.Imports{{Name: "token3", Path: "go/token"}: {}},
			wantRemap:   map[string]string{"token": "token3"},
		},
		{
			// Importing the path a second time would be redundant,
			// the generated code has to use the target's name instead.
			name:        "path imported by target under other name is reused",
			funcSource:  "package p\n\nimport \"go/token\"\n\nfunc F(pos token.Pos) {}\n",
			targetSrc:   "package t\n\nimport gotoken \"go/token\"\n",
			wantImports: astvisit.Imports{},
			wantRemap:   map[string]string{"token": "gotoken"},
		},
		{
			// Another wrapper of the target file needs the import without alias,
			// so the import with alias is added beside it.
			name:        "alias with other name is added beside import without alias",
			funcSource:  "package p\n\nimport tok \"go/token\"\n\nfunc F(pos tok.Pos) {}\n",
			initial:     astvisit.Imports{{Path: "go/token"}: {}},
			wantImports: astvisit.Imports{{Path: "go/token"}: {}, {Name: "tok", Path: "go/token"}: {}},
			wantRemap:   map[string]string{},
		},
		{
			// Both imports would declare the name token, so the alias replaces the other.
			name:        "alias with package name replaces import without alias",
			funcSource:  "package p\n\nimport token \"go/token\"\n\nfunc F(pos token.Pos) {}\n",
			initial:     astvisit.Imports{{Path: "go/token"}: {}},
			wantImports: astvisit.Imports{{Name: "token", Path: "go/token"}: {}},
			wantRemap:   map[string]string{},
		},
		{
			// The named import function "github.com/domonda/go-function" declares the package name,
			// adding the same package without alias would declare the name function twice.
			name:        "unaliased import is not added again when imported with its package name",
			funcSource:  "package p\n\nimport \"github.com/domonda/go-function\"\n\nfunc F(w function.Wrapper) {}\n",
			initial:     astvisit.Imports{{Name: "function", Path: "github.com/domonda/go-function"}: {}},
			wantImports: astvisit.Imports{{Name: "function", Path: "github.com/domonda/go-function"}: {}},
			wantRemap:   map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			funcFile, err := parser.ParseFile(fset, "func.go", tt.funcSource, 0)
			require.NoError(t, err)
			funcDecl := funcFile.Decls[len(funcFile.Decls)-1].(*ast.FuncDecl)

			// "." is the directory of this test package,
			// so the imports are resolved like the go command does there, here through go.work
			target := &targetFile{dir: ".", names: make(packageNames)}
			if tt.targetSrc != "" {
				parsedTarget, err := parser.ParseFile(fset, "target.go", tt.targetSrc, 0)
				require.NoError(t, err)
				target, err = newTargetFile(parsedTarget, ".", make(packageNames))
				require.NoError(t, err)
			}

			imports := make(astvisit.Imports)
			maps.Copy(imports, tt.initial)

			remap, err := gatherFieldListImports(funcFile, ".", funcDecl.Type.Params, imports, target)
			require.NoError(t, err)
			assert.Equal(t, tt.wantImports, imports, "imports")
			assert.Equal(t, tt.wantRemap, remap, "package remap")
		})
	}
}

// Test_gatherFieldListImports_PackageNamedFunction tests that the package
// of a parameter type that is declared as package function, but is not
// go-function, is imported with another name, because the generated code
// refers to go-function by the name function, also if the file
// doesn't import go-function yet, like a file with a wrapper declared
// by its comments, and that the generated code compiles and a second run
// doesn't change it, because CI runs -validate on the generated files.
func Test_gatherFieldListImports_PackageNamedFunction(t *testing.T) {
	const functionPath = "example.com/m/function"
	tests := map[string]string{
		"TODO call":                  "package m\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n",
		"declaration without import": "package m\n\n// fWrapper wraps F as function.Wrapper (generated code)\nvar fWrapper fWrapperT\n\n// fWrapperT wraps F as function.Wrapper (generated code)\ntype fWrapperT struct{}\n",
	}
	for name, wrappersSource := range tests {
		t.Run(name, func(t *testing.T) {
			dir := writeModule(t, map[string]string{
				"function/function.go": "package function\n\ntype Options struct{ Verbose bool }\n",
				"funcs.go":             "package m\n\nimport \"" + functionPath + "\"\n\nfunc F(opts function.Options, n int) {}\n",
				"wrappers.go":          wrappersSource,
			})
			wrappersFile := filepath.Join(dir, "wrappers.go")
			err := RewriteFile(wrappersFile, false, nil, false, nil, nil)
			require.NoError(t, err)
			generated, err := os.ReadFile(wrappersFile)
			require.NoError(t, err)
			assert.Contains(t, string(generated), "\tfunction2 \""+functionPath+"\"\n")
			assert.Contains(t, string(generated), "reflect.TypeFor[function2.Options]()")
			assert.Contains(t, string(generated), "\t\"github.com/domonda/go-function\"\n")
			assert.Contains(t, string(generated), "function.ScanString(", "go-function should keep the name function")

			fset := token.NewFileSet()
			pkg, err := astvisit.ParsePackage(fset, dir, filterOutTests)
			require.NoError(t, err)
			// The source importer resolves go-function through the go.work
			// of this repository, the package of the module without imports
			// is type checked without importer
			conf := types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
				if path != functionPath {
					return sourceImporter.Import(path)
				}
				functionPkg, err := astvisit.ParsePackage(fset, filepath.Join(dir, "function"), nil)
				if err != nil {
					return nil, err
				}
				return new(types.Config).Check(functionPath, fset, slices.Collect(maps.Values(functionPkg.Files)), nil)
			})}
			_, err = conf.Check(pkg.Name, fset, slices.Collect(maps.Values(pkg.Files)), nil)
			require.NoError(t, err, "generated code should compile:\n%s", generated)

			err = RewriteFile(wrappersFile, false, nil, true, nil, nil)
			assert.NoError(t, err, "a second run should not change the generated code")
		})
	}
}

// Test_localAndImportedFunctions tests that the functions a wrapper can reference
// are keyed by the name the file uses for their package, because wrappers like
// function.WrapperTODO(fn.ScanString) are resolved by that qualifier,
// and that only imports the wrappers can reference are located,
// because other imports may not be locatable on this platform.
func Test_localAndImportedFunctions(t *testing.T) {
	// "." is the directory of this test package,
	// so the imports are resolved like the go command does there, here through go.work
	localAndImportedFunctionsOfSource := func(t *testing.T, source string, importNames ...string) (map[string]packageFuncs, error) {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "test.go", source, 0)
		require.NoError(t, err)
		names := map[string]struct{}{"": {}}
		for _, name := range importNames {
			names[name] = struct{}{}
		}
		return localAndImportedFunctions(fset, "testpkg", map[string]*ast.File{"test.go": file}, file, ".", names)
	}

	t.Run("keys imported packages by import name", func(t *testing.T) {
		source := `package testpkg

import (
	"fmt"

	fn "github.com/domonda/go-function"
	"github.com/ungerik/go-astvisit"
)

func Local() string { return fmt.Sprint(fn.ScanString, astvisit.ExprString) }
`
		functions, err := localAndImportedFunctionsOfSource(t, source, "fmt", "fn", "astvisit")
		require.NoError(t, err)

		// "fmt" is a standard library package whose functions can be wrapped too,
		// "go-astvisit" is the last path element but not the package name,
		// "function" is the package name but the file imports it as fn
		require.Equal(t, []string{"", "astvisit", "fmt", "fn"}, slices.Sorted(maps.Keys(functions)))
		assert.Contains(t, functions["fmt"].Funcs, "Sprint")

		assert.Equal(t, "testpkg", functions[""].Location.Name)
		assert.Contains(t, functions[""].Funcs, "Local")

		assert.Equal(t, "function", functions["fn"].Location.Name)
		assert.Contains(t, functions["fn"].Funcs, "ScanString")

		assert.Equal(t, "astvisit", functions["astvisit"].Location.Name)
		assert.Contains(t, functions["astvisit"].Funcs, "ExprString")
	})

	t.Run("finds package named unlike its path", func(t *testing.T) {
		// The package name yaml can't be guessed from the path go.yaml.in/yaml/v3
		source := `package testpkg

import "go.yaml.in/yaml/v3"
`
		functions, err := localAndImportedFunctionsOfSource(t, source, "yaml")
		require.NoError(t, err)
		require.Contains(t, functions, "yaml")
		assert.Contains(t, functions["yaml"].Funcs, "Marshal")
	})

	t.Run("unreferenced imports are not located", func(t *testing.T) {
		// Like a cgo file or a file with packages for other platforms
		// whose wrappers only reference the function F of its package
		source := `package testpkg

import "C"

import (
	fn "example.com/does/not/exist/aliased"
	"example.com/does/not/exist"
)

func F() {}
`
		functions, err := localAndImportedFunctionsOfSource(t, source)
		require.NoError(t, err)
		assert.Equal(t, []string{""}, slices.Sorted(maps.Keys(functions)))
		assert.Contains(t, functions[""].Funcs, "F")
	})

	t.Run("unresolvable referenced import is an error", func(t *testing.T) {
		source := `package testpkg

import "example.com/does/not/exist"
`
		_, err := localAndImportedFunctionsOfSource(t, source, "exist")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "example.com/does/not/exist", "error should name the import path that can't be resolved")
	})
}

// Test_guessPackageNameFromPath tests the package names guessed from import paths,
// because a guessed name that differs from the package name
// makes the generator locate the package with the go command.
// The major version suffixes are the ones of module paths,
// so gopkg.in/yaml.v3 is named yaml, but the v1 of k8s.io/api/core/v1
// is no major version suffix and the name of its package.
func Test_guessPackageNameFromPath(t *testing.T) {
	tests := []struct {
		path    string
		want    string
		wantErr bool
	}{
		{path: "fmt", want: "fmt"},
		{path: `"go/token"`, want: "token"},
		{path: "github.com/domonda/go-function", want: "function"},
		// The last element of a major version path is no package name
		{path: "math/rand/v2", want: "rand"},
		{path: "example.com/x/mylib/v2", want: "mylib"},
		{path: "example.com/x/mylib/v10", want: "mylib"},
		{path: "example.com/x/v2go", want: "v2go"},
		{path: "gopkg.in/yaml.v3", want: "yaml"},
		{path: "k8s.io/api/core/v1", want: "v1"},
		{path: "example.com/my-pkg", wantErr: true},
		{path: "v2", want: "v2"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := guessPackageNameFromPath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test_localAndImportedFunctions_StdPackage tests that functions
// of standard library packages can be wrapped like functions
// of other imported packages, for example function.WrapperTODO(strings.ToUpper).
func Test_localAndImportedFunctions_StdPackage(t *testing.T) {
	source := `package testpkg

import "strings"

var _ = strings.ToUpper
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, 0)
	require.NoError(t, err)
	functions, err := localAndImportedFunctions(fset, "testpkg", map[string]*ast.File{"test.go": file}, file, ".", map[string]struct{}{"strings": {}})
	require.NoError(t, err)
	require.Contains(t, functions, "strings")
	assert.Contains(t, functions["strings"].Funcs, "ToUpper")
}

// Test_localAndImportedFunctions_PlatformFiles tests that a function
// declared in files for different platforms is taken from the file
// of the current platform, because ParsePackage parses the files
// of all platforms and a randomly picked declaration
// makes the generated code and -validate flaky.
func Test_localAndImportedFunctions_PlatformFiles(t *testing.T) {
	dir := t.TempDir()
	otherOS := []string{"plan9", "windows", "linux", "darwin"}
	otherOS = slices.DeleteFunc(otherOS, func(goos string) bool { return goos == runtime.GOOS })
	for _, goos := range append(otherOS[:2], runtime.GOOS) {
		source := "package p\n\nfunc F(" + goos + "Arg int) {}\n"
		err := os.WriteFile(filepath.Join(dir, "f_"+goos+".go"), []byte(source), 0600)
		require.NoError(t, err)
	}

	// Map iteration order is random, so parse the package several times
	for range 20 {
		fset := token.NewFileSet()
		pkg, err := astvisit.ParsePackage(fset, dir, nil)
		require.NoError(t, err)
		require.Len(t, pkg.Files, 3, "files of all platforms are parsed")
		var anyFile *ast.File
		for _, f := range pkg.Files {
			anyFile = f
		}
		functions, err := localAndImportedFunctions(fset, pkg.Name, pkg.Files, anyFile, dir, map[string]struct{}{"": {}})
		require.NoError(t, err)
		assert.Equal(t, []string{runtime.GOOS + "Arg"}, funcTypeArgNames(functions[""].Funcs["F"].Decl.Type))
	}
}

// Test_localAndImportedFunctions_ParsesOnlyReferencedPackages tests that
// the imports whose package name had to be located are only parsed
// if a wrapper references them, because parsing every other import
// of the file, like net/http, slows down the generator, and a package
// that doesn't parse must not fail wrappers that don't reference it.
func Test_localAndImportedFunctions_ParsesOnlyReferencedPackages(t *testing.T) {
	dir := writeModule(t, map[string]string{
		// The package name yaml can't be guessed from the path element yaml.v3,
		// so all imports without explicit name are located
		"yaml.v3/yaml.go": "package yaml\n\nfunc Marshal(v any) ([]byte, error) { return nil, nil }\n",
		// go list only reads the package clause and imports,
		// but parsing the package fails because of the syntax error
		"broken/broken.go": "package broken\n\nfunc F( {\n",
	})
	source := `package m

import (
	"example.com/m/broken"
	"example.com/m/yaml.v3"
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(dir, "m.go"), source, 0)
	require.NoError(t, err)
	importNames := map[string]struct{}{"": {}, "yaml": {}}
	functions, err := localAndImportedFunctions(fset, "m", map[string]*ast.File{filepath.Join(dir, "m.go"): file}, file, dir, importNames)
	require.NoError(t, err, "the unreferenced package broken should not be parsed")
	assert.Equal(t, []string{"", "yaml"}, slices.Sorted(maps.Keys(functions)))
	assert.Contains(t, functions["yaml"].Funcs, "Marshal")
}
