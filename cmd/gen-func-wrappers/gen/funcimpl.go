package gen

import (
	"fmt"
	"go/ast"
	"io"
	"strconv"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// Impl represents which wrapper interfaces should be implemented.
// Multiple interfaces can be combined using bitwise OR.
type Impl int

const (
	// ImplDescription implements function.Description interface
	ImplDescription Impl = 1 << iota

	// ImplCallWrapper implements function.CallWrapper interface
	ImplCallWrapper

	// ImplCallWithStringsWrapper implements function.CallWithStringsWrapper interface
	ImplCallWithStringsWrapper

	// ImplCallWithNamedStringsWrapper implements function.CallWithNamedStringsWrapper interface
	ImplCallWithNamedStringsWrapper

	// ImplCallWithJSONWrapper implements function.CallWithJSONWrapper interface
	ImplCallWithJSONWrapper

	// ImplWrapper implements the full function.Wrapper interface (all of the above)
	ImplWrapper = ImplDescription | ImplCallWrapper | ImplCallWithStringsWrapper | ImplCallWithNamedStringsWrapper | ImplCallWithJSONWrapper
)

// ImplFromString parses a string representation of an interface name into an Impl value.
//
// Supported strings:
//   - "function.Wrapper" -> ImplWrapper (all interfaces)
//   - "function.Description" -> ImplDescription
//   - "function.CallWrapper" -> ImplCallWrapper
//   - "function.CallWithStringsWrapper" -> ImplCallWithStringsWrapper
//   - "function.CallWithNamedStringsWrapper" -> ImplCallWithNamedStringsWrapper
//   - "function.ImplCallWithJSONWrapper" -> ImplCallWithJSONWrapper
//
// Returns an error if the string doesn't match any known interface.
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

// String returns the string representation of the Impl value.
// For combined implementations (bitwise OR), it returns a generic "Impl(n)" format.
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

// WriteFunctionWrapper generates a complete wrapper implementation for a function.
// This is the core code generation logic that produces type-safe wrapper methods.
//
// Parameters:
//   - w: Writer to output generated code to
//   - funcFile: The AST file containing the function (needed for imports)
//   - funcDecl: The function declaration to wrap
//   - implType: Name of the generated wrapper type (e.g., "myFunctionT")
//   - funcPackage: Package name qualifier for the wrapped function (empty string if same package)
//   - neededImportLines: Map to collect all imports needed by the generated code
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON unmarshalling
//   - targetFileImports: Import specs from the target file to check for conflicts
//
// Returns:
//   - error if code generation fails
//
// The generated code includes:
//  1. Wrapper type declaration (struct{})
//  2. String() method (always generated)
//  3. Description methods (if ImplDescription is set): Name, NumArgs, ArgNames, ArgTypes, etc.
//  4. Call method (if ImplCallWrapper is set): Calls function with []any arguments
//  5. CallWithStrings method (if ImplCallWithStringsWrapper is set): Parses string arguments
//  6. CallWithNamedStrings method (if ImplCallWithNamedStringsWrapper is set): Uses map[string]string
//  7. CallWithJSON method (if ImplCallWithJSONWrapper is set): Unmarshals JSON to arguments
//
// The method handles:
//   - context.Context as first argument (automatic detection and handling)
//   - Variadic parameters (...type)
//   - Error return values (automatic error result detection)
//   - Type conversions for string parsing
//   - Proper argument descriptions from function comments
func (impl Impl) WriteFunctionWrapper(w io.Writer, funcFile *ast.File, funcDecl *ast.FuncDecl, implType, funcPackage string, neededImportLines map[string]struct{}, jsonTypeReplacements map[string]string, targetFileImports []*ast.ImportSpec) error {
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
			fmt.Fprintf(w, "\tresults = make([]any, %d)\n", numResultsWithoutErr)
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

	// Always get imports of function arguments and build package name remapping
	packageRemap, err := gatherFieldListImports(funcFile, funcDecl.Type.Params, neededImportLines, targetFileImports)
	if err != nil {
		return err
	}

	if impl&ImplDescription != 0 {
		neededImportLines[`"reflect"`] = struct{}{}

		// Get imports of results only for function.Description.ArgTypes() method
		resultRemap, err := gatherFieldListImports(funcFile, funcDecl.Type.Results, neededImportLines, targetFileImports)
		if err != nil {
			return err
		}
		// Merge result remapping into package remap
		for src, tgt := range resultRemap {
			if packageRemap == nil {
				packageRemap = make(map[string]string)
			}
			packageRemap[src] = tgt
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
				fmt.Fprintf(w, "\t\t%s,\n", reflectTypeOfTypeName(argType, packageRemap))
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
				fmt.Fprintf(w, "\t\t%s,\n", reflectTypeOfTypeName(resultType, packageRemap))
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

	resultsDecl := "(results []any, err error)"
	if numResults == 0 {
		resultsDecl = "([]any, error)"
	}

	if impl&ImplCallWrapper != 0 {
		neededImportLines[`"context"`] = struct{}{}

		var argsArgName string
		if !hasContextArg && numArgs > 0 || hasContextArg && numArgs > 1 {
			argsArgName = "args "
		} else if hasContextArg {
			argsArgName = "_ "
		}

		fmt.Fprintf(w, "func (%s) Call(%scontext.Context, %s[]any) %s {\n", implType, ctxArgName, argsArgName, resultsDecl)
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
				if argType == "any" {
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

		receiver := ""
		for i, argName := range argNames {
			if i == 0 && hasContextArg || argName == "_" {
				continue
			}
			if argTypes[i] != "string" {
				// If there is any named non string argument
				// then the method code below needs a receiver
				receiver = "f "
				break
			}
		}
		fmt.Fprintf(w, "func (%s%s) CallWithStrings(%scontext.Context, %s...string) %s {\n", receiver, implType, ctxArgName, strsArgName, resultsDecl)
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
							fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q, strs[%d])\n", argName, strsIndex)
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

		receiver := ""
		for i, argName := range argNames {
			if i == 0 && hasContextArg || argName == "_" {
				continue
			}
			if argTypes[i] != "string" {
				// If there is any named non string argument
				// then the method code below needs a receiver
				receiver = "f "
				break
			}
		}
		fmt.Fprintf(w, "func (%s%s) CallWithNamedStrings(%scontext.Context, %smap[string]string) %s {\n", receiver, implType, ctxArgName, strsArgName, resultsDecl)
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
							fmt.Fprintf(w, "\t\t\treturn nil, function.NewErrParseArgString(err, f, %q, str)\n", argName)
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

			receiver := ""
			if numArgs > 1 || numArgs == 1 && !hasContextArg {
				receiver = "f "
			}
			fmt.Fprintf(w, "func (%s%s) CallWithJSON(%scontext.Context, %s[]byte) (results []any, err error) {\n", receiver, implType, ctxArgName, argsJSONArgName)
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

// reflectTypeOfTypeName generates code for obtaining a reflect.Type for a given type name.
// It uses reflect.TypeFor which is the modern generic-based approach.
//
// Variadic parameters (...T) are converted to slices ([]T) before creating the reflect.Type.
//
// The packageRemap parameter translates package qualifiers from source to target context.
//
// Example:
//   - "string" -> "reflect.TypeFor[string]()"
//   - "...int" -> "reflect.TypeFor[[]int]()"
//   - "gmail.Label" with remap["gmail"]="gmailapi" -> "reflect.TypeFor[gmailapi.Label]()"
func reflectTypeOfTypeName(typeName string, packageRemap map[string]string) string {
	typeName = strings.Replace(typeName, "...", "[]", 1)
	typeName = remapPackageQualifiers(typeName, packageRemap)
	return fmt.Sprintf("reflect.TypeFor[%s]()", typeName)
}

// remapPackageQualifiers translates package qualifiers in a type name using the provided mapping.
// For example, "gmail.Label" with remap["gmail"]="gmailapi" becomes "gmailapi.Label".
func remapPackageQualifiers(typeName string, packageRemap map[string]string) string {
	if len(packageRemap) == 0 {
		return typeName
	}

	// Simple approach: replace each occurrence of "pkgName." with "newName."
	// This works for most cases but could be improved with proper parsing
	result := typeName
	for srcPkg, tgtPkg := range packageRemap {
		// Replace "srcPkg." with "tgtPkg." but only when followed by an identifier
		// Use word boundary to avoid replacing partial matches
		oldPattern := srcPkg + "."
		newPattern := tgtPkg + "."
		result = strings.ReplaceAll(result, oldPattern, newPattern)
	}
	return result
}

// exportedName converts a variable name to an exported (capitalized) name.
// This is used when generating struct fields for CallWithJSON argument unmarshalling,
// since JSON struct fields must be exported.
//
// Special cases:
//   - "id" -> "ID" (common acronym)
//   - Names starting with known acronyms get fully uppercased (e.g., "apiKey" -> "APIKey")
//
// Examples:
//   - "name" -> "Name"
//   - "userID" -> "UserID"
//   - "apiKey" -> "APIKey"
//   - "htmlContent" -> "HTMLContent"
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

// allUpper lists common acronyms that should be fully uppercased in exported names.
// This is used by exportedName to properly capitalize field names like "apiKey" -> "APIKey".
var allUpper = []string{
	"acl",
	"api",
	"csv",
	"html",
	"http",
	"jpeg",
	"json",
	"png",
	"tiff",
	"uuid",
	"xml",
}
