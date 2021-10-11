package function

import (
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

		args, err := getArgs(request)
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
