package function

import (
	"context"
)

var (
	_ CallWrapper                 = CallWrapperFunc(nil)
	_ CallWithStringsWrapper      = CallWithStringsWrapperFunc(nil)
	_ CallWithNamedStringsWrapper = CallWithNamedStringsWrapperFunc(nil)
)

type Wrapper interface {
	Description
	CallWrapper
	CallWithStringsWrapper
	CallWithNamedStringsWrapper
}

type CallWrapper interface {
	Call(ctx context.Context, args []interface{}) (results []interface{}, err error)
}

type CallWrapperFunc func(ctx context.Context, args []interface{}) (results []interface{}, err error)

func (f CallWrapperFunc) Call(ctx context.Context, args []interface{}) (results []interface{}, err error) {
	return f(ctx, args)
}

type CallWithStringsWrapper interface {
	CallWithStrings(ctx context.Context, args ...string) (results []interface{}, err error)
}

type CallWithStringsWrapperFunc func(ctx context.Context, args ...string) (results []interface{}, err error)

func (f CallWithStringsWrapperFunc) CallWithStrings(ctx context.Context, args ...string) (results []interface{}, err error) {
	return f(ctx, args...)
}

type CallWithNamedStringsWrapper interface {
	CallWithNamedStrings(ctx context.Context, args map[string]string) (results []interface{}, err error)
}

type CallWithNamedStringsWrapperFunc func(ctx context.Context, args map[string]string) (results []interface{}, err error)

func (f CallWithNamedStringsWrapperFunc) CallWithNamedStrings(ctx context.Context, args map[string]string) (results []interface{}, err error) {
	return f(ctx, args)
}

func WrapperTODO(f interface{}) Wrapper {
	panic("function.WrapperTODO: run gen-func-wrappers")
}

func CallWrapperTODO(f interface{}) CallWrapper {
	panic("function.CallWrapperTODO: run gen-func-wrappers")
}

func CallWithStringsWrapperTODO(f interface{}) CallWithStringsWrapper {
	panic("function.CallWithStringsWrapperTODO: run gen-func-wrappers")
}

func CallWithNamedStringsWrapperTODO(f interface{}) CallWithNamedStringsWrapper {
	panic("function.CallWithNamedStringsWrapperTODO: run gen-func-wrappers")
}
