package gen

import (
	"fmt"
	"go/ast"
	"io"
	"strconv"
	"strings"

	"github.com/ungerik/go-astvisit"
)

type Impl int

const (
	ImplDescription Impl = 1 << iota
	ImplCallWrapper
	ImplCallWithStringsWrapper
	ImplCallWithNamedStringsWrapper
	ImplCallWithJSONWrapper

	ImplWrapper = ImplDescription | ImplCallWrapper | ImplCallWithStringsWrapper | ImplCallWithNamedStringsWrapper | ImplCallWithJSONWrapper
)

func ImplFromString(str string) (Impl, error) {
	switch str {
	case "function.Wrapper":
		return ImplWrapper, nil
	case "function.Description":
		return ImplDescription, nil
	case "function.CallWrapper":
		return ImplCallWrapper, nil
	case "function.CallWithStringsWrapper":
		return ImplCallWithStringsWrapper, nil
	case "function.CallWithNamedStringsWrapper":
		return ImplCallWithNamedStringsWrapper, nil
	case "function.ImplCallWithJSONWrapper":
		return ImplCallWithJSONWrapper, nil
	default:
		return 0, fmt.Errorf("can't implement %q", str)
	}
}

func (impl Impl) String() string {
	switch impl {
	case ImplWrapper:
		return "function.Wrapper"
	case ImplDescription:
		return "function.Description"
	case ImplCallWrapper:
		return "function.CallWrapper"
	case ImplCallWithStringsWrapper:
		return "function.CallWithStringsWrapper"
	case ImplCallWithNamedStringsWrapper:
		return "function.CallWithNamedStringsWrapper"
	case ImplCallWithJSONWrapper:
		return "function.CallWithJSONWrapper"
	default:
		return fmt.Sprintf("Impl(%d)", impl)
	}
}

