package function

import (
	"errors"
	"fmt"
)

var (
	// ErrTypeNotSupported is returned by string scanners for a destination type
	// they don't support. TypeStringScanners tries the next matching scanner
	// if a scanner returns an error wrapping ErrTypeNotSupported.
	ErrTypeNotSupported = errors.New("type not supported")
)

// ErrParseArgString is returned by CallWithStrings and CallWithNamedStrings
// if a string can't be parsed as the value of an argument.
// Err is the error of the string scanner,
// Func the wrapper of the function, usually a Description.
//
// Error doesn't contain ArgValue, because the value can be large or private,
// but the error of the string scanner usually describes it.
type ErrParseArgString struct {
	Err      error
	Func     fmt.Stringer
	ArgName  string
	ArgValue string
}

// NewErrParseArgString returns an ErrParseArgString,
// generated wrappers use it.
func NewErrParseArgString(err error, f fmt.Stringer, argName, argValue string) ErrParseArgString {
	return ErrParseArgString{Err: err, Func: f, ArgName: argName, ArgValue: argValue}
}

func (e ErrParseArgString) Error() string {
	return fmt.Sprintf("can't parse string value of argument '%s' of function %s: %s", e.ArgName, e.Func, e.Err)
}

func (e ErrParseArgString) Unwrap() error {
	return e.Err
}

// ErrParseArgJSON is returned by CallWithJSON if the JSON value
// of an argument can't be unmarshalled as the argument type.
// Err is the error of the JSON decoder,
// Func the wrapper of the function, usually a Description.
type ErrParseArgJSON struct {
	Err     error
	Func    fmt.Stringer
	ArgName string
}

// NewErrParseArgJSON returns an ErrParseArgJSON,
// generated wrappers use it.
func NewErrParseArgJSON(err error, f fmt.Stringer, argName string) ErrParseArgJSON {
	return ErrParseArgJSON{Err: err, Func: f, ArgName: argName}
}

func (e ErrParseArgJSON) Error() string {
	return fmt.Sprintf("error unmarshalling JSON for argument %q of function %s: %s", e.ArgName, e.Func, e.Err)
}

func (e ErrParseArgJSON) Unwrap() error {
	return e.Err
}

// ErrParseArgsJSON is returned by CallWithJSON if the JSON
// with the arguments is no JSON object.
// Err is the error of the JSON decoder,
// Func the wrapper of the function, usually a Description.
// The JSON isn't part of the error, because it can be large or private.
type ErrParseArgsJSON struct {
	Err  error
	Func fmt.Stringer
}

// NewErrParseArgsJSON returns an ErrParseArgsJSON,
// generated wrappers use it.
func NewErrParseArgsJSON(err error, f fmt.Stringer) ErrParseArgsJSON {
	return ErrParseArgsJSON{Err: err, Func: f}
}

func (e ErrParseArgsJSON) Error() string {
	return fmt.Sprintf("error unmarshalling JSON object for arguments of function %s: %s", e.Func, e.Err)
}

func (e ErrParseArgsJSON) Unwrap() error {
	return e.Err
}
