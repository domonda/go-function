package httpfun

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"

	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
)

// ErrorHandler writes the response for an error of a Handler.
// HandleError returns true if it handled the error by writing a response,
// or false to pass the error on to the next error handler,
// like an ErrorHandler that only logs errors.
// An ErrorHandler that wrote to the response must return true,
// because the next error handler would write another response.
//
// The method is the one of the Handler interface of
// github.com/ungerik/go-httpx/httperr, so its handlers
// like httperr.DefaultHandler can be used as ErrorHandler.
type ErrorHandler interface {
	HandleError(err error, response http.ResponseWriter, request *http.Request) (handled bool)
}

// ErrorHandlerFunc is a function type that implements ErrorHandler.
type ErrorHandlerFunc func(err error, response http.ResponseWriter, request *http.Request) (handled bool)

func (f ErrorHandlerFunc) HandleError(err error, response http.ResponseWriter, request *http.Request) (handled bool) {
	return f(err, response, request)
}

// Handler creates an http.HandlerFunc from a wrapped function.
//
// Parameters:
//   - getArgs: Extracts function arguments from the HTTP request (can be nil for no args)
//   - function: The wrapped function to execute
//   - resultsWriter: Formats and writes results to the response (can be nil for no response)
//   - errHandlers: Optional error handlers tried in order before the global HandleError
//
// The handler:
//   - Recovers from panics if CatchHandlerPanics is true
//   - Parses arguments from the request using getArgs
//   - Executes the wrapped function with request.Context()
//   - Writes results using resultsWriter
//   - Passes errors to errHandlers in order until one handles them,
//     and to the global HandleError if none does
//
// Errors of getArgs, like of an unreadable request body, and argument values
// that the wrapped function can't parse are errors of the request,
// passed to the error handlers wrapped so that DefaultHandleError answers them with 400 Bad Request,
// or 413 Request Entity Too Large if the request body exceeded a limit
// like MaxRequestBodySize or MaxMultipartFormSize, without calling resultsWriter.
// Errors of the file system, like a failed temporary file of an upload,
// and a missing environment variable of RequestArgFromEnvVar are errors of the server.
// A RequestArgsFunc can return an error that implements http.Handler,
// like the errors of httperr, to respond with another status.
// Temporary files of uploads parsed by getArgs are removed after the response.
//
// An invalid argument value is recognized by the Func of its
// function.ErrParseArgString, ErrParseArgJSON, or ErrParseArgsJSON,
// which has to be the passed wrapper, the value it points to,
// or a wrapper embedded in it, like by a decorating struct.
// So a parse error of another wrapper that the wrapped function called
// and returned is an error of the server, also if it was returned unwrapped.
// A wrapper behind a function adapter like function.CallWithNamedStringsWrapperFunc,
// in a named struct field, embedded more than maxWrapperEmbeddingDepth levels deep,
// or a value of a type that can't be compared with ==, like a struct with a slice field,
// can't be recognized, so its invalid argument values are answered as errors
// of the server: embed the wrapper or pass a pointer to keep 400 Bad Request.
//
// Example:
//
//	func Calculate(ctx context.Context, a, b int) (int, error) {
//	    return a + b, nil
//	}
//
//	handler := httpfun.Handler(
//	    httpfun.RequestQueryArgs,
//	    function.MustReflectWrapper(Calculate, "ctx", "a", "b"),
//	    httpfun.RespondJSON,
//	)
//	http.Handle("/calculate", handler)
//
// Usage: GET /calculate?a=5&b=3 returns: 8
func Handler(getArgs RequestArgsFunc, function function.CallWithNamedStringsWrapper, resultsWriter ResultsWriter, errHandlers ...ErrorHandler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if CatchHandlerPanics {
			defer handlePanic(errHandlers, response, request)
		}

		if request.MultipartForm == nil {
			// Remove the temporary files of uploads that getArgs buffered
			// after the response, because net/http doesn't remove them
			// for requests cloned by middleware
			defer func() {
				if request.MultipartForm != nil {
					_ = request.MultipartForm.RemoveAll()
				}
			}()
		}

		var args map[string]string
		if getArgs != nil {
			a, err := getArgs(request)
			if err != nil {
				handleError(argsRequestError(err), errHandlers, response, request)
				return
			}
			args = a
		}

		results, err := function.CallWithNamedStrings(request.Context(), args)
		if reqErr, ok := argParseRequestError(err, function); ok {
			// The function was not called because the request
			// contained an invalid argument value
			handleError(reqErr, errHandlers, response, request)
			return
		}
		if resultsWriter != nil {
			err = resultsWriter.WriteResults(results, err, response, request)
		}
		if err != nil {
			// If this is an error from resultsWriter.WriteResults
			// then we don't know if the http.ResponseWriter already
			// was written to, but better to err on the side
			// of always writing the error even if it collides
			// with some buffered response content.
			handleError(err, errHandlers, response, request)
		}
	}
}

