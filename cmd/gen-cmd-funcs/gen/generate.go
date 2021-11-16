package gen

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"

	"golang.org/x/tools/imports"
)

func PackageFunctions(pkgDir, genFilename, namePrefix string, printOnly bool, onlyFuncs ...string) error {
	pkgName, funcs, err := parsePackage(pkgDir, genFilename, onlyFuncs...)
	if err != nil {
		return err
	}

	importLines := map[string]struct{}{
		`"reflect"`: {},
		`"context"`: {},
		`function "github.com/domonda/go-function"`: {},
	}
	for _, fun := range funcs {
		err = gatherFunctionImports(fun.File, fun.Decl.Type, importLines)
		if err != nil {
			return err
		}
	}
	var sortedImportLines []string
	for l := range importLines {
		sortedImportLines = append(sortedImportLines, l)
	}
	sort.Strings(sortedImportLines)

	b := bytes.NewBuffer(nil)

	fmt.Fprintf(b, "// This file has been AUTOGENERATED!\n\n")
	fmt.Fprintf(b, "package %s\n\n", pkgName)
	if len(sortedImportLines) > 0 {
		fmt.Fprintf(b, "import (\n")
		for _, importLine := range sortedImportLines {
			fmt.Fprintf(b, "\t%s\n", importLine)
		}
		fmt.Fprintf(b, ")\n\n")
	}

	for funName, fun := range funcs {
		err = ImplWrapper.WriteFunctionWrapper(b, fun.File, fun.Decl, namePrefix+funName, "", importLines)
		if err != nil {
			return err
		}
	}

	genFileData := b.Bytes()
	genFilePath := filepath.Join(pkgDir, genFilename)

	imports.LocalPrefix = "github.com/domonda/"
	genFileData, err = imports.Process(genFilePath, genFileData, &imports.Options{Comments: true, FormatOnly: true})
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Println(genFileData)
	} else {
		fmt.Println("Writing file", genFilePath)
		err = ioutil.WriteFile(genFilePath, genFileData, 0660)
		if err != nil {
			return err
		}
	}
	// err = exec.Command("gofmt", "-s", "-w", genFile).Run()
	// if err != nil {
	// 	return err
	// }

	return nil
}
