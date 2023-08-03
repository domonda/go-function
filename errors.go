package function

import (
	"errors"
	"fmt"
)

var (
	ErrTypeNotSupported = errors.New("type not supported")
)

type ErrParseArgString struct {
	Err  error
	Func fmt.Stringer
	Arg  string
}

func NewErrParseArgString(err error, f fmt.Stringer, arg string) ErrParseArgString {
	return ErrParseArgString{Err: err, Func: f, Arg: arg}
}

func (e ErrParseArgString) Error() string {
	return fmt.Sprintf("string conversion error for argument %s of function %s: %s", e.Arg, e.Func, e.Err)
}

func (e ErrParseArgString) Unwrap() error {
	return e.Err
}

type ErrParseArgJSON struct {
	Err  error
	Func fmt.Stringer
	Arg  string
}

func NewErrParseArgJSON(err error, f fmt.Stringer, arg string) ErrParseArgJSON {
	return ErrParseArgJSON{Err: err, Func: f, Arg: arg}
}

func (e ErrParseArgJSON) Error() string {
	return fmt.Sprintf("error unmarshalling JSON for argument %s of function %s: %s", e.Arg, e.Func, e.Err)
}

func (e ErrParseArgJSON) Unwrap() error {
	return e.Err
}

type ErrParseArgsJSON struct {
	Err  error
	Func fmt.Stringer
	JSON string
}

func NewErrParseArgsJSON(err error, f fmt.Stringer, argsJSON []byte) ErrParseArgsJSON {
	return ErrParseArgsJSON{Err: err, Func: f, JSON: string(argsJSON)}
}

func (e ErrParseArgsJSON) Error() string {
	return fmt.Sprintf("error unmarshalling JSON object for arguments of function %s: %s", e.Func, e.Err)
}

func (e ErrParseArgsJSON) Unwrap() error {
	return e.Err
}
