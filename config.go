package function

import (
	"context"
	"reflect"
	"time"
)

var (
	StringScanners = NewTypeStringScanners(StringScannerFunc(DefaultScanString))

	// TimeFormats used in that order to try parse time strings.
	// If a time format has not time zone part,
	// then the parsed time is interpreted in TimeLocation.
	TimeFormats = []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.DateOnly + " 15:04:05.999999999 -0700 MST", // Used by time.Time.String()
		time.DateTime,
		time.DateOnly + " 15:04",
		time.DateOnly + "T15:04", // Used by browser datetime-local input type
		time.DateOnly,
	}

	// TimeLocation is used by string-scanned time values whose source string
	// has no time zone component. Defaults to time.Local; set to time.UTC
	// (or any other *time.Location) at process start to override.
	TimeLocation = time.Local
)

var (
	typeOfError   = reflect.TypeFor[error]()
	typeOfContext = reflect.TypeFor[context.Context]()
	typeOfAny     = reflect.TypeFor[any]()
)
