package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	var (
		funcVal  = reflect.ValueOf(function)
		funcType = funcVal.Type()
	)
	switch {
	case funcType.Kind() != reflect.Func:
		return nil, fmt.Errorf("expected function but got %s", funcType)

	case len(argNames) == 0 && funcType.NumIn() == 1 && funcType.In(0) == typeOfContext:
		argNames = []string{"ctx"}

	case len(argNames) != funcType.NumIn():
		return nil, fmt.Errorf("%d argNames passed, but %s has %d arguments", len(argNames), funcType, funcType.NumIn())
	}
	return &reflectWrapper{funcVal, funcType, argNames}, nil
}

type reflectWrapper struct {
	funcVal  reflect.Value
	funcType reflect.Type
	argNames []string
}

func (f *reflectWrapper) String() string {
	return f.funcVal.String()
}

func (f *reflectWrapper) Name() string {
	return f.String()
}

func (f *reflectWrapper) NumArgs() int {
	return f.funcType.NumIn()
}

func (f *reflectWrapper) ContextArg() bool {
	return f.funcType.NumIn() > 0 && f.funcType.In(0) == typeOfContext
}

func (f *reflectWrapper) NumResults() int {
	numResults := f.funcType.NumOut()
	if numResults > 0 && f.funcType.Out(numResults-1) == typeOfError {
		numResults--
	}
	return numResults
}

func (f *reflectWrapper) ErrorResult() bool {
	numOut := f.funcType.NumOut()
	return numOut > 0 && f.funcType.Out(numOut-1) == typeOfError
}

func (f *reflectWrapper) ArgNames() []string {
	return f.argNames
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
	out := f.funcVal.Call(in)
	resultsLen := len(out)
	if f.ErrorResult() {
		resultsLen--
		err, _ = out[len(out)-1].Interface().(error)
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
	for i, arg := range args {
		in[i+offs] = reflect.ValueOf(arg)
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
	for i := offs; i < len(in); i++ {
		argType := f.funcType.In(i)
		if i-offs >= len(strs) {
			// Pass default value if not enough strs
			in[i] = reflect.Zero(argType)
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
		return nil, NewErrParseArgsJSON(err, f, argsJSON)
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
					return nil, NewErrParseArgsJSON(err, f, argsJSON)
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
				return nil, NewErrParseArgsJSON(err, f, argsJSON)
			}
		}
		in[i] = destPtr.Elem()
	}
	return f.call(in)
}

///////////////////////////////////////////////////////////////////////////////
// Reflection helpers

// ReflectType returns the reflect.Type of the generic type T
func ReflectType[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}
