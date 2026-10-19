package function

import (
	"context"
	"reflect"
)

// StringArgsFunc calls a function with string arguments
// and handles its results, see NewStringArgsFunc.
type StringArgsFunc func(ctx context.Context, args ...string) error

// NewStringArgsFunc returns a StringArgsFunc that calls f.CallWithStrings
// and passes the results to resultsHandlers in order.
// It returns the error of the first handler that returns another error
// than the result error, else the result error of f,
// see ResultsHandler for details.
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

// NamedStringArgsFunc calls a function with named string arguments
// and handles its results, see NewNamedStringArgsFunc.
type NamedStringArgsFunc func(ctx context.Context, args map[string]string) error

// NewNamedStringArgsFunc returns a NamedStringArgsFunc that calls f.CallWithNamedStrings
// and passes the results to resultsHandlers like NewStringArgsFunc.
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

// JSONArgsFunc calls a function with the arguments of a JSON object
// and handles its results, see NewJSONArgsFunc.
type JSONArgsFunc func(ctx context.Context, jsonObject []byte) error

// NewJSONArgsFunc returns a JSONArgsFunc that calls f.CallWithJSON
// and passes the results to resultsHandlers like NewStringArgsFunc.
func NewJSONArgsFunc(f CallWithJSONWrapper, resultsHandlers ...ResultsHandler) JSONArgsFunc {
	return func(ctx context.Context, jsonObject []byte) error {
		results, resultErr := f.CallWithJSON(ctx, jsonObject)
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
