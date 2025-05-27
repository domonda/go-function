package httpfun

import (
	"net/http"

	"github.com/ungerik/go-httpx/httperr"
)

var (
	PrettyPrint       = true
	PrettyPrintIndent = "  "

	CatchHandlerPanics = true

	// HandleError will handle a non nil error by writing it to the response.
	// The default is to use github.com/ungerik/go-httpx/httperr.DefaultHandler.
	HandleError = func(err error, response http.ResponseWriter, request *http.Request) {
		if err != nil {
			httperr.DefaultHandler.HandleError(err, response, request)
		}
	}
)
