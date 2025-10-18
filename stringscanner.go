package function

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ScanString converts a string to the type pointed to by destPtr.
// It uses the globally configured StringScanners to perform type conversion.
//
// Supported conversions include:
//   - Basic types: int, float, bool, string, byte slice
//   - Time types: time.Time (multiple formats), time.Duration
//   - Pointers: "nil" or "null" converts to nil
//   - Slices/arrays: JSON-like syntax [1,2,3] or ["a","b"]
//   - Structs: JSON object syntax
//   - Custom types: encoding.TextUnmarshaler, json.Unmarshaler, or types with SetNull() method
//
// Example:
//
//	var i int
//	err := ScanString("42", &i)  // i = 42
//
//	var t time.Time
//	err := ScanString("2024-01-15", &t)
//
//	var slice []int
//	err := ScanString("[1,2,3]", &slice)  // slice = []int{1,2,3}
func ScanString(sourceStr string, destPtr any) error {
	return StringScanners.ScanString(sourceStr, destPtr)
}

// ScanStrings converts multiple strings to their respective destination types.
// It scans sourceStrings[i] into destPtrs[i] for each index.
// If the slices have different lengths, only the minimum number of elements are scanned.
// Returns an error on the first conversion failure.
//
// Example:
//
//	var a int
//	var b string
//	var c bool
//	err := ScanStrings([]string{"42", "hello", "true"}, &a, &b, &c)
func ScanStrings(sourceStrings []string, destPtrs ...any) error {
	l := len(sourceStrings)
	if ll := len(destPtrs); ll < l {
		l = ll
	}
	for i := 0; i < l; i++ {
		err := ScanString(sourceStrings[i], destPtrs[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// StringScanner defines the interface for converting strings to typed values.
// Custom implementations can be registered via TypeStringScanners to add
// support for additional types or override default conversion behavior.
type StringScanner interface {
	ScanString(sourceStr string, destPtr any) error
}

// StringScannerFunc is a function type that implements StringScanner.
// It allows standalone functions to be used as StringScanners.
type StringScannerFunc func(sourceStr string, destPtr any) error

func (f StringScannerFunc) ScanString(sourceStr string, destPtr any) error {
	return f(sourceStr, destPtr)
}

// DefaultScanString is the default string-to-type conversion function.
// It validates that destPtr is a non-nil pointer and delegates to scanString
// for the actual conversion logic.
//
// Special string values:
//   - Empty string, "nil", or "null" are treated as zero/nil values
//   - Whitespace is trimmed for most types (except string itself)
//
// Returns an error if destPtr is nil, not a pointer, or conversion fails.
func DefaultScanString(sourceStr string, destPtr any) (err error) {
	if destPtr == nil {
		return errors.New("destination pointer is nil")
	}
	destPtrVal := reflect.ValueOf(destPtr)
	if destPtrVal.Kind() != reflect.Pointer {
		return fmt.Errorf("expected destination pointer type but got: %s", destPtrVal.Type())
	}
	if destPtrVal.IsNil() {
		return errors.New("destination pointer is nil")
	}
	return scanString(sourceStr, destPtrVal.Elem())
}

// scanString is the internal implementation that converts sourceStr to destVal.
// It handles type-specific conversion logic for all supported types.
//
// The conversion strategy is:
// 1. Check for types with SetNull() method for nil values
// 2. Type switch on common pointer types (string, error, time, etc.)
// 3. Check for encoding.TextUnmarshaler interface
// 4. Check for json.Unmarshaler interface
// 5. Reflection-based conversion by Kind (bool, int, float, slice, etc.)
// 6. Fallback to fmt.Sscan for any remaining types
//
// This function is performance-critical and is called for every argument conversion.
func scanString(sourceStr string, destVal reflect.Value) (err error) {
	var (
		destPtr    = destVal.Addr().Interface()
		trimmedSrc = strings.TrimSpace(sourceStr)
		nilSrc     = trimmedSrc == "" ||
			strings.EqualFold(trimmedSrc, "nil") ||
			strings.EqualFold(trimmedSrc, "null")
	)

	if n, ok := destPtr.(interface{ SetNull() }); ok && nilSrc {
		n.SetNull()
		return nil
	}

	switch dest := destPtr.(type) {
	case *string:
		*dest = sourceStr
		return nil

	case *error:
		if nilSrc {
			*dest = nil
		} else {
			*dest = errors.New(trimmedSrc)
		}
		return nil

	case *time.Time:
		if nilSrc {
			*dest = time.Time{}
			return nil
		}
		for _, format := range TimeFormats {
			t, err := time.ParseInLocation(format, trimmedSrc, time.Local)
			if err == nil {
				*dest = t
				return nil
			}
		}
		return fmt.Errorf("can't parse %q as time.Time using formats %#v", trimmedSrc, TimeFormats)

	case interface{ Set(time.Time) }:
		if nilSrc {
			dest.Set(time.Time{})
			return nil
		}
		for _, format := range TimeFormats {
			t, err := time.ParseInLocation(format, trimmedSrc, time.Local)
			if err == nil {
				dest.Set(t)
				return nil
			}
		}
		return fmt.Errorf("can't parse %q as time.Time using formats %#v", trimmedSrc, TimeFormats)

	case *time.Duration:
		if nilSrc {
			*dest = 0
			return nil
		}
		duration, err := time.ParseDuration(trimmedSrc)
		if err != nil {
			return fmt.Errorf("can't parse %q as time.Duration because of: %w", trimmedSrc, err)
		}
		*dest = duration
		return nil

	case encoding.TextUnmarshaler:
		return dest.UnmarshalText([]byte(sourceStr))

	case json.Unmarshaler:
		source := []byte(trimmedSrc)
		if !json.Valid(source) {
			// sourceStr is not already valid JSON
			// then escape it as JSON string
			source, err = json.Marshal(sourceStr)
			if err != nil {
				return fmt.Errorf("can't marshal %q as JSON string: %w", sourceStr, err)
			}
		}
		return dest.UnmarshalJSON(source)

	case *map[string]any:
		return json.Unmarshal([]byte(trimmedSrc), destPtr)

	case *[]any:
		return json.Unmarshal([]byte(trimmedSrc), destPtr)

	case *[]byte:
		*dest = []byte(sourceStr)
		return nil
	}

	switch destVal.Kind() {
	case reflect.Pointer:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		ptr := destVal
		if ptr.IsNil() {
			ptr = reflect.New(destVal.Type().Elem())
		}
		err := scanString(sourceStr, ptr.Elem())
		if err != nil {
			return err
		}
		destVal.Set(ptr)
		return nil

	case reflect.String:
		destVal.SetString(sourceStr) // Don't trim whitespace
		return nil

	case reflect.Bool:
		if nilSrc {
			destVal.SetBool(false)
			return nil
		}
		b, err := strconv.ParseBool(trimmedSrc)
		if err != nil {
			return fmt.Errorf("can't parse %q as bool because of: %w", trimmedSrc, err)
		}
		destVal.SetBool(b)
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if nilSrc {
			destVal.SetInt(0)
			return nil
		}
		i, err := strconv.ParseInt(trimmedSrc, 10, destVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't parse %q as int because of: %w", trimmedSrc, err)
		}
		destVal.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if nilSrc {
			destVal.SetUint(0)
			return nil
		}
		u, err := strconv.ParseUint(trimmedSrc, 10, destVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't parse %q as uint because of: %w", trimmedSrc, err)
		}
		destVal.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		if nilSrc {
			destVal.SetFloat(0)
			return nil
		}
		f, err := strconv.ParseFloat(trimmedSrc, destVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't parse %q as float because of: %w", trimmedSrc, err)
		}
		destVal.SetFloat(f)
		return nil

	case reflect.Struct:
		// JSON might not be the best format for command line arguments,
		// but it could have also come from a HTTP request body or other sources
		return json.Unmarshal([]byte(trimmedSrc), destPtr)

	case reflect.Slice:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		var sourceStrings []string
		if strings.HasPrefix(trimmedSrc, "[") && strings.HasSuffix(trimmedSrc, "]") {
			sourceStrings, err = sliceLiteralFields(trimmedSrc)
			if err != nil {
				return err
			}
		} else {
			// Treat non-slice literals as single element slice
			sourceStrings = []string{trimmedSrc}
		}
		sliceLen := len(sourceStrings)
		destVal.Set(reflect.MakeSlice(destVal.Type(), sliceLen, sliceLen))
		for i := 0; i < sliceLen; i++ {
			err = scanString(sourceStrings[i], destVal.Index(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Array:
		var sourceStrings []string
		if strings.HasPrefix(trimmedSrc, "[") && strings.HasSuffix(trimmedSrc, "]") {
			sourceStrings, err = sliceLiteralFields(trimmedSrc)
			if err != nil {
				return err
			}
		} else {
			// Treat non-slice literals as single element slice
			sourceStrings = []string{trimmedSrc}
		}
		arrayLen := destVal.Len()
		if len(sourceStrings) != arrayLen {
			return fmt.Errorf("array value %q needs to have %d elements, but has %d", sourceStr, arrayLen, len(sourceStrings))
		}
		for i := range arrayLen {
			err := scanString(sourceStrings[i], destVal.Index(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Map, reflect.Chan, reflect.Func:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		return fmt.Errorf("%w: %s", ErrTypeNotSupported, destVal.Type())
	}

	// If all else fails, use fmt scanning
	// for generic type conversation from string
	_, err = fmt.Sscan(sourceStr, destPtr)
	if err != nil {
		return fmt.Errorf("%w: %s, fmt.Sscan error: %s", ErrTypeNotSupported, destVal.Type(), err)
	}

	return nil
}

// sliceLiteralFields parses a JSON-like slice literal string into individual field strings.
// It handles nested structures by tracking brace and bracket depth, and respects quoted strings.
//
// Examples:
//   - "[1,2,3]" -> ["1", "2", "3"]
//   - `["a","b"]` -> [`"a"`, `"b"`]
//   - "[[1,2],[3,4]]" -> ["[1,2]", "[3,4]"]
//   - `[{"key":"value"},null]` -> [`{"key":"value"}`, "null"]
//
// This is a complex state machine parser that carefully handles:
//   - Nested objects {...} and arrays [...]
//   - Quoted strings with escaped quotes
//   - Comma separation at the top level only
//   - Proper validation of bracket matching
//
// Returns an error if the string doesn't start with '[', end with ']',
// or has unbalanced brackets/braces.
func sliceLiteralFields(sourceStr string) (fields []string, err error) {
	if !strings.HasPrefix(sourceStr, "[") {
		return nil, fmt.Errorf("slice value %q does not begin with '['", sourceStr)
	}
	if !strings.HasSuffix(sourceStr, "]") {
		return nil, fmt.Errorf("slice value %q does not end with ']'", sourceStr)
	}
	var (
		objectDepth  = 0  // Tracks nesting depth of {} braces
		bracketDepth = 0  // Tracks nesting depth of [] brackets
		rLast        rune // Previous rune (for escape detection)
		withinQuote  rune // Non-zero when inside quotes ('"')
		begin        = 1  // Start index of current field
	)
	for i, r := range sourceStr {
		if withinQuote != 0 {
			if r == '"' && rLast != '\\' {
				withinQuote = 0
			}
			continue
		}

		switch r {
		case '{':
			objectDepth++

		case '}':
			objectDepth--
			if objectDepth < 0 {
				return nil, fmt.Errorf("slice value %q has too many '}'", sourceStr)
			}

		case '[':
			bracketDepth++

		case ']':
			bracketDepth--
			if bracketDepth < 0 {
				return nil, fmt.Errorf("slice value %q has too many ']'", sourceStr)
			}
			if objectDepth == 0 && bracketDepth == 0 && i-begin > 0 {
				fields = append(fields, strings.TrimSpace(sourceStr[begin:i]))
			}

		case ',':
			if objectDepth == 0 && bracketDepth == 1 {
				fields = append(fields, strings.TrimSpace(sourceStr[begin:i]))
				begin = i + 1
			}

		case '"':
			withinQuote = r
		}

		rLast = r
	}
	return fields, nil
}
