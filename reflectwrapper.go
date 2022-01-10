package function

import (
	"context"
	"encoding/json"
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
func ReflectWrapper(function interface{}, argNames ...string) (Wrapper, error) {
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

// MustReflectWrapper calls ReflectWrapper and panics any error.
func MustReflectWrapper(function interface{}) Wrapper {
	w, err := ReflectWrapper(function)
	if err != nil {
		panic(err)
	}
	return w
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
	return f.funcVal.Type().NumIn()
}

func (f *reflectWrapper) ContextArg() bool {
	return f.funcType.NumIn() > 0 && f.funcType.In(0) == typeOfContext
}

func (f *reflectWrapper) NumResults() int {
	return f.funcVal.Type().NumOut()
}

func (f *reflectWrapper) ErrorResult() bool {
	return f.funcType.NumOut() > 0 && f.funcType.In(f.funcType.NumOut()-1) == typeOfError
}

func (f *reflectWrapper) ArgNames() []string {
	return f.argNames
}

func (f *reflectWrapper) ArgDescriptions() []string {
	return make([]string, f.NumArgs())
}

func (f *reflectWrapper) ArgTypes() []reflect.Type {
	a := make([]reflect.Type, f.funcType.NumIn())
	for i := range a {
		a[i] = f.funcType.In(i)
	}
	return a
}

func (f *reflectWrapper) ResultTypes() []reflect.Type {
	r := make([]reflect.Type, f.funcType.NumOut())
	for i := range r {
		r[i] = f.funcType.Out(i)
	}
	return r
}

func (f *reflectWrapper) call(in []reflect.Value) (results []interface{}, err error) {
	out := f.funcVal.Call(in)
	resultsLen := len(out)
	if f.ErrorResult() {
		resultsLen--
		err = out[len(out)-1].Interface().(error)
	}
	results = make([]interface{}, resultsLen)
	for i := range results {
		results[i] = out[i].Interface()
	}
	return results, err
}

func (f *reflectWrapper) Call(ctx context.Context, args []interface{}) (results []interface{}, err error) {
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

func (f *reflectWrapper) CallWithStrings(ctx context.Context, strs ...string) (results []interface{}, err error) {
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
		if argType == typeOfEmptyInterface {
			// Pass string directly for argument of type interface{}
			in[i] = reflect.ValueOf(str)
			continue
		}
		destPtr := reflect.New(argType)
		err = ScanString(str, destPtr)
		if err != nil {
			return nil, NewErrParseArgString(err, f, f.argNames[i])
		}
		in[i] = destPtr.Elem()
	}
	return f.call(in)
}

func (f *reflectWrapper) CallWithNamedStrings(ctx context.Context, strs map[string]string) (results []interface{}, err error) {
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
			if argType == typeOfEmptyInterface {
				// Pass string directly for argument of type interface{}
				in[i] = reflect.ValueOf(str)
				continue
			}
			destPtr := reflect.New(argType)
			err = ScanString(str, destPtr)
			if err != nil {
				return nil, NewErrParseArgString(err, f, f.argNames[i])
			}
			in[i] = destPtr.Elem()
		}
	}
	return f.call(in)
}

func (f *reflectWrapper) CallWithJSON(ctx context.Context, argsJSON []byte) (results []interface{}, err error) {
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
			err = json.Unmarshal(arg, destPtr.Interface())
			if err != nil {
				return nil, NewErrParseArgsJSON(err, f, argsJSON)
			}
		}
		in[i] = destPtr.Elem()
	}
	return f.call(in)
}