func (impl Impl) WriteFunctionWrapper(w io.Writer, funcFile *ast.File, funcDecl *ast.FuncDecl, implType, funcPackage string, neededImportLines map[string]struct{}, jsonTypeReplacements map[string]string) error {
	var (
		argNames        = funcTypeArgNames(funcDecl.Type)
		argDescriptions = funcDeclArgDescriptions(funcDecl)
		argTypes        = funcTypeArgTypes(funcDecl.Type, funcPackage)
		numArgs         = len(argTypes)
		resultTypes     = funcTypeResultTypes(funcDecl.Type, funcPackage)
		numResults      = len(resultTypes)
		hasContextArg   = numArgs > 0 && argTypes[0] == "context.Context"
		hasErrorResult  = numResults > 0 && resultTypes[numResults-1] == "error"
		funcPackageSel  = ""
	)
	if funcPackage != "" {
		funcPackageSel = funcPackage + "."
	}

	writeFuncCall := func(args []string) {
		numResultsWithoutErr := numResults
		if hasErrorResult {
			numResultsWithoutErr--
		}
		if numResultsWithoutErr > 0 {
			fmt.Fprintf(w, "\tresults = make([]interface{}, %d)\n", numResultsWithoutErr)
		}
		fmt.Fprintf(w, "\t")
		for i := 0; i < numResultsWithoutErr; i++ {
			if i > 0 {
				fmt.Fprintf(w, ", ")
			}
			fmt.Fprintf(w, "results[%d]", i)
		}
		if hasErrorResult {
			if numResultsWithoutErr == 0 {
				fmt.Fprintf(w, "err")
			} else {
				fmt.Fprintf(w, ", err")
			}
		}
		if numResults > 0 {
			fmt.Fprintf(w, " = ")
		}
		ellipsis := ""
		if numArgs > 0 && strings.HasPrefix(argTypes[numArgs-1], "...") {
			ellipsis = "..."
		}
		fmt.Fprintf(w, "%s%s(%s%s) // wrapped call\n", funcPackageSel, funcDecl.Name.Name, strings.Join(args, ", "), ellipsis)
		if numResults > 0 {
			fmt.Fprintf(w, "\treturn results, err\n")
		} else {
			fmt.Fprintf(w, "\treturn nil, nil\n")
		}
	}

	fmt.Fprintf(w, "// %s wraps %s%s as %s (generated code)\n", implType, funcPackageSel, funcDecl.Name.Name, impl)
	fmt.Fprintf(w, "type %s struct{}\n\n", implType)

	// Always implement fmt.Stringer
	fmt.Fprintf(w, "func (%s) String() string {\n", implType)
	fmt.Fprintf(w, "\treturn \"%s%s%s\"\n", funcPackageSel, funcDecl.Name.Name, astvisit.FuncTypeString(funcDecl.Type))
	fmt.Fprintf(w, "}\n\n")

	// Always get imports of function arguments
	err := gatherFieldListImports(funcFile, funcDecl.Type.Params, neededImportLines)
	if err != nil {
		return err
	}

	if impl&ImplDescription != 0 {
		neededImportLines[`"reflect"`] = struct{}{}

		// Get imports of results only for function.Description.ArgTypes() method
		err = gatherFieldListImports(funcFile, funcDecl.Type.Results, neededImportLines)
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "func (%s) Name() string {\n", implType)
		fmt.Fprintf(w, "\treturn \"%s\"\n", funcDecl.Name.Name)
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) NumArgs() int      { return %d }\n", implType, numArgs)
		fmt.Fprintf(w, "func (%s) ContextArg() bool  { return %t }\n", implType, hasContextArg)
		fmt.Fprintf(w, "func (%s) NumResults() int   { return %d }\n", implType, numResults)
		fmt.Fprintf(w, "func (%s) ErrorResult() bool { return %t }\n\n", implType, hasErrorResult)

		fmt.Fprintf(w, "func (%s) ArgNames() []string {\n", implType)
		{
			fmt.Fprintf(w, "\treturn %#v\n", argNames)
		}
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) ArgDescriptions() []string {\n", implType)
		{
			fmt.Fprintf(w, "\treturn %#v\n", argDescriptions)
		}
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) ArgTypes() []reflect.Type {\n", implType)
		if numArgs == 0 {
			fmt.Fprintf(w, "\treturn nil\n")
		} else {
			fmt.Fprintf(w, "\treturn []reflect.Type{\n")
			for _, argType := range argTypes {
				fmt.Fprintf(w, "\t\t%s,\n", reflectTypeOfTypeName(argType))
			}
			fmt.Fprintf(w, "\t}\n")
		}
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) ResultTypes() []reflect.Type {\n", implType)
		if numResults == 0 {
			fmt.Fprintf(w, "\treturn nil\n")
		} else {
			fmt.Fprintf(w, "\treturn []reflect.Type{\n")
			for _, resultType := range resultTypes {
				fmt.Fprintf(w, "\t\t%s,\n", reflectTypeOfTypeName(resultType))
			}
			fmt.Fprintf(w, "\t}\n")
		}
		fmt.Fprintf(w, "}\n\n")
	}

	var ctxArgName string
	if hasContextArg {
		ctxArgName = "ctx "
	} else if numArgs > 0 {
		ctxArgName = "_ "
	}

	resultsDecl := "(results []interface{}, err error)"
	if numResults == 0 {
		resultsDecl = "([]interface{}, error)"
	}

	if impl&ImplCallWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}

		var argsArgName string
		if !hasContextArg && numArgs > 0 || hasContextArg && numArgs > 1 {
			argsArgName = "args "
		} else if hasContextArg {
			argsArgName = "_ "
		}

		fmt.Fprintf(w, "func (f %s) Call(%scontext.Context, %s[]interface{}) %s {\n", implType, ctxArgName, argsArgName, resultsDecl)
		{
			callParams := make([]string, numArgs)
			for i, argType := range argTypes {
				if i == 0 && hasContextArg {
					callParams[0] = "ctx"
					continue
				}
				argType = strings.Replace(argType, "...", "[]", 1)
				argsIndex := i
				if hasContextArg {
					argsIndex--
				}
				if argType == "interface{}" {
					callParams[i] = fmt.Sprintf("args[%d]", argsIndex) // no type conversion needed
				} else {
					callParams[i] = fmt.Sprintf("args[%d].(%s)", argsIndex, argType)
				}
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithStringsWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}
		neededImportLines[`"github.com/domonda/go-function"`] = struct{}{}

		var strsArgName string
		if !hasContextArg && numArgs > 0 || hasContextArg && numArgs > 1 {
			strsArgName = "strs "
		} else if hasContextArg {
			strsArgName = "_ "
		}

		fmt.Fprintf(w, "func (f %s) CallWithStrings(%scontext.Context, %s...string) %s {\n", implType, ctxArgName, strsArgName, resultsDecl)
		{
			var callParams []string
			switch {
			case numArgs == 1 && hasContextArg:
				callParams = []string{"ctx"}

			case numArgs > 0:
				callParams = make([]string, len(argNames))
				fmt.Fprintf(w, "\tvar a struct {\n")
				for i, argName := range argNames {
					if i == 0 && hasContextArg {
						callParams[i] = "ctx"
						continue
					}
					if argName == "_" {
						argName = "ignoredArg" + strconv.Itoa(i)
					}
					argType := strings.Replace(argTypes[i], "...", "[]", 1)
					fmt.Fprintf(w, "\t\t%s %s\n", argName, argType)

					callParams[i] = "a." + argName
				}
				fmt.Fprintf(w, "\t}\n")

				for i, argName := range argNames {
					if i == 0 && hasContextArg || argName == "_" {
						continue
					}
					strsIndex := i
					if hasContextArg {
						strsIndex--
					}
					fmt.Fprintf(w, "\tif %d < len(strs) {\n", strsIndex)
					if argTypes[i] == "string" {
						fmt.Fprintf(w, "\t\t%s = strs[%d]\n", callParams[i], strsIndex)
					} else {
						fmt.Fprintf(w, "\t\terr := function.ScanString(strs[%d], &%s)\n", strsIndex, callParams[i])
						fmt.Fprintf(w, "\t\tif err != nil {\n")
						{
							fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q)\n", argName)
						}
						fmt.Fprintf(w, "\t\t}\n")
					}
					fmt.Fprintf(w, "\t}\n")
				}
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithNamedStringsWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}
		neededImportLines[`"github.com/domonda/go-function"`] = struct{}{}

		var strsArgName string
		if !hasContextArg && numArgs > 0 || hasContextArg && numArgs > 1 {
			strsArgName = "strs "
		} else if hasContextArg {
			strsArgName = "_ "
		}

		fmt.Fprintf(w, "func (f %s) CallWithNamedStrings(%scontext.Context, %smap[string]string) %s {\n", implType, ctxArgName, strsArgName, resultsDecl)
		{
			var callParams []string
			switch {
			case numArgs == 1 && hasContextArg:
				callParams = []string{"ctx"}

			case numArgs > 0:
				callParams = make([]string, len(argNames))
				fmt.Fprintf(w, "\tvar a struct {\n")
				for i, argName := range argNames {
					if i == 0 && hasContextArg {
						callParams[i] = "ctx"
						continue
					}
					if argName == "_" {
						argName = "ignoredArg" + strconv.Itoa(i)
					}
					argType := strings.Replace(argTypes[i], "...", "[]", 1)
					fmt.Fprintf(w, "\t\t%s %s\n", argName, argType)

					callParams[i] = "a." + argName
				}
				fmt.Fprintf(w, "\t}\n")

				for i, argName := range argNames {
					if i == 0 && hasContextArg || argName == "_" {
						continue
					}
					fmt.Fprintf(w, "\tif str, ok := strs[%q]; ok {\n", argName)
					if argTypes[i] == "string" {
						fmt.Fprintf(w, "\t\t%s = str\n", callParams[i])
					} else {
						fmt.Fprintf(w, "\t\terr := function.ScanString(str, &%s)\n", callParams[i])
						fmt.Fprintf(w, "\t\tif err != nil {\n")
						{
							fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q)\n", argName)
						}
						fmt.Fprintf(w, "\t\t}\n")
					}
					fmt.Fprintf(w, "\t}\n")
				}
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")

		if impl&ImplCallWithJSONWrapper != 0 {
			neededImportLines[`"context"`] = struct{}{}
			neededImportLines[`"github.com/domonda/go-function"`] = struct{}{}

			var argsJSONArgName string
			if !hasContextArg && numArgs > 0 || hasContextArg && numArgs > 1 {
				neededImportLines[`"encoding/json"`] = struct{}{}
				argsJSONArgName = "argsJSON "
			} else if hasContextArg {
				argsJSONArgName = "_ "
			}

			fmt.Fprintf(w, "func (f %s) CallWithJSON(%scontext.Context, %s[]byte) (results []interface{}, err error) {\n", implType, ctxArgName, argsJSONArgName)
			{
				var callParams []string
				switch {
				case numArgs == 1 && hasContextArg:
					callParams = []string{"ctx"}

				case numArgs > 0:
					callParams = make([]string, len(argNames))
					fmt.Fprintf(w, "\tvar a struct {\n")
					for i, argName := range argNames {
						if i == 0 && hasContextArg {
							callParams[i] = "ctx"
							continue
						}
						if argName == "_" {
							argName = "ignoredArg" + strconv.Itoa(i)
						} else {
							argName = exportedName(argName)
						}
						argType := strings.Replace(argTypes[i], "...", "[]", 1)
						if replacementType, ok := jsonTypeReplacements[argType]; ok {
							argType = replacementType
						}
						fmt.Fprintf(w, "\t\t%s %s\n", argName, argType)

						callParams[i] = "a." + argName
					}
					fmt.Fprintf(w, "\t}\n")

					fmt.Fprintf(w, "\terr = json.Unmarshal(argsJSON, &a)\n")
					fmt.Fprintf(w, "\tif err != nil {\n")
					{
						fmt.Fprintf(w, "\t\treturn nil, function.NewErrParseArgsJSON(err, f, argsJSON)\n")
					}
					fmt.Fprintf(w, "\t}\n")
				}
				writeFuncCall(callParams)
			}
			fmt.Fprintf(w, "}\n\n")
		}
	}

	return nil
}

// func (impl Impl) FunctionWrapperString(file *ast.File, funcDecl *ast.FuncDecl, implType, funcPackageSel string) (implSource string, err error) {
// 	b := new(strings.Builder)
// 	err = impl.WriteFunctionWrapper(b, file, funcDecl, implType, funcPackageSel)
// 	if err != nil {
// 		return "", err
// 	}
// 	return b.String(), nil
// }

func reflectTypeOfTypeName(typeName string) string {
	typeName = strings.Replace(typeName, "...", "[]", 1)
	if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "[]") || strings.HasPrefix(typeName, "map[") {
		return fmt.Sprintf("reflect.TypeOf((%s)(nil))", typeName)
	}
	return fmt.Sprintf("reflect.TypeOf((*%s)(nil)).Elem()", typeName)
}

func exportedName(name string) string {
	if name == "id" {
		return "ID"
	}
	numUpper := 1
	for _, u := range allUpper {
		if strings.HasPrefix(name, u) {
			numUpper = len(u)
			break
		}
	}
	return strings.ToUpper(name[:numUpper]) + name[numUpper:]
}

var allUpper = []string{
	"xml",
	"html",
	"json",
	"http",
}
