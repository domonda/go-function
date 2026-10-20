package function

import (
	"context"
	"reflect"
	"time"
)

var (
	// StringScanners is used by ScanString and initialized with
	// DefaultScanString as default scanner in init,
	// because DefaultScanString uses StringScanners for nested values.
	StringScanners *TypeStringScanners

	// TimeFormats used in that order to try parse time strings.
	// If a time format has not time zone part,
	// then the parsed time is interpreted in TimeLocation.
	TimeFormats = []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.DateOnly + " 15:04:05.999999999 -0700 MST", // Used by time.Time.String()
		time.DateTime,
		time.DateOnly + " 15:04",
		time.DateOnly + "T15:04:05.999999999", // Used by browser datetime-local input type with seconds
		time.DateOnly + "T15:04",              // Used by browser datetime-local input type
		time.DateOnly,
	}

	// TimeLocation is used by string-scanned time values whose source string
	// has no time zone component. Defaults to time.Local; set to time.UTC
	// (or any other *time.Location) at process start to override.
	TimeLocation = time.Local

	// MaxSliceLiteralElements limits the number of elements of a slice
	// or array literal like [1,2,3] that is scanned from a string,
	// because scanning allocates memory for every element and untrusted input
	// like an HTTP request body must not make it allocate without limit.
	// The limit applies to every slice literal, also to nested ones,
	// but not to arrays that encoding/json decodes:
	// the arrays in JSON objects of struct or map destinations,
	// and []any destinations like the slice of a ...any argument.
	// A value <= 0 disables the limit.
	MaxSliceLiteralElements = 1 << 20
)

func init() {
	StringScanners = NewTypeStringScanners(defaultStringScanner{})
}

var (
	typeOfError   = reflect.TypeFor[error]()
	typeOfContext = reflect.TypeFor[context.Context]()
	typeOfAny     = reflect.TypeFor[any]()
)
