package httpfun

import (
	"context"
	"net/http"

	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
)

// Handler creates an http.HandlerFunc from a wrapped function.
//
// Parameters:
//   - getArgs: Extracts function arguments from the HTTP request (can be nil for no args)
//   - function: The wrapped function to execute
//   - resultsWriter: Formats and writes results to the response (can be nil for no response)
//   - errHandlers: Optional custom error handlers (uses global HandleError if empty)
//
// The handler:
//   - Recovers from panics if CatchHandlerPanics is true
//   - Parses arguments from the request using getArgs
//   - Executes the wrapped function with request.Context()
//   - Writes results using resultsWriter
//   - Handles errors via errHandlers or global HandleError
//
// Example:
//
//	func Calculate(ctx context.Context, a, b int) (int, error) {
//	    return a + b, nil
//	}
//
//	handler := httpfun.Handler(
//	    httpfun.RequestQueryArgs,
//	    function.MustReflectWrapper("Calculate", Calculate),
//	    httpfun.RespondJSON,
//	)
//	http.Handle("/calculate", handler)
//
// Usage: GET /calculate?a=5&b=3 returns: 8
func Handler(getArgs RequestArgsFunc, function function.CallWithNamedStringsWrapper, resultsWriter ResultsWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if CatchHandlerPanics {
			defer func() {
				if p := recover(); p != nil {
					handleError(httperr.AsError(p), errHandlers, response, request)
				}
			}()
		}

		var args map[string]string
		if getArgs != nil {
			a, err := getArgs(request)
			if err != nil {
				if len(errHandlers) == 0 {
					http.Error(response, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				} else {
					for _, errHandler := range errHandlers {
						errHandler.HandleError(err, response, request)
					}
				}
				return
			}
			args = a
		}

		results, err := function.CallWithNamedStrings(request.Context(), args)
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
//   - errHandlers: Optional custom error handlers
//
// Example:
//
//	func GetStatus(ctx context.Context) ([]byte, error) {
//	    return []byte(`{"status":"ok"}`), nil
//	}
//
//	http.Handle("/status", httpfun.HandlerNoWrapper(
//	    GetStatus,
//	    httpfun.RespondJSON,
//	))
func HandlerNoWrapper(function func(context.Context) ([]byte, error), resultsWriter ResultsWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if CatchHandlerPanics {
			defer func() {
				if p := recover(); p != nil {
					handleError(httperr.AsError(p), errHandlers, response, request)
				}
			}()
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

// handleError routes errors to the appropriate error handler.
// If custom errHandlers are provided, they're called in order.
// Otherwise, the global HandleError function is used.
func handleError(err error, errHandlers []httperr.Handler, response http.ResponseWriter, request *http.Request) {
	if err == nil {
		return
	}
	if len(errHandlers) == 0 {
		HandleError(err, response, request)
		return
	}
	for _, errHandler := range errHandlers {
		errHandler.HandleError(err, response, request)
	}
}
