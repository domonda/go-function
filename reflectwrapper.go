package function

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ReflectWrapper returns a Wrapper for the passed function
// using reflection and the passed argNames.
// The number of passed argNames must match the number
// of function arguments.
// Except when the function only has one argument
// of type context.Context then "ctx" is assumed
// as argument name in case no name has been passed.
func ReflectWrapper(function any, argNames ...string) (Wrapper, error) {
	return newReflectWrapper(function, argNames)
}

// MustReflectWrapper calls ReflectWrapper and panics any error.
func MustReflectWrapper(function any, argNames ...string) Wrapper {
	w, err := newReflectWrapper(function, argNames)
	if err != nil {
		panic(err)
	}
	return w
}

// newReflectWrapper unexported function returns testable struct type
func newReflectWrapper(function any, argNames []string) (*reflectWrapper, error) {
	funcVal := reflect.ValueOf(function)
	if funcVal.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected function but got %T", function)
	}
	funcType := funcVal.Type()
	switch {
	case len(argNames) == 0 && funcType.NumIn() == 1 && funcType.In(0) == typeOfContext:
		argNames = []string{"ctx"}

	case len(argNames) != funcType.NumIn():
		return nil, fmt.Errorf("%d argNames passed, but %s has %d arguments", len(argNames), funcType, funcType.NumIn())
	}
	// Clone so that a slice passed as argNames... can be modified by the caller
	return &reflectWrapper{funcVal, funcType, slices.Clone(argNames)}, nil
}

type reflectWrapper struct {
	funcVal  reflect.Value
	funcType reflect.Type
	argNames []string
}

// String returns the signature of the wrapped function.
func (f *reflectWrapper) String() string {
	argTypes := make([]reflect.Type, f.funcType.NumIn())
	for i := range argTypes {
		argTypes[i] = f.funcType.In(i)
	}
	resultTypes := make([]reflect.Type, f.funcType.NumOut())
	for i := range resultTypes {
		resultTypes[i] = f.funcType.Out(i)
	}
	return signatureString(f.Name(), f.argNames, argTypes, resultTypes, f.funcType.IsVariadic())
}

// Name returns the name of the wrapped function like "pkg.Func"
// qualified with the package name guessed from its import path,
// or "func" if the name can't be determined.
func (f *reflectWrapper) Name() string {
	if fn := runtime.FuncForPC(f.funcVal.Pointer()); fn != nil {
		return qualifiedFuncName(fn.Name())
	}
	return "func"
}

// qualifiedFuncName returns the name of a function as reported by the runtime,
// like "github.com/domonda/go-function.ScanString", qualified with the
// package name that goimports assumes for the import path, like "function.ScanString",
// because the runtime only knows the import path and not the package name.
func qualifiedFuncName(runtimeName string) string {
	// The import path ends at the first dot after its last slash,
	// because the runtime escapes dots in the last element of the import path.
	// Type arguments like in "pkg.F[...]" follow the import path.
	pathAndName := runtimeName
	if bracket := strings.IndexByte(runtimeName, '['); bracket >= 0 {
		pathAndName = runtimeName[:bracket]
	}
	slash := strings.LastIndexByte(pathAndName, '/')
	dot := strings.IndexByte(runtimeName[slash+1:], '.')
	if dot < 0 {
		return runtimeName
	}
	importPath, name := runtimeName[:slash+1+dot], runtimeName[slash+1+dot:]
	if unescaped, err := url.PathUnescape(importPath); err == nil {
		importPath = unescaped
	}
	return cmp.Or(assumedPackageName(importPath), path.Base(importPath)) + name
}

// assumedPackageName returns the package name that goimports assumes
// for an import path: the last element that is not a major version like v2,
// without a "go-" prefix, up to the first character that can't be in an identifier.
// So "github.com/domonda/go-function" is "function" and "gopkg.in/yaml.v3" is "yaml".
func assumedPackageName(importPath string) string {
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			if dir := path.Dir(importPath); dir != "." {
				base = path.Base(dir)
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, notIdentifier); i >= 0 {
		base = base[:i]
	}
	return base
}

func notIdentifier(r rune) bool {
	return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_' ||
		r >= utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)))
}

func (f *reflectWrapper) NumArgs() int {
	return f.funcType.NumIn()
}

func (f *reflectWrapper) ContextArg() bool {
	return f.funcType.NumIn() > 0 && f.funcType.In(0) == typeOfContext
}

func (f *reflectWrapper) Variadic() bool {
	return f.funcType.IsVariadic()
}

func (f *reflectWrapper) NumResults() int {
	return f.funcType.NumOut()
}

func (f *reflectWrapper) ErrorResult() bool {
	numOut := f.funcType.NumOut()
	return numOut > 0 && f.funcType.Out(numOut-1) == typeOfError
}

func (f *reflectWrapper) ArgNames() []string {
	return slices.Clone(f.argNames)
}

func (f *reflectWrapper) ArgDescriptions() []string {
	numIn := f.funcType.NumIn()
	if numIn == 0 {
		return nil
	}
	return make([]string, numIn)
}

func (f *reflectWrapper) ArgTypes() []reflect.Type {
	numIn := f.funcType.NumIn()
	if numIn == 0 {
		return nil
	}
	a := make([]reflect.Type, numIn)
	for i := range a {
		a[i] = f.funcType.In(i)
	}
	return a
}

func (f *reflectWrapper) ResultTypes() []reflect.Type {
	numResults := f.NumResults()
	if numResults == 0 {
		return nil
	}
	r := make([]reflect.Type, numResults)
	for i := range r {
		r[i] = f.funcType.Out(i)
	}
	return r
}

