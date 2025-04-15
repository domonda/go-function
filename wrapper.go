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

// PlainFuncWrapper returns a Wrapper for a function call
// without arguments and without results.
func PlainFuncWrapper(name string, call func()) Wrapper {
	return plainFuncWrapper{name, call}
}

type plainFuncWrapper struct {
	name string
	call func()
}

func (f plainFuncWrapper) String() string { return f.name }
func (f plainFuncWrapper) Name() string   { return f.name }

func (plainFuncWrapper) NumArgs() int                { return 0 }
func (plainFuncWrapper) ContextArg() bool            { return false }
func (plainFuncWrapper) NumResults() int             { return 0 }
func (plainFuncWrapper) ErrorResult() bool           { return false }
func (plainFuncWrapper) ArgNames() []string          { return nil }
func (plainFuncWrapper) ArgDescriptions() []string   { return nil }
func (plainFuncWrapper) ArgTypes() []reflect.Type    { return nil }
func (plainFuncWrapper) ResultTypes() []reflect.Type { return nil }

func (f plainFuncWrapper) Call(context.Context, []any) ([]any, error) {
	f.call()
	return nil, nil
}

func (f plainFuncWrapper) CallWithStrings(context.Context, ...string) ([]any, error) {
	f.call()
	return nil, nil
}

func (f plainFuncWrapper) CallWithNamedStrings(context.Context, map[string]string) ([]any, error) {
	f.call()
	return nil, nil
}

func (f plainFuncWrapper) CallWithJSON(context.Context, []byte) (results []any, err error) {
	f.call()
	return nil, nil
}

// PlainErrorFuncWrapper returns a Wrapper for a function call
// without arguments and with one error result.
func PlainErrorFuncWrapper(name string, call func() error) Wrapper {
	return plainErrorFuncWrapper{name, call}
}

type plainErrorFuncWrapper struct {
	name string
	call func() error
}

func (f plainErrorFuncWrapper) String() string { return f.name }
func (f plainErrorFuncWrapper) Name() string   { return f.name }

func (plainErrorFuncWrapper) NumArgs() int                { return 0 }
func (plainErrorFuncWrapper) ContextArg() bool            { return false }
func (plainErrorFuncWrapper) NumResults() int             { return 1 }
func (plainErrorFuncWrapper) ErrorResult() bool           { return true }
func (plainErrorFuncWrapper) ArgNames() []string          { return nil }
func (plainErrorFuncWrapper) ArgDescriptions() []string   { return nil }
func (plainErrorFuncWrapper) ArgTypes() []reflect.Type    { return nil }
func (plainErrorFuncWrapper) ResultTypes() []reflect.Type { return []reflect.Type{typeOfError} }

func (f plainErrorFuncWrapper) Call(context.Context, []any) ([]any, error) {
	return nil, f.call()
}

func (f plainErrorFuncWrapper) CallWithStrings(context.Context, ...string) ([]any, error) {
	return nil, f.call()
}

func (f plainErrorFuncWrapper) CallWithNamedStrings(context.Context, map[string]string) ([]any, error) {
	return nil, f.call()
}

func (f plainErrorFuncWrapper) CallWithJSON(context.Context, []byte) (results []any, err error) {
	return nil, f.call()
}
