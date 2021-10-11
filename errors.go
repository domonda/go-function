package function

import (
	"errors"
	"fmt"
)

var (
	ErrCommandNotFound  = errors.New("command not found")
	ErrTypeNotSupported = errors.New("type not supported")
)

type ErrParseArgString struct {
	Err  error
	Func Description
	Arg  string
}

func NewErrParseArgString(err error, f Description, arg string) ErrParseArgString {
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
	Func Description
	Arg  string
}

func NewErrParseArgJSON(err error, f Description, arg string) ErrParseArgJSON {
	return ErrParseArgJSON{Err: err, Func: f, Arg: arg}
}

func (e ErrParseArgJSON) Error() string {
	return fmt.Sprintf("error unmarshalling JSON for argument %s of function %s: %s", e.Arg, e.Func, e.Err)
}

func (e ErrParseArgJSON) Unwrap() error {
	return e.Err
}