// HandlerNoWrapper creates an http.HandlerFunc from a simple function without using wrappers.
// The function must have the signature: func(context.Context) ([]byte, error)
//
// This is useful for simple handlers that don't need argument parsing,
// or when you want to handle the request directly.
//
// Parameters:
//   - function: Function returning response bytes
//   - resultsWriter: Formats and writes the bytes to the response
//   - errHandlers: Optional error handlers tried in order before the global HandleError
//
// Example:
//
//	func GetStatus(ctx context.Context) ([]byte, error) {
//	    return []byte(`{"status":"ok"}`), nil
//	}
//
//	http.Handle("/status", httpfun.HandlerNoWrapper(
//	    GetStatus,
//	    httpfun.RespondContentType("application/json"),
//	))
func HandlerNoWrapper(function func(context.Context) ([]byte, error), resultsWriter ResultsWriter, errHandlers ...ErrorHandler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if CatchHandlerPanics {
			defer handlePanic(errHandlers, response, request)
		}

		result, err := function(request.Context())
		if resultsWriter != nil {
			err = resultsWriter.WriteResults([]any{result}, err, response, request)
		}
		if err != nil {
			// If this is an error from resultsWriter.WriteResults
			// then we don't know if the http.ResponseWriter already
			// was written to, but better to err on the side
			// of always writing the error even if it collides
			// with some buffered response content.
			handleError(err, errHandlers, response, request)
		}
	}
}

// handlePanic is deferred by the handlers to recover a panic
// and pass it as error to handleError, except http.ErrAbortHandler,
// with which a handler aborts the response, so it is panicked again for net/http.
func handlePanic(errHandlers []ErrorHandler, response http.ResponseWriter, request *http.Request) {
	p := recover()
	if p == nil {
		return
	}
	if p == http.ErrAbortHandler {
		panic(p)
	}
	handleError(httperr.AsError(p), errHandlers, response, request)
}

// handleError passes err to errHandlers in order until one handles it,
// so that only one of them writes the response,
// and to the global HandleError if none of them handles it,
// so that an error is never answered with an empty 200 OK response.
func handleError(err error, errHandlers []ErrorHandler, response http.ResponseWriter, request *http.Request) {
	if err == nil {
		return
	}
	for _, errHandler := range errHandlers {
		if errHandler.HandleError(err, response, request) {
			return
		}
	}
	HandleError(err, response, request)
}

// requestError is an error caused by the request, not by the server,
// like an invalid argument value or an unreadable request body.
// It wraps the original error for logging and responds
// with its status code and a message without details of the server.
type requestError struct {
	err        error
	statusCode int
	message    string
}

func (e *requestError) Error() string { return e.err.Error() }

func (e *requestError) Unwrap() error { return e.err }

// ServeHTTP responds with the status code and message of the error,
// httperr.DefaultHandler uses it for errors wrapping a requestError.
func (e *requestError) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	http.Error(response, e.message, e.statusCode)
}

