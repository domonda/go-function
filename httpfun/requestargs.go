package httpfun

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
)

// RequestArgsFunc is a function type that
// returns a map of argument names and values
// for a given HTTP request.
type RequestArgsFunc func(*http.Request) (map[string]string, error)

// ConstRequestArgs returns a RequestArgsFunc
// that returns a constant map of argument names and values.
func ConstRequestArgs(args map[string]string) RequestArgsFunc {
	return func(*http.Request) (map[string]string, error) {
		return args, nil
	}
}

// ConstRequestArg returns a RequestArgsFunc
// that returns a constant value for an argument name.
func ConstRequestArg(name, value string) RequestArgsFunc {
	return ConstRequestArgs(map[string]string{name: value})
}

// RequestBodyAsArg returns a RequestArgsFunc
// that returns the body of the request as the value of the argument argName.
func RequestBodyAsArg(argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		defer request.Body.Close()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		return map[string]string{argName: string(body)}, nil
	}
}

// MergeRequestArgs returns a RequestArgsFunc
// that merges the arguments of the given getters.
// Later getters overwrite earlier ones.
func MergeRequestArgs(getters ...RequestArgsFunc) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		args := make(map[string]string)
		for _, getArgs := range getters {
			a, err := getArgs(request)
			if err != nil {
				return nil, err
			}
			maps.Copy(args, a)
		}
		return args, nil
	}
}

// RequestQueryArg returns a RequestArgsFunc
// that returns the value of the query param queryKeyArgName
// as the value of the argument queryKeyArgName.
func RequestQueryArg(queryKeyArgName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{queryKeyArgName: request.URL.Query().Get(queryKeyArgName)}, nil
	}
}

// RequestQueryAsArg returns a RequestArgsFunc
// that returns the value of the query param queryKey
// as the value of the argument argName.
func RequestQueryAsArg(queryKey, argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{argName: request.URL.Query().Get(queryKey)}, nil
	}
}

// RequestHeaderArg returns a RequestArgsFunc
// that returns the value of the header headerKeyArgName
// as the value of the argument headerKeyArgName.
func RequestHeaderArg(headerKeyArgName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{headerKeyArgName: request.Header.Get(headerKeyArgName)}, nil
	}
}

// RequestHeaderAsArg returns a RequestArgsFunc
// that returns the value of the header headerKey
// as the value of the argument argName.
func RequestHeaderAsArg(headerKey, argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{argName: request.Header.Get(headerKey)}, nil
	}
}

// RequestHeadersAsArgs returns a RequestArgsFunc
// that returns the values of the request headers as argument values.
// The keys of headerToArg are the header keys and the values are the argument names.
func RequestHeadersAsArgs(headerToArg map[string]string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		args := make(map[string]string)
		for headerKey, argName := range headerToArg {
			args[argName] = request.Header.Get(headerKey)
		}
		return args, nil
	}
}

// RequestArgFromEnvVar returns a RequestArgsFunc
// that returns the value of the environment variable envVar
// as the value of the argument argName.
//
// An error is returned if the environment variable is not set.
func RequestArgFromEnvVar(envVar, argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		value, ok := os.LookupEnv(envVar)
		if !ok {
			return nil, fmt.Errorf("environment variable %s is not set", envVar)
		}
		return map[string]string{argName: value}, nil
	}
}

// RequestQueryArgs returns the query params of the request as string map.
// If a query param has multiple values, they are joined with ";".
func RequestQueryArgs(request *http.Request) (map[string]string, error) {
	args := make(map[string]string)
	for name, values := range request.URL.Query() {
		args[name] = strings.Join(values, ";")
	}
	return args, nil
}

// RequestMultipartFormArgs returns the multipart form values of the request as string map.
// If a form field has multiple values, they are joined with ";".
func RequestMultipartFormArgs(request *http.Request) (map[string]string, error) {
	err := request.ParseMultipartForm(1 << 20)
	if err != nil {
		return nil, err
	}
	args := make(map[string]string)
	for name, values := range request.MultipartForm.Value {
		args[name] = strings.Join(values, ";")
	}
	return args, nil
}

// RequestBodyJSONFieldsAsArgs returns a RequestArgsFunc
// that parses the body of the request as JSON object
// with the object field names as argument names
// and the field values as argument values.
func RequestBodyJSONFieldsAsArgs(request *http.Request) (map[string]string, error) {
	defer request.Body.Close()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	return namedStringsFromJSON(body)
}

func namedStringsFromJSON(jsonObject []byte) (map[string]string, error) {
	fields := make(map[string]json.RawMessage)
	err := json.Unmarshal(jsonObject, &fields)
	if err != nil {
		return nil, err
	}
	args := make(map[string]string)
	for name, rawJSON := range fields {
		if len(rawJSON) > 0 && rawJSON[0] == '"' {
			// Unescape JSON string
			var str string
			err = json.Unmarshal(rawJSON, &str)
			if err != nil {
				return nil, fmt.Errorf("can't unmarshal JSON object value %q as string because of: %w", name, err)
			}
			args[name] = str
			continue
		}
		args[name] = string(rawJSON)
	}
	return args, nil
}
