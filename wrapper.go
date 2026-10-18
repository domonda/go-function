package function

import (
	"context"
	"reflect"
)

// Wrapper is the main interface for wrapped functions, providing multiple calling conventions.
// It combines Description (function metadata), CallWrapper (direct calls),
// CallWithStringsWrapper (string argument parsing), CallWithNamedStringsWrapper (named arguments),
// and CallWithJSONWrapper (JSON argument parsing).
//
// Wrapper implementations can be created either through reflection using ReflectWrapper,
// or through code generation using gen-func-wrappers for better performance.
type Wrapper interface {
	Description
	CallWrapper
	CallWithStringsWrapper
	CallWithNamedStringsWrapper
	CallWithJSONWrapper
}

// WrapperTODO is a placeholder function used during development with code generation.
// It should be replaced by running gen-func-wrappers, which generates optimized
// wrapper code without reflection overhead.
//
// Usage:
//
//	var MyFuncWrapper = function.WrapperTODO(MyFunc)
//
// Then add the code generator as tool to your module once
// and run it:
//
//	go get -tool github.com/domonda/go-function/cmd/gen-func-wrappers@latest
//	go tool gen-func-wrappers ./...
//
// This will replace WrapperTODO calls with generated wrapper implementations.
// The function panics at runtime to ensure you don't forget to run the code generator.
func WrapperTODO(function any) Wrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.WrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.WrapperTODO: run gen-func-wrappers")
}

// CallWrapper provides the ability to call a function with a slice of any values.
// The context is passed as the first argument if the function accepts context.Context,
// so args don't include a context.Context argument.
// The arg for a variadic last argument is a slice of its element type.
// Missing or untyped nil args are passed as zero values.
// ReflectWrapper and generated wrappers return an error for too many args
// or args not assignable to their argument type.
type CallWrapper interface {
	Call(ctx context.Context, args []any) (results []any, err error)
}

// CallWrapperTODO is a placeholder for code generation. See WrapperTODO for details.
func CallWrapperTODO(function any) CallWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWrapperTODO: run gen-func-wrappers")
}

// CallWithStringsWrapper provides the ability to call a function with string arguments
// that are automatically converted to the required types.
// String conversion supports basic types, time values, slices, structs (as JSON), and more.
// See ScanString for conversion details.
//
// Like in a Go call, every string from the position of a variadic last argument on
// is one element of the variadic slice, also a single string, so it is never parsed
// as slice literal. Elements are converted to the element type,
// elements of type any get the unconverted string.
// No strings at the variadic position result in no variadic elements.
// Missing strings for other arguments are passed as zero values.
// ReflectWrapper and generated wrappers return an error
// for too many strings for a function without variadic argument.
//
// Security: the strings become argument values as they are, so don't pass
// untrusted input for arguments of types that name files or other resources,
// like fs.File of github.com/ungerik/go-fs which is a path on the server.
type CallWithStringsWrapper interface {
	CallWithStrings(ctx context.Context, args ...string) (results []any, err error)
}

// CallWithStringsWrapperTODO is a placeholder for code generation. See WrapperTODO for details.
func CallWithStringsWrapperTODO(function any) CallWithStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithStringsWrapperTODO: run gen-func-wrappers")
}

// CallWithNamedStringsWrapper provides the ability to call a function with named string arguments.
// The argument names must match the function's parameter names.
// String values are automatically converted to the required types.
// Missing arguments will use zero values.
// The single string of a variadic last argument is parsed as slice literal like "[1,2]".
//
// Security: the strings become argument values as they are, so don't pass
// untrusted input for arguments of types that name files or other resources,
// like fs.File of github.com/ungerik/go-fs which is a path on the server.
type CallWithNamedStringsWrapper interface {
	CallWithNamedStrings(ctx context.Context, args map[string]string) (results []any, err error)
}

// CallWithNamedStringsWrapperTODO is a placeholder for code generation. See WrapperTODO for details.
func CallWithNamedStringsWrapperTODO(function any) CallWithNamedStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithNamedStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithNamedStringsWrapperTODO: run gen-func-wrappers")
}

// CallWithJSONWrapper provides the ability to call a function with JSON-encoded arguments.
// The JSON must be an object with the argument names as keys,
// a JSON array of arguments is not supported,
// ReflectWrapper and generated wrappers return an error for JSON that is no object,
// except null, which they handle like an empty object.
// Arguments are unmarshaled using json.Unmarshal into the required types,
// except error arguments, which are JSON strings where an empty string
// or null is no error.
//
// Security: the JSON values become argument values as they are, so don't pass
// untrusted input for arguments of types that name files or other resources,
// like fs.File of github.com/ungerik/go-fs which is a path on the server.
type CallWithJSONWrapper interface {
	CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error)
}

// CallWithJSONWrapperTODO is a placeholder for code generation. See WrapperTODO for details.
func CallWithJSONWrapperTODO(function any) CallWithJSONWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithJSONWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithJSONWrapperTODO: run gen-func-wrappers")
}

// Compile-time assertions that the *Func types implement their respective interfaces.
var (
	_ CallWrapper                 = CallWrapperFunc(nil)
	_ CallWithStringsWrapper      = CallWithStringsWrapperFunc(nil)
	_ CallWithNamedStringsWrapper = CallWithNamedStringsWrapperFunc(nil)
	_ CallWithJSONWrapper         = CallWithJSONWrapperFunc(nil)
)

