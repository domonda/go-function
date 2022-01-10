package function

import (
	"context"
	"net/http"

	"github.com/ungerik/go-httpx/httperr"
)

func HTTPHandler(getArgs HTTPRequestArgsGetter, function CallWithNamedStringsWrapper, resultsWriter HTTPResultsWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if CatchHTTPHandlerPanics {
			defer func() {
				if p := recover(); p != nil {
					err := httperr.AsError(p)
					if len(errHandlers) == 0 {
						httperr.Handle(err, writer, request)
					} else {
						for _, errHandler := range errHandlers {
							errHandler.HandleError(err, writer, request)
						}
					}
				}
			}()
		}

		var args map[string]string
		if getArgs != nil {
			a, err := getArgs(request)
			if err != nil {
				if len(errHandlers) == 0 {
					http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				} else {
					for _, errHandler := range errHandlers {
						errHandler.HandleError(err, writer, request)
					}
				}
				return
			}
			args = a
		}

		results, err := function.CallWithNamedStrings(request.Context(), args)
		if resultsWriter != nil {
			err = resultsWriter.WriteResults(results, err, writer, request)
		}
		if err == nil {
			return
		}

		if len(errHandlers) == 0 {
			httperr.Handle(err, writer, request)
		} else {
			for _, errHandler := range errHandlers {
				errHandler.HandleError(err, writer, request)
			}
		}
	}
}

// HTTPHandlerNoWrapper returns an http.Handler for a function without a wrapper
// of type func(context.Context) ([]byte, error) that returns response bytes.
func HTTPHandlerNoWrapper(function func(context.Context) ([]byte, error), resultsWriter HTTPResultsWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if CatchHTTPHandlerPanics {
			defer func() {
				if p := recover(); p != nil {
					err := httperr.AsError(p)
					if len(errHandlers) == 0 {
						httperr.Handle(err, writer, request)
					} else {
						for _, errHandler := range errHandlers {
							errHandler.HandleError(err, writer, request)
						}
					}
				}
			}()
		}

		result, err := function(request.Context())
		if resultsWriter != nil {
			err = resultsWriter.WriteResults([]interface{}{result}, err, writer, request)
		}
		if err == nil {
			return
		}

		if len(errHandlers) == 0 {
			httperr.Handle(err, writer, request)
		} else {
			for _, errHandler := range errHandlers {
				errHandler.HandleError(err, writer, request)
			}
		}
	}
}
