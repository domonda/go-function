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

// ScanString uses the configured DefaultStringScanner
// to scan sourceStr to destPtr.
func ScanString(sourceStr string, destPtr any) error {
	return StringScanners.ScanString(sourceStr, destPtr)
}

// ScanStrings uses the configured DefaultStringScanner
// to scan sourceStrings to destPtrs.
// If the number of sourceStrings and destPtrs is not identical
// then only the lower number of either will be scanned.
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

type StringScanner interface {
	ScanString(sourceStr string, destPtr any) error
}

type StringScannerFunc func(sourceStr string, destPtr any) error

func (f StringScannerFunc) ScanString(sourceStr string, destPtr any) error {
	return f(sourceStr, destPtr)
}

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
		for i := 0; i < arrayLen; i++ {
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

func sliceLiteralFields(sourceStr string) (fields []string, err error) {
	if !strings.HasPrefix(sourceStr, "[") {
		return nil, fmt.Errorf("slice value %q does not begin with '['", sourceStr)
	}
	if !strings.HasSuffix(sourceStr, "]") {
		return nil, fmt.Errorf("slice value %q does not end with ']'", sourceStr)
	}
	var (
		objectDepth  = 0
		bracketDepth = 0
		rLast        rune
		withinQuote  rune
		begin        = 1
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
