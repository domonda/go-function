package function

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

type ResultsHandler interface {
	HandleResults(ctx context.Context, results []any, resultErr error) error
}

type ResultsHandlerFunc func(ctx context.Context, results []any, resultErr error) error

func (f ResultsHandlerFunc) HandleResults(ctx context.Context, results []any, resultErr error) error {
	return f(ctx, results, resultErr)
}

func makeResultsPrintable(results []any) ([]any, error) {
	for i, result := range results {
		if b, ok := result.([]byte); ok {
			results[i] = string(b)
			continue
		}

		switch derefValue(reflect.ValueOf(result)).Kind() {
		case reflect.Struct, reflect.Slice, reflect.Array:
			b, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("can't print command result as JSON because: %w", err)
			}
			results[i] = string(b)

		case reflect.Func, reflect.Chan:
			// Use Go source representation for functional types
			// that have no useful printable value
			results[i] = fmt.Sprintf("%#v", result)
		}
	}
	return results, nil
}

// PrintTo calls fmt.Fprint on writer with the result values as varidic arguments
func PrintTo(writer io.Writer) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		r, err := makeResultsPrintable(results)
		if err != nil || len(r) == 0 {
			return err
		}
		_, err = fmt.Fprint(writer, r...)
		return err
	}
}

// PrintlnTo calls fmt.Fprintln on writer for every result
func PrintlnTo(writer io.Writer) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		results, err := makeResultsPrintable(results)
		if err != nil || len(results) == 0 {
			return err
		}
		for _, r := range results {
			_, err = fmt.Fprintln(writer, r)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// Println calls fmt.Println for every result
var Println ResultsHandlerFunc = func(ctx context.Context, results []any, resultErr error) error {
	if resultErr != nil {
		return resultErr
	}
	results, err := makeResultsPrintable(results)
	if err != nil || len(results) == 0 {
		return err
	}
	for _, r := range results {
		_, err = fmt.Println(r)
		if err != nil {
			return err
		}
	}
	return nil
}

// PrintlnWithPrefixTo calls fmt.Fprintln(writer, prefix, result) for every result value
func PrintlnWithPrefixTo(prefix string, writer io.Writer) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		results, err := makeResultsPrintable(results)
		if err != nil || len(results) == 0 {
			return err
		}
		for _, result := range results {
			_, err = fmt.Fprintln(writer, prefix, result)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// PrintlnWithPrefix calls fmt.Println(prefix, result) for every result value
func PrintlnWithPrefix(prefix string) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		results, err := makeResultsPrintable(results)
		if err != nil || len(results) == 0 {
			return err
		}
		for _, result := range results {
			_, err = fmt.Println(prefix, result)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// Logger interface
type Logger interface {
	Printf(format string, args ...any)
}

// LogTo calls logger.Printf(fmt.Sprintln(results...))
func LogTo(logger Logger) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		results, err := makeResultsPrintable(results)
		if err != nil || len(results) == 0 {
			return err
		}
		logger.Printf(fmt.Sprintln(results...))
		return nil
	}
}

// LogWithPrefixTo calls logger.Printf(fmt.Sprintln(results...)) with prefix prepended to the results
func LogWithPrefixTo(prefix string, logger Logger) ResultsHandlerFunc {
	return func(ctx context.Context, results []any, resultErr error) error {
		if resultErr != nil {
			return resultErr
		}
		results, err := makeResultsPrintable(results)
		if err != nil || len(results) == 0 {
			return err
		}
		results = append([]any{prefix}, results...)
		logger.Printf(fmt.Sprintln(results...))
		return nil
	}
}

// PrintlnText prints a fixed string if a command returns without an error
type PrintlnText string

func (t PrintlnText) HandleResults(ctx context.Context, results []any, resultErr error) error {
	if resultErr != nil {
		return resultErr
	}
	_, err := fmt.Println(t)
	return err
}

// derefValue dereferences a reflect.Value until a non pointer type or nil is found
func derefValue(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}
	return v
}
