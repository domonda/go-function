package function

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
)

// HTTPRequestArgsGetter is a function that returns a map of argument names and values
// for a given HTTP request.
type HTTPRequestArgsGetter func(*http.Request) (map[string]string, error)

// ConstHTTPRequestArgs returns a HTTPRequestArgsGetter
// that returns a constant map of argument names and values.
func ConstHTTPRequestArgs(args map[string]string) HTTPRequestArgsGetter {
	return func(*http.Request) (map[string]string, error) {
		return args, nil
	}
}

// ConstHTTPRequestArg returns a HTTPRequestArgsGetter
// that returns a constant value for an argument name.
func ConstHTTPRequestArg(name, value string) HTTPRequestArgsGetter {
	return ConstHTTPRequestArgs(map[string]string{name: value})
}

// HTTPRequestBodyAsArg returns a HTTPRequestArgsGetter
// that returns the body of the request as the value of the argument argName.
func HTTPRequestBodyAsArg(argName string) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		defer request.Body.Close()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		return map[string]string{argName: string(body)}, nil
	}
}

// MergeHTTPRequestArgs returns a HTTPRequestArgsGetter
// that merges the arguments of the given getters.
// Later getters overwrite earlier ones.
func MergeHTTPRequestArgs(getters ...HTTPRequestArgsGetter) HTTPRequestArgsGetter {
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

// HTTPRequestQueryArg returns a HTTPRequestArgsGetter
// that returns the value of the query param queryKeyArgName
// as the value of the argument queryKeyArgName.
func HTTPRequestQueryArg(queryKeyArgName string) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{queryKeyArgName: request.URL.Query().Get(queryKeyArgName)}, nil
	}
}

// HTTPRequestQueryAsArg returns a HTTPRequestArgsGetter
// that returns the value of the query param queryKey
// as the value of the argument argName.
func HTTPRequestQueryAsArg(queryKey, argName string) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		return map[string]string{argName: request.URL.Query().Get(queryKey)}, nil
	}
}

// HTTPRequestArgFromEnvVar returns a HTTPRequestArgsGetter
// that returns the value of the environment variable envVar
// as the value of the argument argName.
//
// An error is returned if the environment variable is not set.
func HTTPRequestArgFromEnvVar(envVar, argName string) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		value, ok := os.LookupEnv(envVar)
		if !ok {
			return nil, fmt.Errorf("environment variable %s is not set", envVar)
		}
		return map[string]string{argName: value}, nil
	}
}

// HTTPRequestQueryArgs returns the query params of the request as string map.
// If a query param has multiple values, they are joined with ";".
func HTTPRequestQueryArgs(request *http.Request) (map[string]string, error) {
	args := make(map[string]string)
	for name, values := range request.URL.Query() {
		args[name] = strings.Join(values, ";")
	}
	return args, nil
}

// HTTPRequestMultipartFormArgs returns the multipart form values of the request as string map.
// If a form field has multiple values, they are joined with ";".
func HTTPRequestMultipartFormArgs(request *http.Request) (map[string]string, error) {
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

// HTTPRequestBodyJSONFieldsAsArgs returns a HTTPRequestArgsGetter
// that parses the body of the request as JSON object
// with the object field names as argument names
// and the field values as argument values.
func HTTPRequestBodyJSONFieldsAsArgs(request *http.Request) (map[string]string, error) {
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
