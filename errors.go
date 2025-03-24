package function

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ungerik/go-httpx/httperr"
)

var (
	// ErrTypeNotSupported indicates that a type is not supported
	ErrTypeNotSupported = errors.New("type not supported")

	// HandleErrorHTTP will handle a non nil error by writing it to the response.
	// The default is to use github.com/ungerik/go-httpx/httperr.DefaultHandler.
	HandleErrorHTTP = func(err error, response http.ResponseWriter, request *http.Request) {
		if err != nil {
			httperr.DefaultHandler.HandleError(err, response, request)
		}
	}
)

type ErrParseArgString struct {
	Err      error
	Func     fmt.Stringer
	ArgName  string
	ArgValue string
}

func NewErrParseArgString(err error, f fmt.Stringer, argName, argValue string) ErrParseArgString {
	return ErrParseArgString{Err: err, Func: f, ArgName: argName, ArgValue: argValue}
}

func (e ErrParseArgString) Error() string {
	return fmt.Sprintf("can't parse argument '%s' string value '%s' as argument for function %s, error: %s", e.ArgName, e.ArgValue, e.Func, e.Err)
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
	return fmt.Sprintf("error unmarshalling JSON for argument %q of function %s: %s", e.Arg, e.Func, e.Err)
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
