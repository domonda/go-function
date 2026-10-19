package gen

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
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
//   - "function.CallWithJSONWrapper" -> ImplCallWithJSONWrapper
//
// It's the inverse of Impl.String for every single interface and ImplWrapper.
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
	case "function.CallWithJSONWrapper":
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
//   - funcDir: The directory of funcFile that resolves its imports
//   - funcDecl: The function declaration to wrap
//   - implType: Name of the generated wrapper type (e.g., "myFunctionT")
//   - funcPackage: Package name qualifier for the wrapped function (empty string if same package)
//   - neededImports: Set to collect all imports needed by the generated code, must not be nil
//   - jsonTypeReplacements: Map of interface types to concrete types for JSON unmarshalling
//   - target: The file the wrappers are generated into, to check for import conflicts
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
//   - Unnamed parameters, named arg0, arg1, ... by their index
//   - Error return values (automatic error result detection)
//   - Type conversions for string parsing
//   - Proper argument descriptions from function comments
//
// Like ReflectWrapper, the generated Call passes the zero value for missing
// or untyped nil arguments, accepts values assignable to the argument type,
// and returns an error for too many arguments or an argument of the wrong type.
// CallWithStrings and CallWithNamedStrings pass the unparsed string
// to an argument of type any. Like in a Go call, CallWithStrings passes
// every string from the variadic argument on as one of its elements.
// CallWithJSON unmarshals the arguments from the values of a JSON object
// with the exact argument names as keys, JSON that is no object
// is an error also for a function without arguments,
// except null, which is handled like an empty object.
//
// Only the imports that the generated code references are added to neededImports.
func (impl Impl) WriteFunctionWrapper(w io.Writer, funcFile *ast.File, funcDir string, funcDecl *ast.FuncDecl, implType, funcPackage string, neededImports astvisit.Imports, jsonTypeReplacements map[string]string, target *targetFile) error {
	var (
		argNames        = funcTypeArgNames(funcDecl.Type)
		argDescriptions = funcDeclArgDescriptions(funcDecl)
		argTypes        = funcTypeArgTypes(funcDecl.Type, funcPackage)
		numArgs         = len(argTypes)
		resultTypes     = funcTypeResultTypes(funcDecl.Type, funcPackage)
		numResults      = len(resultTypes)
		hasContextArg   = numArgs > 0 && isContextType(funcDecl.Type.Params.List[0].Type, funcFile)
		hasErrorResult  = numResults > 0 && resultTypes[numResults-1] == "error"
		isVariadic      = numArgs > 0 && strings.HasPrefix(argTypes[numArgs-1], "...")
		funcPackageSel  = ""
	)
	if funcPackage != "" {
		funcPackageSel = funcPackage + "."
		err := checkNoDotImportedTypes(funcFile, funcDecl, funcPackage)
		if err != nil {
			return err
		}
	}

	// Always get imports of function arguments and build package name remapping
	packageRemap, err := gatherFieldListImports(funcFile, funcDir, funcDecl.Type.Params, neededImports, target)
	if err != nil {
		return fmt.Errorf("can't import the argument types of %s%s: %w", funcPackageSel, funcDecl.Name.Name, err)
	}
	if impl&ImplDescription != 0 {
		// Get imports of results only for function.Description.ResultTypes() method
		resultRemap, err := gatherFieldListImports(funcFile, funcDir, funcDecl.Type.Results, neededImports, target)
		if err != nil {
			return fmt.Errorf("can't import the result types of %s%s: %w", funcPackageSel, funcDecl.Name.Name, err)
		}
		// Merge result remapping into package remap
		maps.Copy(packageRemap, resultRemap)
	}
	// The types as the target file refers to them
	for i := range argTypes {
		argTypes[i] = remapPackageQualifiers(argTypes[i], packageRemap)
	}
	for i := range resultTypes {
		resultTypes[i] = remapPackageQualifiers(resultTypes[i], packageRemap)
	}

	// importErr is the first error of an import that the generated code needs
	var importErr error
	useImport := func(pkgName, path string) {
		err := target.useImport(neededImports, pkgName, path)
		if err != nil && importErr == nil {
			importErr = err
		}
	}
	functionSel := func() string {
		sel, err := target.functionSel(neededImports)
		if err != nil && importErr == nil {
			importErr = err
		}
		return sel
	}

	var (
		firstArg    = 0      // Index of the first argument that is not the context
		ctxArgName  = ""     // Name of the context.Context parameter of the Call methods
		argFields   []string // Struct field names of the arguments
		valueTypes  []string // Argument types with variadic arguments as slices
		hasCallArgs = numArgs > 0 && !hasContextArg || numArgs > 1
	)
	if hasContextArg {
		firstArg = 1
		ctxArgName = "ctx "
	} else if numArgs > 0 {
		ctxArgName = "_ "
	}
	for i, argName := range argNames {
		if argName == "_" {
			argName = "ignoredArg" + strconv.Itoa(i)
		}
		argFields = append(argFields, argName)
		valueTypes = append(valueTypes, sliceOfVariadic(argTypes[i]))
	}
	// The identifiers declared by the generated methods in the scope
	// of the wrapped call must not shadow the wrapped function or its package,
	// and a function of the package must not be named like an import
	callee := cmp.Or(funcPackage, funcDecl.Name.Name)
	switch {
	case callee == "a" && !hasCallArgs || callee == "ctx" && !hasContextArg:
		// Not declared
	case funcPackage == "" && (callee == "i" || callee == "ok" || callee == "raw" || callee == "str" || callee == "v"):
		// Only declared in blocks that don't call the wrapped function,
		// but they use the argument types that are qualified with a package
	case generatedIdents[callee]:
		return fmt.Errorf("can't wrap %s%s: the generated code declares the identifier %s, rename the function or import its package with another name", funcPackageSel, funcDecl.Name.Name, callee)
	case funcPackage == "" && generatedImports[callee] != "":
		return fmt.Errorf("can't wrap %s: the generated code imports the package %q with the name of the function, rename the function", callee, generatedImports[callee])
	}
	// The call methods use the argument types in the scope of their identifiers,
	// which would shadow declarations of the package that the types refer to
	argTypeIdents := make(map[string]struct{})
	for _, field := range funcDecl.Type.Params.List {
		unqualifiedIdents(field.Type, argTypeIdents)
	}
	for _, ident := range slices.Sorted(maps.Keys(argTypeIdents)) {
		if generatedIdents[ident] {
			return fmt.Errorf("can't wrap %s%s: the generated code declares the identifier %s that the argument types refer to, rename the declaration %s of the package", funcPackageSel, funcDecl.Name.Name, ident, ident)
		}
	}

	// isScanned reports if the argument with the index of the type typeName,
	// its type or for CallWithStrings the element type of a variadic argument,
	// is parsed from a string by CallWithStrings and CallWithNamedStrings.
	// Like ReflectWrapper, only arguments of type any get the unparsed string,
	// strings are scanned so that scanners registered for them apply
	isScanned := func(i int, typeName string) bool {
		return i >= firstArg && argNames[i] != "_" && !isAnyType(typeName)
	}

	// writeArgsStruct writes the declaration of the variable a
	// with a struct field for every argument except the context
	// and returns the parameters to call the wrapped function with
	writeArgsStruct := func(fieldNames, fieldTypes []string) (callParams []string) {
		callParams = make([]string, numArgs)
		if hasContextArg {
			callParams[0] = "ctx"
		}
		if !hasCallArgs {
			return callParams
		}
		fmt.Fprintf(w, "\tvar a struct {\n")
		for i := firstArg; i < numArgs; i++ {
			fmt.Fprintf(w, "\t\t%s %s\n", fieldNames[i], fieldTypes[i])
			callParams[i] = "a." + fieldNames[i]
		}
		fmt.Fprintf(w, "\t}\n")
		return callParams
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
		if isVariadic {
			ellipsis = "..."
		}
		fmt.Fprintf(w, "%s%s(%s%s) // wrapped call\n", funcPackageSel, funcDecl.Name.Name, strings.Join(args, ", "), ellipsis)
		if numResults > 0 {
			fmt.Fprintf(w, "\treturn results, err\n")
		} else {
			fmt.Fprintf(w, "\treturn nil, nil\n")
		}
	}

	// writeTooManyArgsCheck writes the check that the slice variable
	// with the arguments has not more than maxArgs elements
	writeTooManyArgsCheck := func(argsVar string, maxArgs int) {
		useImport("fmt", "fmt")
		fmt.Fprintf(w, "\tif len(%s) > %d {\n", argsVar, maxArgs)
		fmt.Fprintf(w, "\t\treturn nil, fmt.Errorf(\"%%d arguments passed, but function %%s takes %d\", len(%s), f)\n", maxArgs, argsVar)
		fmt.Fprintf(w, "\t}\n")
	}

	fmt.Fprintf(w, "// %s wraps %s%s as %s (generated code)\n", implType, funcPackageSel, funcDecl.Name.Name, impl)
	fmt.Fprintf(w, "type %s struct{}\n\n", implType)

	// Name and String are qualified with the package name
	// like the ones of function.ReflectWrapper
	qualifiedFuncName := funcFile.Name.Name + "." + funcDecl.Name.Name

	// Always implement fmt.Stringer
	fmt.Fprintf(w, "func (%s) String() string {\n", implType)
	fmt.Fprintf(w, "\treturn %s\n", strconv.Quote(signatureString(qualifiedFuncName, argNames, funcTypeArgTypes(funcDecl.Type, funcFile.Name.Name), funcTypeResultTypes(funcDecl.Type, funcFile.Name.Name))))
	fmt.Fprintf(w, "}\n\n")

	if impl&ImplDescription != 0 {
		useImport("reflect", "reflect")

		fmt.Fprintf(w, "func (%s) Name() string {\n", implType)
		fmt.Fprintf(w, "\treturn %q\n", qualifiedFuncName)
		fmt.Fprintf(w, "}\n\n")

		fmt.Fprintf(w, "func (%s) NumArgs() int      { return %d }\n", implType, numArgs)
		fmt.Fprintf(w, "func (%s) ContextArg() bool  { return %t }\n", implType, hasContextArg)
		fmt.Fprintf(w, "func (%s) Variadic() bool    { return %t }\n", implType, isVariadic)
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

	resultsDecl := "(results []any, err error)"
	if numResults == 0 {
		resultsDecl = "([]any, error)"
	}

	if impl&ImplCallWrapper != 0 {
		useImport("context", "context")

		numCallArgs := numArgs - firstArg
		fmt.Fprintf(w, "func (f %s) Call(%scontext.Context, args []any) %s {\n", implType, cmp.Or(ctxArgName, "_ "), resultsDecl)
		{
			writeTooManyArgsCheck("args", numCallArgs)
			callParams := writeArgsStruct(argFields, valueTypes)
			for i := firstArg; i < numArgs; i++ {
				argsIndex := i - firstArg
				if isAnyType(valueTypes[i]) {
					fmt.Fprintf(w, "\tif %d < len(args) {\n", argsIndex)
					fmt.Fprintf(w, "\t\t%s = args[%d]\n", callParams[i], argsIndex)
					fmt.Fprintf(w, "\t}\n")
					continue
				}
				// Untyped nil arguments are passed as zero values
				fmt.Fprintf(w, "\tif %d < len(args) && args[%d] != nil {\n", argsIndex, argsIndex)
				fmt.Fprintf(w, "\t\tv, ok := args[%d].(%s)\n", argsIndex, valueTypes[i])
				if !typeAssertionIsAssignability(valueTypes[i]) {
					// Like ReflectWrapper, accept a value of another type
					// that is assignable to the argument type,
					// with reflection only if the type assertion failed
					useImport("reflect", "reflect")
					fmt.Fprintf(w, "\t\tif !ok && reflect.TypeOf(args[%d]).AssignableTo(%s) {\n", argsIndex, reflectTypeOfTypeName(valueTypes[i]))
					fmt.Fprintf(w, "\t\t\tv, ok = reflect.TypeAssert[%s](reflect.ValueOf(args[%d]).Convert(%s))\n", valueTypes[i], argsIndex, reflectTypeOfTypeName(valueTypes[i]))
					fmt.Fprintf(w, "\t\t}\n")
				}
				fmt.Fprintf(w, "\t\tif !ok {\n")
				// The type is part of the format string, so escape the % of struct tags
				format := fmt.Sprintf("can't use %%T as type %s for argument '%s' of function %%s", strings.ReplaceAll(valueTypes[i], "%", "%%"), argNames[i])
				fmt.Fprintf(w, "\t\t\treturn nil, fmt.Errorf(%s, args[%d], f)\n", strconv.Quote(format), argsIndex)
				fmt.Fprintf(w, "\t\t}\n")
				fmt.Fprintf(w, "\t\t%s = v\n", callParams[i])
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	// receiverIfScanned returns the receiver name for the string methods
	// that need the receiver for the errors of scanned arguments,
	// with variadicElems for CallWithStrings that scans the elements
	// of a variadic argument
	receiverIfScanned := func(variadicElems bool) string {
		for i := range numArgs {
			typeName := argTypes[i]
			if variadicElems {
				typeName = strings.TrimPrefix(typeName, "...")
			}
			if isScanned(i, typeName) {
				return "f "
			}
		}
		return ""
	}

	if impl&ImplCallWithStringsWrapper != 0 {
		useImport("context", "context")

		// More strings than arguments are an error,
		// except for a variadic argument that takes the remaining strings
		receiver := receiverIfScanned(true)
		if !isVariadic {
			receiver = "f "
		}
		fmt.Fprintf(w, "func (%s%s) CallWithStrings(%scontext.Context, strs ...string) %s {\n", receiver, implType, cmp.Or(ctxArgName, "_ "), resultsDecl)
		{
			if !isVariadic {
				writeTooManyArgsCheck("strs", numArgs-firstArg)
			}
			callParams := writeArgsStruct(argFields, valueTypes)
			for i := firstArg; i < numArgs; i++ {
				if argNames[i] == "_" {
					continue
				}
				strsIndex := i - firstArg
				if isVariadic && i == numArgs-1 {
					// Like in a Go call, every string from the variadic argument on
					// is one of its elements, also a single string
					elemType := strings.TrimPrefix(argTypes[i], "...")
					fmt.Fprintf(w, "\tif len(strs) > %d {\n", strsIndex)
					fmt.Fprintf(w, "\t\t%s = make(%s, len(strs)-%d)\n", callParams[i], valueTypes[i], strsIndex)
					fmt.Fprintf(w, "\t\tfor i, str := range strs[%d:] {\n", strsIndex)
					if isScanned(i, elemType) {
						functionSel := functionSel()
						fmt.Fprintf(w, "\t\t\terr := %sScanString(str, &%s[i])\n", functionSel, callParams[i])
						fmt.Fprintf(w, "\t\t\tif err != nil {\n")
						fmt.Fprintf(w, "\t\t\t\treturn nil, %sNewErrParseArgString(err, f, %q, str)\n", functionSel, argNames[i])
						fmt.Fprintf(w, "\t\t\t}\n")
					} else {
						fmt.Fprintf(w, "\t\t\t%s[i] = str\n", callParams[i])
					}
					fmt.Fprintf(w, "\t\t}\n")
					fmt.Fprintf(w, "\t}\n")
					continue
				}
				fmt.Fprintf(w, "\tif %d < len(strs) {\n", strsIndex)
				if isScanned(i, argTypes[i]) {
					functionSel := functionSel()
					fmt.Fprintf(w, "\t\terr := %sScanString(strs[%d], &%s)\n", functionSel, strsIndex, callParams[i])
					fmt.Fprintf(w, "\t\tif err != nil {\n")
					{
						fmt.Fprintf(w, "\t\t\treturn nil, %sNewErrParseArgString(err, f, %q, strs[%d])\n", functionSel, argNames[i], strsIndex)
					}
					fmt.Fprintf(w, "\t\t}\n")
				} else {
					// The unparsed string for any
					fmt.Fprintf(w, "\t\t%s = strs[%d]\n", callParams[i], strsIndex)
				}
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithNamedStringsWrapper != 0 {
		useImport("context", "context")

		var strsArgName string
		if hasCallArgs {
			strsArgName = "strs "
		} else if hasContextArg {
			strsArgName = "_ "
		}
		fmt.Fprintf(w, "func (%s%s) CallWithNamedStrings(%scontext.Context, %smap[string]string) %s {\n", receiverIfScanned(false), implType, ctxArgName, strsArgName, resultsDecl)
		{
			callParams := writeArgsStruct(argFields, valueTypes)
			for i := firstArg; i < numArgs; i++ {
				if argNames[i] == "_" {
					continue
				}
				fmt.Fprintf(w, "\tif str, ok := strs[%q]; ok {\n", argNames[i])
				if isScanned(i, argTypes[i]) {
					functionSel := functionSel()
					fmt.Fprintf(w, "\t\terr := %sScanString(str, &%s)\n", functionSel, callParams[i])
					fmt.Fprintf(w, "\t\tif err != nil {\n")
					{
						fmt.Fprintf(w, "\t\t\treturn nil, %sNewErrParseArgString(err, f, %q, str)\n", functionSel, argNames[i])
					}
					fmt.Fprintf(w, "\t\t}\n")
				} else {
					// The unparsed string for any
					fmt.Fprintf(w, "\t\t%s = str\n", callParams[i])
				}
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if impl&ImplCallWithJSONWrapper != 0 {
		useImport("context", "context")

		fmt.Fprintf(w, "func (f %s) CallWithJSON(%scontext.Context, argsJSON []byte) (results []any, err error) {\n", implType, cmp.Or(ctxArgName, "_ "))
		{
			// Like ReflectWrapper, the arguments are the values
			// of the JSON object with the exact argument names as keys,
			// a struct would match the keys case-insensitively
			// and use the last of the keys that only differ in case.
			// Also without arguments, JSON that is no object is an error
			// and the function is not called, null is handled like an empty object.
			useImport("json", "encoding/json")
			argsDest := "new(map[string]json.RawMessage)"
			if hasCallArgs {
				fmt.Fprintf(w, "\tvar args map[string]json.RawMessage\n")
				argsDest = "&args"
			}
			fmt.Fprintf(w, "\terr = json.Unmarshal(argsJSON, %s)\n", argsDest)
			fmt.Fprintf(w, "\tif err != nil {\n")
			{
				fmt.Fprintf(w, "\t\treturn nil, %sNewErrParseArgsJSON(err, f)\n", functionSel())
			}
			fmt.Fprintf(w, "\t}\n")
			callParams := writeArgsStruct(argFields, valueTypes)
			// writeUnmarshalArg writes the unmarshalling of the JSON value raw
			// into the variable or field dest for the argument with the index
			writeUnmarshalArg := func(i int, dest string) {
				fmt.Fprintf(w, "\t\terr = json.Unmarshal(raw, &%s)\n", dest)
				fmt.Fprintf(w, "\t\tif err != nil {\n")
				fmt.Fprintf(w, "\t\t\treturn nil, %sNewErrParseArgJSON(err, f, %q)\n", functionSel(), argNames[i])
				fmt.Fprintf(w, "\t\t}\n")
			}
			for i := firstArg; i < numArgs; i++ {
				if argNames[i] == "_" {
					continue
				}
				// A variadic argument is replaced by a slice of the replacement of its element type
				elemType, variadic := strings.CutPrefix(argTypes[i], "...")
				replacementType, replaced := jsonTypeReplacements[elemType]
				if replaced {
					var err error
					replacementType, err = target.addImportsOfType(replacementType, funcFile, funcDir, neededImports)
					if err != nil {
						return fmt.Errorf("can't use JSON type replacement %s for %s of %s%s: %w", jsonTypeReplacements[elemType], elemType, funcPackageSel, funcDecl.Name.Name, err)
					}
				}
				if replaced && !variadic {
					// JSON null keeps the zero value of the argument,
					// a nil pointer of the replacement type in an interface
					// argument would not be nil for the wrapped function
					fmt.Fprintf(w, "\tif raw, ok := args[%q]; ok && string(raw) != \"null\" {\n", argNames[i])
				} else {
					fmt.Fprintf(w, "\tif raw, ok := args[%q]; ok {\n", argNames[i])
				}
				switch {
				case replaced && variadic:
					// The elements of the replacement type have to be converted
					// to the element type of the variadic argument,
					// JSON null elements keep their zero value like a null argument
					fmt.Fprintf(w, "\t\tvar v []json.RawMessage\n")
					writeUnmarshalArg(i, "v")
					fmt.Fprintf(w, "\t\t%s = make(%s, len(v))\n", callParams[i], valueTypes[i])
					fmt.Fprintf(w, "\t\tfor i, raw := range v {\n")
					fmt.Fprintf(w, "\t\t\tif string(raw) == \"null\" {\n")
					fmt.Fprintf(w, "\t\t\t\tcontinue\n")
					fmt.Fprintf(w, "\t\t\t}\n")
					fmt.Fprintf(w, "\t\t\tvar v %s\n", replacementType)
					fmt.Fprintf(w, "\t\t\terr = json.Unmarshal(raw, &v)\n")
					fmt.Fprintf(w, "\t\t\tif err != nil {\n")
					fmt.Fprintf(w, "\t\t\t\treturn nil, %sNewErrParseArgJSON(err, f, %q)\n", functionSel(), argNames[i])
					fmt.Fprintf(w, "\t\t\t}\n")
					fmt.Fprintf(w, "\t\t\t%s[i] = v\n", callParams[i])
					fmt.Fprintf(w, "\t\t}\n")
				case replaced:
					fmt.Fprintf(w, "\t\tvar v %s\n", replacementType)
					writeUnmarshalArg(i, "v")
					fmt.Fprintf(w, "\t\t%s = v\n", callParams[i])
				case argTypes[i] == "error":
					// Like ReflectWrapper, an error is unmarshalled from its message,
					// fmt.Errorf without %w returns the error of errors.New
					// without importing errors that the file could use for another package
					useImport("fmt", "fmt")
					fmt.Fprintf(w, "\t\tvar str string\n")
					writeUnmarshalArg(i, "str")
					fmt.Fprintf(w, "\t\tif str != \"\" {\n")
					fmt.Fprintf(w, "\t\t\t%s = fmt.Errorf(\"%%s\", str)\n", callParams[i])
					fmt.Fprintf(w, "\t\t}\n")
				default:
					writeUnmarshalArg(i, callParams[i])
				}
				fmt.Fprintf(w, "\t}\n")
			}
			writeFuncCall(callParams)
		}
		fmt.Fprintf(w, "}\n\n")
	}

	if importErr != nil {
		return fmt.Errorf("can't wrap %s%s: %w", funcPackageSel, funcDecl.Name.Name, importErr)
	}
	return nil
}

// addImportsOfType adds the imports to setImports that the type typeName
// needs in the target file and returns the type with the package qualifiers
// that the target file uses. The package qualifiers of the type
// are resolved by the imports of the target file first,
// then by the imports of funcFile in funcDir.
// Returns an error if typeName is no valid type expression
// or a package qualifier is imported by neither of the files.
func (target *targetFile) addImportsOfType(typeName string, funcFile *ast.File, funcDir string, setImports astvisit.Imports) (string, error) {
	typeExpr, err := parser.ParseExpr(typeName)
	if err != nil {
		return "", fmt.Errorf("invalid type: %w", err)
	}
	qualifiers := make(map[string]struct{})
	astvisit.TypeExprNameQualifyers(typeExpr, qualifiers)
	for qualifier := range qualifiers {
		if _, ok := target.pathOfName(qualifier); ok {
			delete(qualifiers, qualifier)
		}
	}
	packageRemap, err := target.addImportsOfQualifiers(funcFile, funcDir, qualifiers, setImports)
	if err != nil {
		return "", err
	}
	return remapPackageQualifiers(typeName, packageRemap), nil
}

// signatureString returns the signature of a function in the format
// of the String method of function.ReflectWrapper, like
// "pkg.Func(a int, b int, more ...string) (int, error)",
// with one argument per name and the results without names.
// The types are formatted like in the file of the function
// with the exported names of its package qualified with the package name,
// except empty interfaces, which are written as any like by function.ReflectWrapper,
// which can't know if the source code wrote interface{} or any.
func signatureString(qualifiedFuncName string, argNames, argTypes, resultTypes []string) string {
	var b strings.Builder
	b.WriteString(qualifiedFuncName)
	b.WriteByte('(')
	for i, argName := range argNames {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(argName)
		b.WriteByte(' ')
		b.WriteString(emptyInterfaceAsAny(argTypes[i]))
	}
	b.WriteByte(')')
	switch len(resultTypes) {
	case 0:
	case 1:
		b.WriteByte(' ')
		b.WriteString(emptyInterfaceAsAny(resultTypes[0]))
	default:
		b.WriteString(" (")
		for i, resultType := range resultTypes {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(emptyInterfaceAsAny(resultType))
		}
		b.WriteByte(')')
	}
	return b.String()
}

// emptyInterfaceAsAny returns typeName with empty interfaces written as any,
// also in types like map[string]interface{}.
func emptyInterfaceAsAny(typeName string) string {
	return strings.ReplaceAll(typeName, "interface{}", "any")
}

// checkNoDotImportedTypes returns an error if the file of a function of another package
// has dot imports and the parameters or results of the function use unqualified exported names,
// because they are qualified with funcPackage, which is wrong for names of a dot imported package.
func checkNoDotImportedTypes(funcFile *ast.File, funcDecl *ast.FuncDecl, funcPackage string) error {
	var dotImports []string
	for _, imp := range funcFile.Imports {
		if imp.Name != nil && imp.Name.Name == "." {
			dotImports = append(dotImports, imp.Path.Value)
		}
	}
	if len(dotImports) == 0 {
		return nil
	}
	for _, fieldList := range []*ast.FieldList{funcDecl.Type.Params, funcDecl.Type.Results} {
		if fieldList == nil {
			continue
		}
		for _, field := range fieldList.List {
			if astvisit.ExprStringWithExportedNameQualifyer(field.Type, funcPackage) != astvisit.ExprStringWithExportedNameQualifyer(field.Type, "") {
				return fmt.Errorf("can't wrap %s.%s: dot imports in the file of a wrapped function from another package are not supported, the types of its arguments or results could be from the dot import %s", funcPackage, funcDecl.Name.Name, strings.Join(dotImports, ", "))
			}
		}
	}
	return nil
}

// isAnyType reports if typeName is the empty interface,
// so that any value can be assigned to it without type assertion.
func isAnyType(typeName string) bool {
	return typeName == "any" || typeName == "interface{}"
}

// typeAssertionIsAssignability reports if a type assertion to the type
// succeeds for exactly the values that are assignable to the type,
// so that the generated Call doesn't need reflection to accept
// the assignable values of other types like ReflectWrapper.
//
// That's the case for interface types, because a value of another type
// is assignable to an interface type if it implements it,
// and for the predeclared types like int, because a value of another type
// is only assignable to a type with an identical underlying type
// if one of the types is no named type, and the underlying type
// of a predeclared type is no type literal that could be its type.
// A named type like time.Duration could be an interface type
// or have a type literal as underlying type, like type MyInts []int,
// which the generator doesn't know without type checking.
// A predeclared type name that the package of the function
// declares as another type is not supported.
func typeAssertionIsAssignability(typeName string) bool {
	expr, err := parser.ParseExpr(typeName)
	if err != nil {
		return false
	}
	switch expr := expr.(type) {
	case *ast.Ident:
		_, predeclared := types.Universe.Lookup(expr.Name).(*types.TypeName)
		return predeclared
	case *ast.InterfaceType:
		return true
	}
	return false
}

// sliceOfVariadic returns the slice type of a variadic argument type like "...int",
// and other types unchanged.
func sliceOfVariadic(typeName string) string {
	if elemType, ok := strings.CutPrefix(typeName, "..."); ok {
		return "[]" + elemType
	}
	return typeName
}

// reflectTypeOfTypeName generates code for obtaining a reflect.Type for a given type name.
// It uses reflect.TypeFor which is the modern generic-based approach.
//
// Variadic parameters (...T) are converted to slices ([]T) before creating the reflect.Type.
//
// Example:
//   - "string" -> "reflect.TypeFor[string]()"
//   - "...int" -> "reflect.TypeFor[[]int]()"
//   - "func(...int)" -> "reflect.TypeFor[func(...int)]()"
func reflectTypeOfTypeName(typeName string) string {
	return fmt.Sprintf("reflect.TypeFor[%s]()", sliceOfVariadic(typeName))
}

// remapPackageQualifiers translates package qualifiers in a type name using the provided mapping.
// For example, "gmail.Label" with remap["gmail"]="gmailapi" becomes "gmailapi.Label".
// Only identifiers followed by a dot are renamed, not other identifiers
// that contain a renamed name or the contents of struct tags.
func remapPackageQualifiers(typeName string, packageRemap map[string]string) string {
	if len(packageRemap) == 0 {
		return typeName
	}
	src := []byte(typeName)
	fset := token.NewFileSet()
	var scan scanner.Scanner
	scan.Init(fset.AddFile("", fset.Base(), len(src)), src, nil, 0)
	type tok struct {
		offset int
		tok    token.Token
		lit    string
	}
	var toks []tok
	for {
		pos, t, lit := scan.Scan()
		if t == token.EOF {
			break
		}
		toks = append(toks, tok{offset: fset.Position(pos).Offset, tok: t, lit: lit})
	}
	var b strings.Builder
	written := 0
	for i, t := range toks {
		isQualifier := t.tok == token.IDENT &&
			i+1 < len(toks) && toks[i+1].tok == token.PERIOD &&
			(i == 0 || toks[i-1].tok != token.PERIOD)
		if newName, ok := packageRemap[t.lit]; ok && isQualifier {
			b.WriteString(typeName[written:t.offset])
			b.WriteString(newName)
			written = t.offset + len(t.lit)
		}
	}
	b.WriteString(typeName[written:])
	return b.String()
}