func (f *reflectWrapper) call(in []reflect.Value) (results []any, err error) {
	// Replace untyped nil values with typed zero values
	for i := range in {
		if !in[i].IsValid() {
			in[i] = reflect.Zero(f.funcType.In(i))
		}
	}
	var out []reflect.Value
	if f.funcType.IsVariadic() {
		// The last argument is a slice with the variadic arguments
		out = f.funcVal.CallSlice(in)
	} else {
		out = f.funcVal.Call(in)
	}
	resultsLen := len(out)
	if f.ErrorResult() {
		resultsLen--
		err, _ = reflect.TypeAssert[error](out[len(out)-1])
	}
	results = make([]any, resultsLen)
	for i := range results {
		results[i] = out[i].Interface()
	}
	return results, err
}

func (f *reflectWrapper) Call(ctx context.Context, args []any) (results []any, err error) {
	in := make([]reflect.Value, f.NumArgs())
	offs := 0
	if f.ContextArg() {
		offs = 1
		in[0] = reflect.ValueOf(ctx)
	}
	if len(args) > len(in)-offs {
		return nil, fmt.Errorf("%d arguments passed, but function %s takes %d", len(args), f, len(in)-offs)
	}
	for i, arg := range args {
		argVal := reflect.ValueOf(arg)
		// Untyped nil is passed as zero value by f.call
		if argVal.IsValid() && !argVal.Type().AssignableTo(f.funcType.In(i+offs)) {
			return nil, fmt.Errorf("can't use %s as type %s for argument '%s' of function %s", argVal.Type(), f.funcType.In(i+offs), f.argNames[i+offs], f)
		}
		in[i+offs] = argVal
	}
	return f.call(in)
}

func (f *reflectWrapper) CallWithStrings(ctx context.Context, strs ...string) (results []any, err error) {
	in := make([]reflect.Value, f.NumArgs())
	offs := 0
	if f.ContextArg() {
		offs = 1
		in[0] = reflect.ValueOf(ctx)
	}
	variadic := f.funcType.IsVariadic()
	if len(strs) > len(in)-offs && !variadic {
		return nil, fmt.Errorf("%d arguments passed, but function %s takes %d", len(strs), f, len(in)-offs)
	}
	for i := offs; i < len(in); i++ {
		argType := f.funcType.In(i)
		if i-offs >= len(strs) {
			// Pass default value if not enough strs
			in[i] = reflect.Zero(argType)
			continue
		}
		if variadic && i == len(in)-1 {
			// Like in a Go call every string from the variadic
			// argument on is one element, also a single string
			variadicStrs := strs[i-offs:]
			elems := reflect.MakeSlice(argType, len(variadicStrs), len(variadicStrs))
			for j, str := range variadicStrs {
				if argType.Elem() == typeOfAny {
					elems.Index(j).Set(reflect.ValueOf(str))
					continue
				}
				err = ScanString(str, elems.Index(j).Addr().Interface())
				if err != nil {
					return nil, NewErrParseArgString(err, f, f.argNames[i], str)
				}
			}
			in[i] = elems
			continue
		}
		str := strs[i-offs]
		if argType == typeOfAny {
			// Pass string directly for argument of type any
			in[i] = reflect.ValueOf(str)
			continue
		}
		destPtr := reflect.New(argType)
		err = ScanString(str, destPtr.Interface())
		if err != nil {
			return nil, NewErrParseArgString(err, f, f.argNames[i], str)
		}
		in[i] = destPtr.Elem()
	}
	return f.call(in)
}

func (f *reflectWrapper) CallWithNamedStrings(ctx context.Context, strs map[string]string) (results []any, err error) {
	in := make([]reflect.Value, f.NumArgs())
	offs := 0
	if f.ContextArg() {
		offs = 1
		in[0] = reflect.ValueOf(ctx)
	}
	for i := offs; i < len(in); i++ {
		argType := f.funcType.In(i)
		argName := f.argNames[i]
		if str, ok := strs[argName]; ok {
			if argType == typeOfAny {
				// Pass string directly for argument of type any
				in[i] = reflect.ValueOf(str)
				continue
			}
			destPtr := reflect.New(argType)
			err = ScanString(str, destPtr.Interface())
			if err != nil {
				return nil, NewErrParseArgString(err, f, f.argNames[i], str)
			}
			in[i] = destPtr.Elem()
		}
	}
	return f.call(in)
}

func (f *reflectWrapper) CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error) {
	args := make(map[string]json.RawMessage)
	err = json.Unmarshal(argsJSON, &args)
	if err != nil {
		return nil, NewErrParseArgsJSON(err, f)
	}
	in := make([]reflect.Value, f.NumArgs())
	offs := 0
	if f.ContextArg() {
		offs = 1
		in[0] = reflect.ValueOf(ctx)
	}
	for i := offs; i < len(in); i++ {
		argType := f.funcType.In(i)
		destPtr := reflect.New(argType)
		argName := f.argNames[i]
		if arg, ok := args[argName]; ok {
			if argType == typeOfError {
				// json.Unmarshal does not work for errors
				// so unmarshal string and create error from it
				var errStr string
				err = json.Unmarshal(arg, &errStr)
				if err != nil {
					return nil, NewErrParseArgJSON(err, f, argName)
				}
				var err error
				if errStr != "" {
					err = errors.New(errStr)
				}
				in[i] = reflect.ValueOf(err)
				continue
			}
			err = json.Unmarshal(arg, destPtr.Interface())
			if err != nil {
				return nil, NewErrParseArgJSON(err, f, argName)
			}
		}
		in[i] = destPtr.Elem()
	}
	return f.call(in)
}
