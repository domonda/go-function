package function

import (
	"context"
	"reflect"
)

type Wrapper interface {
	Description
	CallWrapper
	CallWithStringsWrapper
	CallWithNamedStringsWrapper
	CallWithJSONWrapper
}

func WrapperTODO(function any) Wrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.WrapperTODO must be used with a function as argument, then run gen-func-wrappers to to replace it with generated code")
	}
	panic("function.WrapperTODO: run gen-func-wrappers")
}

type CallWrapper interface {
	Call(ctx context.Context, args []any) (results []any, err error)
}

func CallWrapperTODO(function any) CallWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWrapperTODO must be used with a function as argument, then run gen-func-wrappers to to replace it with generated code")
	}
	panic("function.CallWrapperTODO: run gen-func-wrappers")
}

type CallWithStringsWrapper interface {
	CallWithStrings(ctx context.Context, args ...string) (results []any, err error)
}

func CallWithStringsWrapperTODO(function any) CallWithStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithStringsWrapperTODO: run gen-func-wrappers")
}

type CallWithNamedStringsWrapper interface {
	CallWithNamedStrings(ctx context.Context, args map[string]string) (results []any, err error)
}

func CallWithNamedStringsWrapperTODO(function any) CallWithNamedStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithNamedStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithNamedStringsWrapperTODO: run gen-func-wrappers")
}

type CallWithJSONWrapper interface {
	CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error)
}

func CallWithJSONWrapperTODO(function any) CallWithJSONWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithJSONWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithJSONWrapperTODO: run gen-func-wrappers")
}

// Implementations of the call interfaces as higher order functions
var (
	_ CallWrapper                 = CallWrapperFunc(nil)
	_ CallWithStringsWrapper      = CallWithStringsWrapperFunc(nil)
	_ CallWithNamedStringsWrapper = CallWithNamedStringsWrapperFunc(nil)
	_ CallWithJSONWrapper         = CallWithJSONWrapperFunc(nil)
)

type CallWrapperFunc func(ctx context.Context, args []any) (results []any, err error)

func (f CallWrapperFunc) Call(ctx context.Context, args []any) (results []any, err error) {
	return f(ctx, args)
}

type CallWithStringsWrapperFunc func(ctx context.Context, args ...string) (results []any, err error)

func (f CallWithStringsWrapperFunc) CallWithStrings(ctx context.Context, args ...string) (results []any, err error) {
	return f(ctx, args...)
}

type CallWithNamedStringsWrapperFunc func(ctx context.Context, args map[string]string) (results []any, err error)

func (f CallWithNamedStringsWrapperFunc) CallWithNamedStrings(ctx context.Context, args map[string]string) (results []any, err error) {
	return f(ctx, args)
}

type CallWithJSONWrapperFunc func(ctx context.Context, argsJSON []byte) (results []any, err error)

func (f CallWithJSONWrapperFunc) CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error) {
	return f(ctx, argsJSON)
}

// VoidFuncWrapper returns a Wrapper for a function call
// without arguments and without results.
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

// ErrorFuncWrapper returns a Wrapper for a function call
// without arguments and with one error result.
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