// CallWrapperFunc is a function type that implements CallWrapper.
// It allows any function with the matching signature to be used as a CallWrapper.
type CallWrapperFunc func(ctx context.Context, args []any) (results []any, err error)

func (f CallWrapperFunc) Call(ctx context.Context, args []any) (results []any, err error) {
	return f(ctx, args)
}

// CallWithStringsWrapperFunc is a function type that implements CallWithStringsWrapper.
// It allows any function with the matching signature to be used as a CallWithStringsWrapper.
type CallWithStringsWrapperFunc func(ctx context.Context, args ...string) (results []any, err error)

func (f CallWithStringsWrapperFunc) CallWithStrings(ctx context.Context, args ...string) (results []any, err error) {
	return f(ctx, args...)
}

// CallWithNamedStringsWrapperFunc is a function type that implements CallWithNamedStringsWrapper.
// It allows any function with the matching signature to be used as a CallWithNamedStringsWrapper.
type CallWithNamedStringsWrapperFunc func(ctx context.Context, args map[string]string) (results []any, err error)

func (f CallWithNamedStringsWrapperFunc) CallWithNamedStrings(ctx context.Context, args map[string]string) (results []any, err error) {
	return f(ctx, args)
}

// CallWithJSONWrapperFunc is a function type that implements CallWithJSONWrapper.
// It allows any function with the matching signature to be used as a CallWithJSONWrapper.
type CallWithJSONWrapperFunc func(ctx context.Context, argsJSON []byte) (results []any, err error)

func (f CallWithJSONWrapperFunc) CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error) {
	return f(ctx, argsJSON)
}

// VoidFuncWrapper is a Wrapper for a function without arguments and without results.
// It implements all wrapper interfaces and can be used for simple callback-style functions.
// Its Call methods ignore all arguments.
//
// Example:
//
//	var PrintHello VoidFuncWrapper = func() { fmt.Println("Hello") }
//	PrintHello.Call(ctx, nil) // Prints "Hello"
type VoidFuncWrapper func()

func (VoidFuncWrapper) String() string { return "func()" }
func (VoidFuncWrapper) Name() string   { return "func()" }

func (VoidFuncWrapper) NumArgs() int                { return 0 }
func (VoidFuncWrapper) ContextArg() bool            { return false }
func (VoidFuncWrapper) NumResults() int             { return 0 }
func (VoidFuncWrapper) ErrorResult() bool           { return false }
func (VoidFuncWrapper) ArgNames() []string          { return nil }
func (VoidFuncWrapper) ArgDescriptions() []string   { return nil }
func (VoidFuncWrapper) ArgTypes() []reflect.Type    { return nil }
func (VoidFuncWrapper) ResultTypes() []reflect.Type { return nil }

func (f VoidFuncWrapper) Call(context.Context, []any) ([]any, error) {
	f()
	return nil, nil
}

func (f VoidFuncWrapper) CallWithStrings(context.Context, ...string) ([]any, error) {
	f()
	return nil, nil
}

func (f VoidFuncWrapper) CallWithNamedStrings(context.Context, map[string]string) ([]any, error) {
	f()
	return nil, nil
}

func (f VoidFuncWrapper) CallWithJSON(context.Context, []byte) (results []any, err error) {
	f()
	return nil, nil
}

// ErrorFuncWrapper is a Wrapper for a function without arguments that returns only an error.
// It implements all wrapper interfaces and is useful for validation or initialization functions.
// Its Call methods ignore all arguments.
//
// Example:
//
//	var ValidateConfig ErrorFuncWrapper = func() error {
//	    if config == nil { return errors.New("config is nil") }
//	    return nil
//	}
//	_, err := ValidateConfig.Call(ctx, nil)
type ErrorFuncWrapper func() error

func (ErrorFuncWrapper) String() string { return "func() error" }
func (ErrorFuncWrapper) Name() string   { return "func() error" }

func (ErrorFuncWrapper) NumArgs() int                { return 0 }
func (ErrorFuncWrapper) ContextArg() bool            { return false }
func (ErrorFuncWrapper) NumResults() int             { return 1 }
func (ErrorFuncWrapper) ErrorResult() bool           { return true }
func (ErrorFuncWrapper) ArgNames() []string          { return nil }
func (ErrorFuncWrapper) ArgDescriptions() []string   { return nil }
func (ErrorFuncWrapper) ArgTypes() []reflect.Type    { return nil }
func (ErrorFuncWrapper) ResultTypes() []reflect.Type { return []reflect.Type{typeOfError} }

func (f ErrorFuncWrapper) Call(context.Context, []any) ([]any, error) {
	return nil, f()
}

func (f ErrorFuncWrapper) CallWithStrings(context.Context, ...string) ([]any, error) {
	return nil, f()
}

func (f ErrorFuncWrapper) CallWithNamedStrings(context.Context, map[string]string) ([]any, error) {
	return nil, f()
}

func (f ErrorFuncWrapper) CallWithJSON(context.Context, []byte) (results []any, err error) {
	return nil, f()
}
