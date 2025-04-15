package function

import (
	"context"
	"reflect"
	"time"
)

var (
	CatchHTTPHandlerPanics = true
	PrettyPrint            = true
	PrettyPrintIndent      = "  "
)

var (
	StringScanners = NewTypeStringScanners(StringScannerFunc(DefaultScanString))

	ArgNameTag        = "arg"
	ArgDescriptionTag = "desc"

	// TimeFormats used in that order to try parse time strings.
	// If a time format has not time zone part,
	// then the date is returned in the local time zone.
	TimeFormats = []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.DateOnly + " 15:04:05.999999999 -0700 MST", // Used by time.Time.String()
		time.DateTime,
		time.DateOnly + " 15:04",
		time.DateOnly + "T15:04", // Used by browser datetime-local input type
		time.DateOnly,
	}
)

var (
	typeOfError   = reflect.TypeFor[error]()
	typeOfContext = reflect.TypeFor[context.Context]()
	typeOfAny     = reflect.TypeFor[any]()
)
