package function

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
)

// CallFunctionWithJSONArgs calls f with the arguments unmarshalled from
// jsonObject like CallWithJSONWrapper.CallWithJSON: a JSON object with the
// argument names as keys, missing arguments are passed as zero values,
// and an error argument is a JSON string where an empty string or null is no error.
// A context.Context argument gets ctx.
//
// Security: the JSON values become argument values as they are, so don't pass
// untrusted input for arguments of types that name files or other resources,
// like fs.File of github.com/ungerik/go-fs which is a path on the server.
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
		return nil, NewErrParseArgsJSON(err, f, jsonObject)
	}
	// The context.Context argument is not part of args
	// because it is passed separately to Wrapper.Call
	offs := 0
	if f.ContextArg() {
		offs = 1
	}
	args = make([]any, f.NumArgs()-offs)
	argTypes := f.ArgTypes()
	for i, argName := range f.ArgNames()[offs:] {
		argType := argTypes[i+offs]
		if argJSON, ok := argsJSON[argName]; ok {
			if argType == typeOfError {
				// json.Unmarshal does not work for errors
				// so like ReflectWrapper.CallWithJSON unmarshal
				// a string and create an error from it if not empty
				var errStr string
				err = json.Unmarshal(argJSON, &errStr)
				if err != nil {
					return nil, NewErrParseArgJSON(err, f, argName)
				}
				if errStr != "" {
					args[i] = errors.New(errStr)
				}
				continue
			}
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
