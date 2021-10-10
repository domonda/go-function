package function

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/ungerik/go-httpx/httperr"
)

type HTTPRequestArgsFunc func(*http.Request) map[string]string

func HTTPHandler(getArgs HTTPRequestArgsFunc, commandFunc Wrapper, resultsWriter ResultsHTTPWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if CatchHTTPHandlerPanics {
			defer func() {
				handleErr(httperr.AsError(recover()), writer, request, errHandlers)
			}()
		}
		args := getArgs(request)
		results, err := commandFunc.CallWithNamedStrings(request.Context(), args)
		if resultsWriter != nil {
			err = resultsWriter.WriteResults(results, err, writer, request)
		}
		handleErr(err, writer, request, errHandlers)
	}
}

type RequestBodyArgConverter interface {
	RequestBodyToArg(request *http.Request) (name, value string, err error)
}

type RequestBodyArgConverterFunc func(request *http.Request) (name, value string, err error)

func (f RequestBodyArgConverterFunc) RequestBodyToArg(request *http.Request) (name, value string, err error) {
	return f(request)
}

func RequestBodyAsArg(name string) RequestBodyArgConverterFunc {
	return func(request *http.Request) (string, string, error) {
		defer request.Body.Close()
		b, err := ioutil.ReadAll(request.Body)
		if err != nil {
			return "", "", err
		}
		return name, string(b), nil
	}
}

func HTTPHandlerRequestBodyArg(bodyConverter RequestBodyArgConverter, getArgs HTTPRequestArgsFunc, commandFunc Wrapper, resultsWriter ResultsHTTPWriter, errHandlers ...httperr.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if CatchHTTPHandlerPanics {
			defer func() {
				handleErr(httperr.AsError(recover()), writer, request, errHandlers)
			}()
		}

		args := getArgs(request)
		name, value, err := bodyConverter.RequestBodyToArg(request)
		if err != nil {
			handleErr(err, writer, request, errHandlers)
			return
		}
		if _, exists := args[name]; exists {
			err = fmt.Errorf("argument '%s' already set by request URL path", name)
			handleErr(err, writer, request, errHandlers)
			return
		}
		args[name] = value

		results, err := commandFunc.CallWithNamedStrings(request.Context(), args)

		if resultsWriter != nil {
			err = resultsWriter.WriteResults(results, err, writer, request)
		}
		handleErr(err, writer, request, errHandlers)
	}
}

func handleErr(err error, writer http.ResponseWriter, request *http.Request, errHandlers []httperr.Handler) {
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

func HTTPHandlerMapJSONBodyFieldsAsArgs(getArgs HTTPRequestArgsFunc, mapping map[string]string, wrappedHandler http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			httperr.BadRequest.ServeHTTP(writer, request)
			return
		}
		args := getArgs(request)
		err = jsonBodyFieldsAsVars(body, mapping, args)
		if err != nil {
			httperr.BadRequest.ServeHTTP(writer, request)
			return
		}
		wrappedHandler.ServeHTTP(writer, request)
	}
}

func HTTPHandlerJSONBodyFieldsAsArgs(getArgs HTTPRequestArgsFunc, wrappedHandler http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			httperr.BadRequest.ServeHTTP(writer, request)
			return
		}
		args := getArgs(request)
		err = jsonBodyFieldsAsVars(body, nil, args)
		if err != nil {
			httperr.BadRequest.ServeHTTP(writer, request)
			return
		}
		wrappedHandler.ServeHTTP(writer, request)
	}
}

func jsonBodyFieldsAsVars(body []byte, mapping map[string]string, vars map[string]string) error {
	fields := make(map[string]json.RawMessage)
	err := json.Unmarshal(body, &fields)
	if err != nil {
		return err
	}

	if mapping != nil {
		mappedFields := make(map[string]json.RawMessage, len(fields))
		for fieldName, mappedName := range mapping {
			if value, ok := fields[fieldName]; ok {
				mappedFields[mappedName] = value
			}
		}
		fields = mappedFields
	}

	for name, value := range fields {
		if len(value) == 0 {
			// should never happen with well formed JSON
			return fmt.Errorf("JSON body field %q is empty", name)
		}
		valueStr := string(value)
		switch {
		case valueStr == "null":
			// JSON nulls are left alone

		case valueStr[0] == '"':
			// Unescape JSON string
			err = json.Unmarshal(value, &valueStr)
			if err != nil {
				return fmt.Errorf("can't unmarshal JSON body field %q as string because of: %w", name, err)
			}
			vars[name] = valueStr

		default:
			// All other JSON types are mapped directly to string
			vars[name] = valueStr
		}
	}
	return nil
}
