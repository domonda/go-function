package function

import (
	"context"
	"encoding/json"
	"reflect"
)

func CallFunctionWithJSONArgs(ctx context.Context, f Wrapper, jsonObject []byte) (results []any, err error) {
	args, err := unmarshalJSONFunctionArgs(f, jsonObject)
	if err != nil {
		return nil, err
	}
	return f.Call(ctx, args)
}

func unmarshalJSONFunctionArgs(f Description, jsonObject []byte) (args []any, err error) {
	argsJSON := make(map[string]json.RawMessage)
	err = json.Unmarshal(jsonObject, &argsJSON)
	if err != nil {
		return nil, err
	}
	args = make([]any, f.NumArgs())
	argTypes := f.ArgTypes()
	for i, argName := range f.ArgNames() {
		argType := argTypes[i]
		if argJSON, ok := argsJSON[argName]; ok {
			ptrVal := reflect.New(argType)
			err = json.Unmarshal(argJSON, ptrVal.Interface())
			if err != nil {
				return nil, NewErrParseArgJSON(err, f, argName)
			}
			args[i] = ptrVal.Elem().Interface()
		} else {
			args[i] = reflect.Zero(argType).Interface()
		}
	}
	return args, nil
}
