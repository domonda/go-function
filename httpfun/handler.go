package httpfun

import (
	"context"
	"net/http"

	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
)

// Handler returns an http.Handler for a function with a CallWithNamedStringsWrapper
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

// HandlerNoWrapper returns an http.Handler for a function without a wrapper
// of type func(context.Context) ([]byte, error) that returns response bytes.
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
