package httpfun

import (
	"net/http"

	"github.com/ungerik/go-httpx/httperr"
)

var (
	// PrettyPrint controls whether JSON and XML responses are formatted with indentation.
	// Set to false in production for smaller responses and better performance.
	// Default: true
	PrettyPrint = true

	// PrettyPrintIndent is the indentation string used when PrettyPrint is true.
	// Default: "  " (two spaces)
	PrettyPrintIndent = "  "

	// CatchHandlerPanics controls whether panics in HTTP handlers are recovered.
	// When true, panics are caught and converted to errors
	// for the error handlers of the Handler and HandleError.
	// When false, panics will crash the server (useful for debugging).
	// Default: true
	//
	// WARNING: This is a global setting. Set it once during initialization.
	// Not thread-safe to modify after handlers start running.
	CatchHandlerPanics = true

	// HandleError writes the response for an error during request handling
	// that none of the error handlers passed to Handler or HandlerNoWrapper
	// handled, and for an error of RespondRedirectFunc as http.Handler.
	// Its result is ignored, because no other error handler is left.
	// Default: DefaultHandleError
	//
	// Customize this to implement your own error handling strategy.
	// Delegate to DefaultHandleError to keep its responses,
	// like 400 Bad Request for invalid argument values:
	//
	//	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) bool {
	//	    log.Printf("HTTP error: %v", err)
	//	    return httpfun.DefaultHandleError(err, w, r)
	//	}
	//
	// WARNING: This is a global setting. Set it once during initialization.
	// Not thread-safe to modify after handlers start running.
	HandleError ErrorHandlerFunc = DefaultHandleError

	// MaxRequestBodySize caps the request body size read by
	// RequestBodyAsArg and RequestBodyJSONFieldsAsArgs.
	// Requests larger than this are rejected before they can exhaust memory,
	// Handler answers them with 413 Request Entity Too Large.
	// Default: 32 MiB.
	MaxRequestBodySize int64 = 32 << 20

	// MaxMultipartFormSize caps the total request body size accepted by
	// RequestMultipartFormArgs. Requests larger than this are rejected
	// before they can exhaust memory.
	// Default: 32 MiB.
	MaxMultipartFormSize int64 = 32 << 20

	// MaxMultipartFormMemory is the in-memory threshold passed to
	// http.Request.ParseMultipartForm. Form parts above this size are
	// spooled to temporary files.
	// Default: 1 MiB.
	MaxMultipartFormMemory int64 = 1 << 20
)

// DefaultHandleError is the default implementation of HandleError.
// Custom HandleError implementations can delegate to it.
//
// Errors caused by the request that Handler found,
// like an invalid argument value or an unreadable request body,
// are answered with their 4xx status code and a short message
// that names an invalid argument but not its value,
// which can be large or private.
// Request bodies larger than MaxRequestBodySize or MaxMultipartFormSize
// are answered with 413 Request Entity Too Large.
// All other errors are handled by github.com/ungerik/go-httpx/httperr.DefaultHandler,
// which provides sensible error responses with proper HTTP status codes
// and doesn't show the text of internal errors in 500 responses
// unless httperr.DebugShowInternalErrorsInResponse is true.
// This includes errors from parsing arguments of other functions
// that the wrapped function called, which are errors of the server.
//
// It returns true if it wrote a response, which it does for every error.
// A nil error is ignored and returns false.
func DefaultHandleError(err error, response http.ResponseWriter, request *http.Request) (handled bool) {
	if err == nil {
		return false
	}
	// A requestError responds as http.Handler found by httperr
	return httperr.DefaultHandler.HandleError(err, response, request)
}