// argsRequestError returns an error of a RequestArgsFunc as requestError,
// except for errors of the server like a missing environment variable
// or a failed temporary file of an upload.
func argsRequestError(err error) error {
	var errResponse http.Handler
	if errors.As(err, &errResponse) {
		// The error already carries its response, like httperr.Unauthorized,
		// also if it is joined with an error of the server
		return err
	}
	if errors.Is(err, errEnvVarNotSet) {
		return err
	}
	if _, ok := errors.AsType[*os.PathError](err); ok {
		// Not wrapped because httperr answers a wrapped fs.ErrNotExist,
		// like of a missing temporary directory, with 404 Not Found
		return fmt.Errorf("can't get arguments of request because of file system error: %s", err)
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok || errors.Is(err, multipart.ErrMessageTooLarge) {
		// The body exceeded a limit like MaxRequestBodySize or MaxMultipartFormSize,
		// or the form fields that aren't files exceeded the memory
		// that http.Request.ParseMultipartForm allows for them
		return &requestError{err: err, statusCode: http.StatusRequestEntityTooLarge, message: http.StatusText(http.StatusRequestEntityTooLarge)}
	}
	return &requestError{err: err, statusCode: http.StatusBadRequest, message: http.StatusText(http.StatusBadRequest)}
}

// argParseRequestError returns err as requestError
// if it is an error from parsing the arguments of wrapper,
// so the request contained an invalid argument value.
// The message names the argument but not its value.
// Errors from parsing arguments of other functions
// that the wrapped function called are errors of the server.
func argParseRequestError(err error, wrapper any) (*requestError, bool) {
	if err == nil {
		return nil, false
	}
	if e, ok := errors.AsType[function.ErrParseArgString](err); ok && isWrapperOf(e.Func, wrapper) {
		return &requestError{err: err, statusCode: http.StatusBadRequest, message: fmt.Sprintf("invalid value for argument %q", e.ArgName)}, true
	}
	if e, ok := errors.AsType[function.ErrParseArgJSON](err); ok && isWrapperOf(e.Func, wrapper) {
		return &requestError{err: err, statusCode: http.StatusBadRequest, message: fmt.Sprintf("invalid JSON for argument %q", e.ArgName)}, true
	}
	if e, ok := errors.AsType[function.ErrParseArgsJSON](err); ok && isWrapperOf(e.Func, wrapper) {
		return &requestError{err: err, statusCode: http.StatusBadRequest, message: "invalid JSON object for arguments"}, true
	}
	return nil, false
}

// isWrapperOf returns true if errFunc, the function of an argument parse error,
// is wrapper itself, the value wrapper points to,
// or a wrapper embedded in wrapper, like by a decorating struct.
func isWrapperOf(errFunc, wrapper any) bool {
	return isValueOf(reflect.ValueOf(errFunc), reflect.ValueOf(wrapper), 0)
}

// maxWrapperEmbeddingDepth limits the pointers, interfaces, and embedded fields
// that isWrapperOf follows from the passed wrapper to the wrapper of an error,
// because a recursive type can embed itself through pointers without end.
const maxWrapperEmbeddingDepth = 8

func isValueOf(errFunc, wrapper reflect.Value, depth int) bool {
	if !errFunc.IsValid() || !wrapper.IsValid() || depth > maxWrapperEmbeddingDepth {
		return false
	}
	if errFunc.Type() == wrapper.Type() && errFunc.Comparable() && wrapper.Comparable() && errFunc.Equal(wrapper) {
		return true
	}
	switch wrapper.Kind() {
	case reflect.Interface, reflect.Pointer:
		return !wrapper.IsNil() && isValueOf(errFunc, wrapper.Elem(), depth+1)
	case reflect.Struct:
		for i := range wrapper.NumField() {
			if wrapper.Type().Field(i).Anonymous && isValueOf(errFunc, wrapper.Field(i), depth+1) {
				return true
			}
		}
	}
	return false
}
