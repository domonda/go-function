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

func WrapperTODO(function interface{}) Wrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.WrapperTODO must be used with a function as argument, then run gen-func-wrappers to to replace it with generated code")
	}
	panic("function.WrapperTODO: run gen-func-wrappers")
}

type CallWrapper interface {
	Call(ctx context.Context, args []interface{}) (results []interface{}, err error)
}

func CallWrapperTODO(function interface{}) CallWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWrapperTODO must be used with a function as argument, then run gen-func-wrappers to to replace it with generated code")
	}
	panic("function.CallWrapperTODO: run gen-func-wrappers")
}

type CallWithStringsWrapper interface {
	CallWithStrings(ctx context.Context, args ...string) (results []interface{}, err error)
}

func CallWithStringsWrapperTODO(function interface{}) CallWithStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithStringsWrapperTODO: run gen-func-wrappers")
}

type CallWithNamedStringsWrapper interface {
	CallWithNamedStrings(ctx context.Context, args map[string]string) (results []interface{}, err error)
}

func CallWithNamedStringsWrapperTODO(function interface{}) CallWithNamedStringsWrapper {
	if reflect.ValueOf(function).Kind() != reflect.Func {
		panic("function.CallWithNamedStringsWrapperTODO must be used with a function as argument, then run gen-func-wrappers to replace it with generated code")
	}
	panic("function.CallWithNamedStringsWrapperTODO: run gen-func-wrappers")
}

type CallWithJSONWrapper interface {
	CallWithJSON(ctx context.Context, argsJSON []byte) (results []interface{}, err error)
}

func CallWithJSONWrapperTODO(function interface{}) CallWithJSONWrapper {
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

type CallWrapperFunc func(ctx context.Context, args []interface{}) (results []interface{}, err error)

func (f CallWrapperFunc) Call(ctx context.Context, args []interface{}) (results []interface{}, err error) {
	return f(ctx, args)
}

type CallWithStringsWrapperFunc func(ctx context.Context, args ...string) (results []interface{}, err error)

func (f CallWithStringsWrapperFunc) CallWithStrings(ctx context.Context, args ...string) (results []interface{}, err error) {
	return f(ctx, args...)
}

type CallWithNamedStringsWrapperFunc func(ctx context.Context, args map[string]string) (results []interface{}, err error)

func (f CallWithNamedStringsWrapperFunc) CallWithNamedStrings(ctx context.Context, args map[string]string) (results []interface{}, err error) {
	return f(ctx, args)
}

type CallWithJSONWrapperFunc func(ctx context.Context, argsJSON []byte) (results []interface{}, err error)

func (f CallWithJSONWrapperFunc) CallWithJSON(ctx context.Context, argsJSON []byte) (results []interface{}, err error) {
	return f(ctx, argsJSON)
}
