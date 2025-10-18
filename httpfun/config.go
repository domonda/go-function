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
	// When true, panics are caught and converted to HTTP errors via HandleError.
	// When false, panics will crash the server (useful for debugging).
	// Default: true
	//
	// WARNING: This is a global setting. Set it once during initialization.
	// Not thread-safe to modify after handlers start running.
	CatchHandlerPanics = true

	// HandleError is called when an error occurs during request handling.
	// It writes the error to the HTTP response.
	//
	// The default implementation uses github.com/ungerik/go-httpx/httperr.DefaultHandler,
	// which provides sensible error responses with proper HTTP status codes.
	//
	// Customize this to implement your own error handling strategy:
	//
	//	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) {
	//	    log.Printf("HTTP error: %v", err)
	//	    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	//	}
	//
	// WARNING: This is a global setting. Set it once during initialization.
	// Not thread-safe to modify after handlers start running.
	HandleError = func(err error, response http.ResponseWriter, request *http.Request) {
		if err != nil {
			httperr.DefaultHandler.HandleError(err, response, request)
		}
	}
)
