package function

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/domonda/go-types/nullable"
	"github.com/ungerik/go-fs"
)

// ScanString uses the configured DefaultStringScanner
// to scan sourceStr
func ScanString(sourceStr string, destPtr interface{}) error {
	return DefaultStringScanner.ScanString(sourceStr, destPtr)
}

type StringScanner interface {
	ScanString(sourceStr string, destPtr interface{}) error
}

type StringScannerFunc func(sourceStr string, destPtr interface{}) error

func (f StringScannerFunc) ScanString(sourceStr string, destPtr interface{}) error {
	return f(sourceStr, destPtr)
}

func DefaultScanString(sourceStr string, destPtr interface{}) (err error) {
	destVal := reflect.ValueOf(destPtr)
	if destVal.Kind() != reflect.Ptr {
		return fmt.Errorf("DefaultStringScannerImpl expected destination pointer type but got: %s", destVal.Type())
	}
	if destVal.IsNil() {

	}
	return defaultScanString(sourceStr, destVal.Elem())
}

func defaultScanString(sourceStr string, destVal reflect.Value) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("DefaultScanString(%s, %q): %w", sourceStr, destVal.Type(), err)
		}
	}()

	destPtr := destVal.Addr().Interface()

	switch dest := destPtr.(type) {
	case *string:
		*dest = sourceStr
		return nil

	case *time.Time:
		for _, format := range TimeFormats {
			t, err := time.ParseInLocation(format, sourceStr, time.Local)
			if err == nil {
				*dest = t
				return nil
			}
		}
		return fmt.Errorf("can't parse %q as time.Time using formats %#v", sourceStr, TimeFormats)

	case *nullable.Time:
		for _, format := range TimeFormats {
			t, err := nullable.TimeParseInLocation(format, sourceStr, time.Local)
			if err == nil {
				*dest = t
				return nil
			}
		}
		return fmt.Errorf("can't parse %q as nullable.Time using formats %#v", sourceStr, TimeFormats)

	case *time.Duration:
		duration, err := time.ParseDuration(sourceStr)
		if err != nil {
			return fmt.Errorf("can't parse %q as time.Duration because of: %w", sourceStr, err)
		}
		*dest = duration
		return nil

	case encoding.TextUnmarshaler:
		return dest.UnmarshalText([]byte(sourceStr))

	case *fs.FileReader:
		*dest = fs.File(sourceStr)
		return nil

	case json.Unmarshaler:
		return dest.UnmarshalJSON([]byte(sourceStr))

	case *map[string]interface{}:
		return json.Unmarshal([]byte(sourceStr), dest)

	case *[]interface{}:
		return json.Unmarshal([]byte(sourceStr), dest)

	case *[]byte:
		*dest = []byte(sourceStr)
		return nil
	}

	switch destVal.Kind() {
	case reflect.String:
		destVal.Set(reflect.ValueOf(sourceStr).Convert(destVal.Type()))
		return nil

	case reflect.Struct:
		// JSON might not be the best format for command line arguments,
		// but it could have also come from a HTTP request body or other sources
		return json.Unmarshal([]byte(sourceStr), destPtr)

	case reflect.Ptr:
		ptr := destVal
		if sourceStr != "nil" {
			if ptr.IsNil() {
				ptr = reflect.New(destVal.Type().Elem())
			}
			err := defaultScanString(sourceStr, ptr.Elem())
			if err != nil {
				return err
			}
			destVal.Set(ptr)
		}
		return nil

	case reflect.Slice:
		if !strings.HasPrefix(sourceStr, "[") {
			return fmt.Errorf("slice value %q does not begin with '['", sourceStr)
		}
		if !strings.HasSuffix(sourceStr, "]") {
			return fmt.Errorf("slice value %q does not end with ']'", sourceStr)
		}
		// elemSourceStrings := strings.Split(sourceStr[1:len(sourceStr)-1], ",")
		sourceFields, err := sliceLiteralFields(sourceStr)
		if err != nil {
			return err
		}

		count := len(sourceFields)
		destVal.Set(reflect.MakeSlice(destVal.Type(), count, count))

		for i := 0; i < count; i++ {
			err := defaultScanString(sourceFields[i], destVal.Index(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Array:
		if !strings.HasPrefix(sourceStr, "[") {
			return fmt.Errorf("array value %q does not begin with '['", sourceStr)
		}
		if !strings.HasSuffix(sourceStr, "]") {
			return fmt.Errorf("array value %q does not end with ']'", sourceStr)
		}
		// elemSourceStrings := strings.Split(sourceStr[1:len(sourceStr)-1], ",")
		sourceFields, err := sliceLiteralFields(sourceStr)
		if err != nil {
			return err
		}

		count := len(sourceFields)
		if count != destVal.Len() {
			return fmt.Errorf("array value %q needs to have %d elements, but has %d", sourceStr, destVal.Len(), count)
		}

		for i := 0; i < count; i++ {
			err := defaultScanString(sourceFields[i], destVal.Index(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Chan, reflect.Func:
		// We can't assign a string to a channel or function, it's OK to ignore it
		// destVal = reflect.Zero(destVal.Type()) // or set nil?
		return NewErrCantScanType(destVal.Type())
	}

	// If all else fails, use fmt scanning
	// for generic type conversation from string
	_, err = fmt.Sscan(sourceStr, destPtr)
	return err
}

func sliceLiteralFields(sourceStr string) (fields []string, err error) {
	if !strings.HasPrefix(sourceStr, "[") {
		return nil, fmt.Errorf("slice value %q does not begin with '['", sourceStr)
	}
	if !strings.HasSuffix(sourceStr, "]") {
		return nil, fmt.Errorf("slice value %q does not end with ']'", sourceStr)
	}
	objectDepth := 0
	bracketDepth := 0
	begin := 1
	for i, r := range sourceStr {
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
				fields = append(fields, sourceStr[begin:i])
			}

		case ',':
			if objectDepth == 0 && bracketDepth == 1 {
				fields = append(fields, sourceStr[begin:i])
				begin = i + 1
			}
		}
	}
	return fields, nil
}
