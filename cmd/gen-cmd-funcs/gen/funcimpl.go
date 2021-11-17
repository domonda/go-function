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

	ImplWrapper = ImplDescription | ImplCallWrapper | ImplCallWithStringsWrapper | ImplCallWithNamedStringsWrapper
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
	default:
		return fmt.Sprintf("Impl(%d)", impl)
	}
}

func (impl Impl) WriteFunctionWrapper(w io.Writer, funcFile *ast.File, funcDecl *ast.FuncDecl, implType, funcPackage string, neededImportLines map[string]struct{}) error {
	var (
		argNames        = funcDeclArgNames(funcDecl)
		argDescriptions = funcDeclArgDescriptions(funcDecl)
		argTypes        = funcDeclArgTypes(funcDecl, funcPackage)
		numArgs         = len(argTypes)
		resultTypes     = funcDeclResultTypes(funcDecl, funcPackage)
		hasContextArg   = numArgs > 0 && argTypes[0] == "context.Context"
		hasErrorResult  = len(resultTypes) > 0 && resultTypes[len(resultTypes)-1] == "error"
		funcPackageSel  = ""
	)
	if funcPackage != "" {
		funcPackageSel = funcPackage + "."
	}

	// if funcDecl.Name.Name == "MyFunc" {
	// 	fmt.Println(funcDecl.Name.Name)
	// }

	err := gatherFunctionImports(funcFile, funcDecl.Type, neededImportLines)
	if err != nil {
		return err
	}

	writeFuncCall := func(args []string) {
		numResultsWithoutErr := len(resultTypes)
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
		if len(resultTypes) > 0 {
			fmt.Fprintf(w, " = ")
		}
		ellipsis := ""
		if numArgs > 0 && strings.HasPrefix(argTypes[numArgs-1], "...") {
			ellipsis = "..."
		}
		fmt.Fprintf(w, "%s%s(%s%s) // call\n", funcPackageSel, funcDecl.Name.Name, strings.Join(args, ", "), ellipsis)
		fmt.Fprintf(w, "\treturn results, err\n")
	}

	fmt.Fprintf(w, "// %s wraps %s%s as %s (generated code)\n", implType, funcPackageSel, funcDecl.Name.Name, impl)
	fmt.Fprintf(w, "type %s struct{}\n\n", implType)

	// Always implement fmt.Stringer
	fmt.Fprintf(w, "func (%s) String() string {\n", implType)
	fmt.Fprintf(w, "\treturn \"%s%s%s\"\n", funcPackageSel, funcDecl.Name.Name, astvisit.FuncTypeString(funcDecl.Type))
	fmt.Fprintf(w, "}\n\n")

	if impl&ImplDescription != 0 {
		neededImportLines[`"reflect"`] = struct{}{}

		fmt.Fprintf(w, "func (%s) Name() string {\n", implType)
		fmt.Fprintf(w, "\treturn \"%s\"\n", funcDecl.Name.Name)
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) NumArgs() int      { return %d }\n", implType, numArgs)
		fmt.Fprintf(w, "func (%s) ContextArg() bool  { return %t }\n", implType, hasContextArg)
		fmt.Fprintf(w, "func (%s) NumResults() int   { return %d }\n", implType, len(resultTypes))
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
		if len(resultTypes) == 0 {
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

	ctxArgName := "ctx"
	if !hasContextArg {
		ctxArgName = "_"
	}
	strsArgName := "strs"
	argsArgName := "args"
	if numArgs == 0 || hasContextArg && numArgs == 1 {
		strsArgName = "_"
		argsArgName = "_"
	}

	if impl&ImplCallWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}

		fmt.Fprintf(w, "func (f %s) Call(%s context.Context, %s []interface{}) (results []interface{}, err error) {\n", implType, ctxArgName, argsArgName)
		{
			args := make([]string, numArgs)
			for i, argType := range argTypes {
				if i == 0 && hasContextArg {
					args[0] = "ctx"
					continue
				}
				argsIndex := i
				if hasContextArg {
					argsIndex--
				}
				args[i] = fmt.Sprintf("args[%d]", argsIndex)
				if argType != "interface{}" {
					args[i] += ".(" + strings.Replace(argType, "...", "[]", 1) + ")"
				}
			}
			writeFuncCall(args)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithStringsWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}
		neededImportLines[`"github.com/domonda/go-function"`] = struct{}{}

		fmt.Fprintf(w, "func (f %s) CallWithStrings(%s context.Context, %s ...string) (results []interface{}, err error) {\n", implType, ctxArgName, strsArgName)
		{
			for i, argName := range argNames {
				if i == 0 && hasContextArg {
					if argName != "ctx" {
						fmt.Fprintf(w, "\t%s := ctx\n", argName)
					}
					continue
				}
				strsIndex := i
				if hasContextArg {
					strsIndex--
				}

				argType := strings.Replace(argTypes[i], "...", "[]", 1)

				if argName == "_" {
					argName := "ignoredArg" + strconv.Itoa(i)
					argNames[i] = argName
					fmt.Fprintf(w, "\tvar %s %s\n", argName, argType)
					continue
				}

				fmt.Fprintf(w, "\tvar %s %s\n", argName, argType)
				fmt.Fprintf(w, "\tif len(strs) > %d {\n", strsIndex)
				if argTypes[i] == "string" {
					fmt.Fprintf(w, "\t\t%s = strs[%d]\n", argName, strsIndex)
				} else {
					fmt.Fprintf(w, "\t\terr = function.ScanString(strs[%d], &%s)\n", strsIndex, argName)
					fmt.Fprintf(w, "\t\tif err != nil {\n")
					{
						fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q)\n", argName)
					}
					fmt.Fprintf(w, "\t\t}\n")
				}
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(argNames)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithNamedStringsWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}
		neededImportLines[`"github.com/domonda/go-function"`] = struct{}{}

		fmt.Fprintf(w, "func (f %s) CallWithNamedStrings(%s context.Context, %s map[string]string) (results []interface{}, err error) {\n", implType, ctxArgName, strsArgName)
		{
			for i, argName := range argNames {
				if i == 0 && hasContextArg {
					if argName != "ctx" {
						fmt.Fprintf(w, "\t%s := ctx\n", argName)
					}
					continue
				}

				argType := strings.Replace(argTypes[i], "...", "[]", 1)

				if argName == "_" {
					argName := "ignoredArg" + strconv.Itoa(i)
					argNames[i] = argName
					fmt.Fprintf(w, "\tvar %s %s\n", argName, argType)
					continue
				}

				fmt.Fprintf(w, "\tvar %s %s\n", argName, argType)
				fmt.Fprintf(w, "\tif str, ok := strs[%q]; ok {\n", argName)
				if argTypes[i] == "string" {
					fmt.Fprintf(w, "\t\t%s = str\n", argName)
				} else {
					fmt.Fprintf(w, "\t\terr = function.ScanString(str, &%s)\n", argName)
					fmt.Fprintf(w, "\t\tif err != nil {\n")
					{
						fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q)\n", argName)
					}
					fmt.Fprintf(w, "\t\t}\n")
				}
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(argNames)
		}
		fmt.Fprintf(w, "}\n")
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
