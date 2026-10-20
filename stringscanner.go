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
	"unicode/utf8"
)

// ScanString converts a string to the type pointed to by destPtr.
// It uses the globally configured StringScanners to perform type conversion.
//
// Supported conversions include:
//   - Basic types: int, float, bool, string, byte slice
//   - Time types: time.Time (multiple formats), time.Duration,
//     types with a Set(time.Time) method like nullable.Time of github.com/domonda/go-types
//     get the time parsed like time.Time
//   - Pointers: "nil" or "null" converts to nil
//   - Slices/arrays: JSON-like syntax [1,2,3] or ["a","b"]
//     with at most MaxSliceLiteralElements elements
//   - Structs: JSON object syntax
//   - Custom types: encoding.TextUnmarshaler gets the unmodified string,
//     json.Unmarshaler the string if it is valid JSON, else the string encoded as JSON string,
//     types with a SetNull() method get it called for "", "nil", or "null"
//     (see DefaultScanString)
//
// Scanners registered in StringScanners also apply to slice and array
// elements and pointer targets scanned by DefaultScanString.
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
// Returns an error without scanning if the numbers of sourceStrings
// and destPtrs differ, or the error of the first conversion failure.
//
// Example:
//
//	var a int
//	var b string
//	var c bool
//	err := ScanStrings([]string{"42", "hello", "true"}, &a, &b, &c)
func ScanStrings(sourceStrings []string, destPtrs ...any) error {
	if len(sourceStrings) != len(destPtrs) {
		return fmt.Errorf("can't scan %d strings into %d destinations", len(sourceStrings), len(destPtrs))
	}
	for i, sourceStr := range sourceStrings {
		err := ScanString(sourceStr, destPtrs[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// StringScanner defines the interface for converting strings to typed values.
// Custom implementations can be registered via TypeStringScanners to add
// support for additional types or override default conversion behavior.
//
// Security: a scanned string becomes the value of the destination as it is,
// so a string scanned into a type that names a file or other resource,
// like fs.File of github.com/ungerik/go-fs, becomes a path on the server.
// Don't scan untrusted input into such types.
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
//     (except for string kinds and []byte, which keep "nil" and "null" as text,
//     and encoding.TextUnmarshaler and json.Unmarshaler)
//   - Whitespace is trimmed for most types
//     (except string, []byte, and encoding.TextUnmarshaler)
//
// A type with a SetNull() method gets it called for an empty string,
// "nil", or "null" before any other conversion.
//
// A type with a Set(time.Time) method gets the string parsed using TimeFormats
// and TimeLocation like time.Time and passed to Set, or the zero time.Time
// for an empty string, "nil", or "null", also if it implements
// encoding.TextUnmarshaler or json.Unmarshaler, so that a nullable time type
// accepts the same formats as time.Time.
//
// Destinations implementing encoding.TextUnmarshaler (except time.Time
// and types with a Set(time.Time) method, which are parsed using TimeFormats)
// get the unmodified string passed
// to UnmarshalText for all input, including whitespace, "", "nil", and "null",
// so the type owns its parsing and validation.
// Destinations implementing json.Unmarshaler get the string passed
// to UnmarshalJSON if it is valid JSON like null, 123, or "text",
// else the unmodified string encoded as JSON string.
// If valid non-string JSON is rejected, it is retried as JSON string.
//
// Elements of a slice literal that are JSON strings like in ["a","b"]
// are unquoted and explicit string values, so "", "nil", and "null"
// are not treated as zero/nil values for them: [1, "", 3] and [1, "null"]
// can't be scanned as []int, ["", "null"] is scanned as []*string
// with pointers to "" and "null", and as []string{"", "null"}.
// Unquoted elements like in [1, null, 3] keep being treated as zero/nil values.
// Quoted elements are also passed as JSON strings to destinations decoding JSON,
// so ["123"] scanned as []json.RawMessage or []any keeps the string "123".
//
// Slice and array elements and pointer targets are scanned with the
// package-level StringScanners, also if DefaultScanString is called directly
// or by another TypeStringScanners, so that scanners registered there apply
// to them like to the top-level destination. Values without a matching
// registered scanner are scanned by the default scanner of StringScanners,
// which is DefaultScanString if not changed, except quoted elements:
// only DefaultScanString knows that they are explicit strings,
// so it scans them instead of the default scanner of StringScanners.
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
	return scanString(sourceStr, destPtrVal.Elem(), false)
}

// scanString is the internal implementation that converts sourceStr to destVal.
// It handles type-specific conversion logic for all supported types.
//
// The conversion strategy is:
//  1. Check for types with SetNull() method for nil values
//  2. Type switch on common pointer types (string, error, time, etc.)
//     and types with a Set(time.Time) method
//  3. Check for encoding.TextUnmarshaler interface
//  4. Check for json.Unmarshaler interface
//  5. Reflection-based conversion by Kind (bool, int, float, slice, etc.)
//  6. Fallback to fmt.Sscan for any remaining types
//
// Slice and array elements and pointer targets are scanned
// with scanNestedString, so that registered scanners apply to them.
//
// If explicitStr is true, then sourceStr is an explicit string value
// like a quoted element of a slice literal, so "", "nil", and "null"
// are scanned as string instead of being interpreted as nil/zero value.
//
// This function is performance-critical and is called for every argument conversion.
func scanString(sourceStr string, destVal reflect.Value, explicitStr bool) (err error) {
	var (
		destPtr    = destVal.Addr().Interface()
		trimmedSrc = strings.TrimSpace(sourceStr)
		nilSrc     = !explicitStr && isNilSource(trimmedSrc)
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
		// Like for the error arguments of CallWithJSON,
		// an empty error message is no error, also for an explicit string
		if nilSrc || trimmedSrc == "" {
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
			t, err := time.ParseInLocation(format, trimmedSrc, TimeLocation)
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
			t, err := time.ParseInLocation(format, trimmedSrc, TimeLocation)
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
			// The error of time.ParseDuration quotes the string
			return fmt.Errorf("can't parse time.Duration: %w", err)
		}
		*dest = duration
		return nil

	case encoding.TextUnmarshaler:
		// The type owns its parsing and validation,
		// so pass the unmodified string for all input
		return dest.UnmarshalText([]byte(sourceStr))

	case json.Unmarshaler:
		source := []byte(trimmedSrc)
		if explicitStr || !json.Valid(source) {
			// sourceStr is an explicit string value
			// or not already valid JSON,
			// then escape it as JSON string
			source, err = json.Marshal(sourceStr)
			if err != nil {
				return fmt.Errorf("can't marshal %q as JSON string: %w", sourceStr, err)
			}
			return dest.UnmarshalJSON(source)
		}
		err = dest.UnmarshalJSON(source)
		if err == nil || source[0] == '"' {
			return err
		}
		// sourceStr is valid JSON but not a JSON string like 123 or true,
		// which a string valued type can't unmarshal,
		// so retry with sourceStr escaped as JSON string
		jsonStr, marshalErr := json.Marshal(sourceStr)
		if marshalErr != nil {
			return err
		}
		retryErr := dest.UnmarshalJSON(jsonStr)
		if retryErr != nil {
			return err // Return the error of the original JSON
		}
		return nil

	case *map[string]any:
		if nilSrc {
			*dest = nil
			return nil
		}
		return unmarshalTrimmedJSON(sourceStr, trimmedSrc, explicitStr, destPtr)

	case *[]any:
		if nilSrc {
			*dest = nil
			return nil
		}
		return unmarshalTrimmedJSON(sourceStr, trimmedSrc, explicitStr, destPtr)

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
		err := scanNestedString(sourceStr, ptr.Elem(), explicitStr)
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
			return fmt.Errorf("can't parse %q as bool because of: %w", trimmedSrc, numErrorReason(err))
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
			return fmt.Errorf("can't parse %q as int because of: %w", trimmedSrc, numErrorReason(err))
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
			return fmt.Errorf("can't parse %q as uint because of: %w", trimmedSrc, numErrorReason(err))
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
			return fmt.Errorf("can't parse %q as float because of: %w", trimmedSrc, numErrorReason(err))
		}
		destVal.SetFloat(f)
		return nil

	case reflect.Struct:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		// JSON might not be the best format for command line arguments,
		// but it could have also come from a HTTP request body or other sources
		return unmarshalTrimmedJSON(sourceStr, trimmedSrc, explicitStr, destPtr)

	case reflect.Slice:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		elems, explicit, err := parseSliceLiteralOrSingle(trimmedSrc, explicitStr)
		if err != nil {
			return err
		}
		destVal.Set(reflect.MakeSlice(destVal.Type(), len(elems), len(elems)))
		for i, elem := range elems {
			err = scanNestedString(elem, destVal.Index(i), explicit[i])
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Array:
		if nilSrc {
			destVal.SetZero()
			return nil
		}
		elems, explicit, err := parseSliceLiteralOrSingle(trimmedSrc, explicitStr)
		if err != nil {
			return err
		}
		arrayLen := destVal.Len()
		if len(elems) != arrayLen {
			return fmt.Errorf("array value %q needs to have %d elements, but has %d", sourceStr, arrayLen, len(elems))
		}
		for i, elem := range elems {
			err := scanNestedString(elem, destVal.Index(i), explicit[i])
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

// numErrorReason returns the reason like strconv.ErrSyntax
// of a *strconv.NumError, which also contains the parsed string,
// so that errors quoting the string don't repeat it, else err.
func numErrorReason(err error) error {
	if numErr, ok := errors.AsType[*strconv.NumError](err); ok {
		return numErr.Err
	}
	return err
}

// isNilSource returns true if the trimmed source string
// is interpreted as nil/zero value: "", "nil", or "null".
func isNilSource(trimmedSrc string) bool {
	return trimmedSrc == "" ||
		strings.EqualFold(trimmedSrc, "nil") ||
		strings.EqualFold(trimmedSrc, "null")
}

// unmarshalTrimmedJSON unmarshals trimmedSrc as JSON into destPtr.
// An explicit string value is unmarshalled as JSON string,
// so it stays a string for destinations like []any
// and is rejected like json.Unmarshal rejects a JSON string
// for a destination that is not a string.
func unmarshalTrimmedJSON(sourceStr, trimmedSrc string, explicitStr bool, destPtr any) error {
	if explicitStr {
		source, err := json.Marshal(sourceStr)
		if err != nil {
			return fmt.Errorf("can't marshal %q as JSON string: %w", sourceStr, err)
		}
		return json.Unmarshal(source, destPtr)
	}
	return json.Unmarshal([]byte(trimmedSrc), destPtr)
}

// scanNestedString scans sourceStr into destVal, which is a slice or array
// element or the target of a pointer, with the package-level StringScanners
// instead of recursing into scanString, so that scanners registered for
// a type, interface, or kind apply at every level of a destination type.
// Like a direct recursion into scanString, this only scans a part
// of the destination and never the same destination again.
// Without StringScanners it recurses into scanString.
//
// An explicitStr is scanned with the type, interface, and kind scanners
// of StringScanners, and else as explicit string by scanString
// instead of the default scanner of StringScanners.
func scanNestedString(sourceStr string, destVal reflect.Value, explicitStr bool) error {
	scanners := StringScanners
	if scanners == nil {
		return scanString(sourceStr, destVal, explicitStr)
	}
	destPtr := destVal.Addr().Interface()
	if explicitStr {
		// Registered scanners get the explicit string first,
		// the default scanner keeps it a string value
		explicitScanners := *scanners
		explicitScanners.defaultScanner = StringScannerFunc(scanExplicitString)
		return explicitScanners.ScanString(sourceStr, destPtr)
	}
	return scanners.ScanString(sourceStr, destPtr)
}

// scanExplicitString is DefaultScanString for an explicit string value
// that is not interpreted as nil/zero value. It is only used as default
// scanner of TypeStringScanners, which validates destPtr before.
func scanExplicitString(sourceStr string, destPtr any) error {
	return scanString(sourceStr, reflect.ValueOf(destPtr).Elem(), true)
}

// parseSliceLiteralOrSingle interprets sourceStr as either a JSON-like slice
// literal `[a,b,...]` (split via sliceLiteralFields) or as a single-element
// slice value when it has neither `[` prefix nor `]` suffix. Mixed forms like
// `[1,2` or `1,2]` are rejected as malformed instead of silently treated as a
// single element — that fallthrough used to surface as misleading downstream
// "can't parse" errors when callers truncated input.
// Elements of a slice literal that are valid JSON strings are unquoted,
// so `["a","b"]` returns the same elements as `[a,b]`,
// and are explicit string values.
// The single element of a value without brackets is explicit if explicitStr is true.
// explicit reports for every element if it is an explicit string value,
// see the explicitStr argument of scanString.
func parseSliceLiteralOrSingle(sourceStr string, explicitStr bool) (elems []string, explicit []bool, err error) {
	hasPrefix := strings.HasPrefix(sourceStr, "[")
	hasSuffix := strings.HasSuffix(sourceStr, "]")
	switch {
	case hasPrefix && hasSuffix:
		elems, err = sliceLiteralFields(sourceStr)
		if err != nil {
			return nil, nil, err
		}
		// Unquote in place, because large literals of request bodies
		// would need a lot of memory for a copy of the elements
		explicit = make([]bool, len(elems))
		for i, field := range elems {
			elems[i], explicit[i] = unquoteSliceElement(field)
		}
		return elems, explicit, nil
	case hasPrefix != hasSuffix:
		return nil, nil, fmt.Errorf("malformed slice literal %q: must be wrapped in matching '[' and ']'", sourceStr)
	default:
		return []string{sourceStr}, []bool{explicitStr}, nil
	}
}

// unquoteSliceElement returns the unquoted string and true
// if field is a valid JSON string, else field and false.
func unquoteSliceElement(field string) (str string, ok bool) {
	if len(field) < 2 || field[0] != '"' || field[len(field)-1] != '"' {
		return field, false
	}
	// Fast path without json.Unmarshal: the unquoted string of a JSON string
	// without escapes, quotes, control characters, and invalid UTF-8
	// (which json.Unmarshal replaces with U+FFFD) is the string between the quotes
	inner := field[1 : len(field)-1]
	simple := true
	for i := 0; i < len(inner); i++ {
		if c := inner[i]; c == '\\' || c == '"' || c < 0x20 {
			simple = false
			break
		}
	}
	if simple && utf8.ValidString(inner) {
		return inner, true
	}
	// Not the named result, which would escape to the heap for every call
	var unquoted string
	err := json.Unmarshal([]byte(field), &unquoted)
	if err != nil {
		return field, false
	}
	return unquoted, true
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
//   - Quoted strings with escaped quotes (`\"`) and escaped backslashes (`\\`)
//   - Comma separation at the top level only
//   - Proper validation of bracket matching
//
// Returns an error if the string doesn't start with '[', end with ']',
// has content after the closing ']', has an empty element,
// or has unbalanced brackets/braces/quotes.
// Brackets containing only whitespace like "[ ]" return no fields like "[]".
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
		withinQuote  bool // True while inside a "..." string
		escaped      bool // True when the next rune is the escaped char (consumed without interpretation)
		begin        = 1  // Start index of current field
	)
	for i, r := range sourceStr {
		if withinQuote {
			// Track JSON-style escapes so that `\"` does not terminate the string
			// and `\\` does not leave a stray backslash that could fool the next char.
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				withinQuote = false
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
			if bracketDepth == 0 {
				// Closing bracket of the slice literal must be the last character
				if i != len(sourceStr)-1 {
					return nil, fmt.Errorf("slice value %q has content after closing ']'", sourceStr)
				}
				if objectDepth == 0 {
					field := strings.TrimSpace(sourceStr[begin:i])
					switch {
					case field != "":
						fields = append(fields, field)
					case len(fields) > 0:
						// Element after last comma is empty
						return nil, fmt.Errorf("slice value %q has an empty element", sourceStr)
					}
					// else only whitespace between the brackets: empty slice
				}
			}

		case ',':
			if objectDepth == 0 && bracketDepth == 1 {
				field := strings.TrimSpace(sourceStr[begin:i])
				if field == "" {
					return nil, fmt.Errorf("slice value %q has an empty element", sourceStr)
				}
				fields = append(fields, field)
				// Another element follows the comma, so stop parsing
				// before allocating memory for more elements than the limit
				if MaxSliceLiteralElements > 0 && len(fields) >= MaxSliceLiteralElements {
					return nil, fmt.Errorf("slice value has more than %d elements (MaxSliceLiteralElements)", MaxSliceLiteralElements)
				}
				begin = i + 1
			}

		case '"':
			withinQuote = true
		}
	}
	if withinQuote {
		return nil, fmt.Errorf("slice value %q has unterminated string literal", sourceStr)
	}
	if objectDepth != 0 {
		return nil, fmt.Errorf("slice value %q has unbalanced '{}'", sourceStr)
	}
	if bracketDepth != 0 {
		return nil, fmt.Errorf("slice value %q has unbalanced '[]'", sourceStr)
	}
	return fields, nil
}
