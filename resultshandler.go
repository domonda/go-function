package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
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
		switch x := result.(type) {
		case fmt.GoStringer:
			results[i] = x.GoString()

		case fmt.Stringer:
			results[i] = x.String()

		case []byte:
			if utf8.Valid(x) && bytes.IndexFunc(x, func(r rune) bool { return !unicode.IsPrint(r) }) == -1 {
				results[i] = string(x)
			} else {
				results[i] = fmt.Sprintf("%#x", x)
			}

		default:
			switch v := derefValue(reflect.ValueOf(result)); v.Kind() {
			case reflect.Float32, reflect.Float64:
				// Print with up to 12 decimals precission
				results[i] = strings.TrimRight(fmt.Sprintf("%.12f", v.Interface()), "0")

			case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
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
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	return v
}

// PrintStructSliceAsTable prints a slice of structs or struct pointers as a padded table
// to os.Stdout using fmt.Sprint to format field values.
var PrintStructSliceAsTable ResultsHandlerFunc = func(ctx context.Context, results []any, resultErr error) error {
	if resultErr != nil {
		return resultErr
	}
	if len(results) == 0 {
		return nil
	}
	// Check if all results are slices of the same type
	resultType := reflect.TypeOf(results[0])
	if resultType.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %T", results[0])
	}
	if resultType.Elem().Kind() != reflect.Struct && resultType.Elem().Kind() != reflect.Ptr {
		return fmt.Errorf("expected slice of structs or struct pointers, got %T", results[0])
	}
	isPtr := resultType.Elem().Kind() == reflect.Ptr
	structType := resultType.Elem()
	if isPtr {
		structType = structType.Elem()
	}
	var header []string
	for col := 0; col < structType.NumField(); col++ {
		header = append(header, structType.Field(col).Name)
	}
	rows := [][]string{header}

	sliceVal := reflect.ValueOf(results[0])
	for row := 0; row < sliceVal.Len(); row++ {
		structVal := sliceVal.Index(row)
		if isPtr {
			if structVal.IsNil() {
				continue
			}
			structVal = structVal.Elem()
		}
		fieldStrings := make([]string, structType.NumField())
		for col := 0; col < structType.NumField(); col++ {
			fieldStrings[col] = fmt.Sprint(structVal.Field(col).Interface())
		}
		rows = append(rows, fieldStrings)
	}

	colWidths := make([]int, structType.NumField())
	for row := range rows {
		for col := 0; col < structType.NumField() && col < len(rows[row]); col++ {
			numRunes := utf8.RuneCountInString(rows[row][col])
			colWidths[col] = max(colWidths[col], numRunes)
		}
	}
	w := os.Stdout
	var err error
	for _, rowStrs := range rows {
		for col, colWidth := range colWidths {
			switch {
			case col == 0:
				_, err = w.Write([]byte("| "))
			case col < len(colWidths):
				_, err = w.Write([]byte(" | "))
			}
			if err != nil {
				return err
			}
			str := ""
			if col < len(rowStrs) {
				str = rowStrs[col]
			}
			_, err = io.WriteString(w, str)
			if err != nil {
				return err
			}
			strLen := utf8.RuneCountInString(str)
			for i := strLen; i < colWidth; i++ {
				_, err = w.Write([]byte{' '})
				if err != nil {
					return err
				}
			}
		}
		_, err = w.Write([]byte(" |\n"))
		if err != nil {
			return err
		}
	}
	return nil
}
