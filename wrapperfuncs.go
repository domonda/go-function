package function

import (
	"context"
	"reflect"
)

type StringArgsFunc func(ctx context.Context, args ...string) error

func NewStringArgsFunc(f CallWithStringsWrapper, resultsHandlers ...ResultsHandler) StringArgsFunc {
	return func(ctx context.Context, args ...string) error {
		results, resultErr := f.CallWithStrings(ctx, args...)
		for _, resultsHandler := range resultsHandlers {
			err := resultsHandler.HandleResults(ctx, results, resultErr)
			if err != nil && !sameError(err, resultErr) {
				return err
			}
		}
		return resultErr
	}
}

type NamedStringArgsFunc func(ctx context.Context, args map[string]string) error

func NewNamedStringArgsFunc(f CallWithNamedStringsWrapper, resultsHandlers ...ResultsHandler) NamedStringArgsFunc {
	return func(ctx context.Context, args map[string]string) error {
		results, resultErr := f.CallWithNamedStrings(ctx, args)
		for _, resultsHandler := range resultsHandlers {
			err := resultsHandler.HandleResults(ctx, results, resultErr)
			if err != nil && !sameError(err, resultErr) {
				return err
			}
		}
		return resultErr
	}
}

type JSONArgsFunc func(ctx context.Context, jsonObject []byte) error

func NewJSONArgsFunc(f Wrapper, resultsHandlers ...ResultsHandler) JSONArgsFunc {
	return func(ctx context.Context, jsonObject []byte) error {
		results, resultErr := CallFunctionWithJSONArgs(ctx, f, jsonObject)
		for _, resultsHandler := range resultsHandlers {
			err := resultsHandler.HandleResults(ctx, results, resultErr)
			if err != nil && !sameError(err, resultErr) {
				return err
			}
		}
		return resultErr
	}
}

// sameError reports if err is resultErr returned unchanged by a ResultsHandler.
// Comparing errors with == panics if both have the same non-comparable
// dynamic type (like a slice of errors) or hold such a value
// in a field of a comparable type, so == is only used if
// it can't panic and reflect.DeepEqual is used otherwise.
func sameError(err, resultErr error) bool {
	if err == nil || resultErr == nil {
		return err == nil && resultErr == nil
	}
	if reflect.TypeOf(err) != reflect.TypeOf(resultErr) {
		return false
	}
	if reflect.ValueOf(err).Comparable() {
		return err == resultErr
	}
	return reflect.DeepEqual(err, resultErr)
}
