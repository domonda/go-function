package httpfun

import (
	"encoding/json"
	"errors"
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
// The request body is capped by MaxRequestBodySize.
func RequestBodyAsArg(argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		defer request.Body.Close()
		body, err := readRequestBody(request)
		if err != nil {
			return nil, err
		}
		return map[string]string{argName: string(body)}, nil
	}
}

// MergeRequestArgs returns a RequestArgsFunc
// that merges the arguments of the given getters.
// Later getters overwrite earlier ones, so put defaults first
// and trusted values last.
//
// The getters of query params and form fields leave out the params
// that are not in the request, so they keep defaults of earlier getters
// like ConstRequestArg. The getters of headers always set their arguments,
// to an empty string for a missing header, because headers are used for
// trusted values like the ones set by a proxy: placed last, a header getter
// overwrites a value of the client also if the header is missing.
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
// Multiple values are passed like by RequestQueryArgs
// as slice literal of JSON strings like ["1","2"].
// A missing query param is left out, see MergeRequestArgs.
func RequestQueryArg(queryKeyArgName string) RequestArgsFunc {
	return RequestQueryAsArg(queryKeyArgName, queryKeyArgName)
}

// RequestQueryAsArg returns a RequestArgsFunc
// that returns the value of the query param queryKey
// as the value of the argument argName.
// Multiple values are passed like by RequestQueryArgs
// as slice literal of JSON strings like ["1","2"].
// A missing query param is left out, see MergeRequestArgs.
func RequestQueryAsArg(queryKey, argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		values, ok := request.URL.Query()[queryKey]
		if !ok {
			return map[string]string{}, nil
		}
		return map[string]string{argName: joinValues(values)}, nil
	}
}

// RequestHeaderArg returns a RequestArgsFunc
// that returns the first value of the header headerKeyArgName
// as the value of the argument headerKeyArgName.
// A missing header gives an empty string, see MergeRequestArgs.
func RequestHeaderArg(headerKeyArgName string) RequestArgsFunc {
	return RequestHeaderAsArg(headerKeyArgName, headerKeyArgName)
}

// RequestHeaderAsArg returns a RequestArgsFunc
// that returns the first value of the header headerKey
// as the value of the argument argName.
// A missing header gives an empty string, see MergeRequestArgs.
func RequestHeaderAsArg(headerKey, argName string) RequestArgsFunc {
	return RequestHeadersAsArgs(map[string]string{headerKey: argName})
}

// RequestHeadersAsArgs returns a RequestArgsFunc
// that returns the first values of the request headers as argument values.
// The keys of headerToArg are the header keys and the values are the argument names.
// Missing headers give empty strings, see MergeRequestArgs.
func RequestHeadersAsArgs(headerToArg map[string]string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		args := make(map[string]string, len(headerToArg))
		for headerKey, argName := range headerToArg {
			args[argName] = request.Header.Get(headerKey)
		}
		return args, nil
	}
}

// errEnvVarNotSet is an error of the server configuration, not of the request
var errEnvVarNotSet = errors.New("environment variable is not set")

// RequestArgFromEnvVar returns a RequestArgsFunc
// that returns the value of the environment variable envVar
// as the value of the argument argName.
//
// An error is returned if the environment variable is not set.
func RequestArgFromEnvVar(envVar, argName string) RequestArgsFunc {
	return func(request *http.Request) (map[string]string, error) {
		value, ok := os.LookupEnv(envVar)
		if !ok {
			return nil, fmt.Errorf("%w: %s", errEnvVarNotSet, envVar)
		}
		return map[string]string{argName: value}, nil
	}
}

// RequestQueryArgs is a RequestArgsFunc that returns
// the query params of the request as arguments.
// A query param with multiple values like ?x=1&x=2 is passed
// as slice literal of JSON strings like ["1","2"],
// which is scanned as the elements of a slice or array argument
// whose element type is scanned from strings, like numbers, strings, or times.
// Elements of types unmarshalled from JSON objects, like structs or maps,
// can't be passed as multiple values, but as one JSON array like [{"a":1},{"a":2}].
// Invalid UTF-8 in one of multiple values is replaced with U+FFFD
// like by encoding/json, a single value is passed unchanged.
func RequestQueryArgs(request *http.Request) (map[string]string, error) {
	args := make(map[string]string)
	for name, values := range request.URL.Query() {
		args[name] = joinValues(values)
	}
	return args, nil
}

// joinValues returns the single value of a request param,
// or multiple values as slice literal of JSON strings like ["1","2"],
// which function.ScanString scans as elements of slices and arrays
// without interpreting values like "" or "null" as zero values.
func joinValues(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	}
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(values) // Can't fail for strings
	return strings.TrimSuffix(b.String(), "\n")
}

// RequestMultipartFormArgs is a RequestArgsFunc that returns
// the multipart form values of the request as arguments.
// A form field with multiple values is passed like by RequestQueryArgs
// as slice literal of JSON strings like ["1","2"].
// The total request body is capped by MaxMultipartFormSize and the in-memory
// portion by MaxMultipartFormMemory.
// Handler removes the temporary files of uploads after the response.
func RequestMultipartFormArgs(request *http.Request) (map[string]string, error) {
	request.Body = http.MaxBytesReader(nil, request.Body, MaxMultipartFormSize)
	err := request.ParseMultipartForm(MaxMultipartFormMemory) // #nosec G120 -- body bounded by MaxMultipartFormSize above
	if err != nil {
		// Parsing can fail after buffering the uploads in temporary files
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
		return nil, err
	}
	args := make(map[string]string)
	for name, values := range request.MultipartForm.Value {
		args[name] = joinValues(values)
	}
	return args, nil
}

// RequestBodyJSONFieldsAsArgs is a RequestArgsFunc
// that parses the body of the request as JSON object
// with the object field names as argument names
// and the field values as argument values.
// JSON strings are passed unquoted, other values as JSON like 42 or [1,2].
// Fields with the value null are left out like missing fields,
// so a string argument gets "" instead of the string "null",
// and with MergeRequestArgs a null field doesn't replace
// the argument of an earlier RequestArgsFunc.
// The request body is capped by MaxRequestBodySize.
func RequestBodyJSONFieldsAsArgs(request *http.Request) (map[string]string, error) {
	defer request.Body.Close()
	body, err := readRequestBody(request)
	if err != nil {
		return nil, err
	}
	return namedStringsFromJSON(body)
}

// readRequestBody reads the body of the request,
// at most MaxRequestBodySize bytes if it is greater than zero.
func readRequestBody(request *http.Request) ([]byte, error) {
	if MaxRequestBodySize <= 0 {
		return io.ReadAll(request.Body)
	}
	return io.ReadAll(http.MaxBytesReader(nil, request.Body, MaxRequestBodySize))
}

func namedStringsFromJSON(jsonObject []byte) (map[string]string, error) {
	fields := make(map[string]json.RawMessage)
	err := json.Unmarshal(jsonObject, &fields)
	if err != nil {
		return nil, err
	}
	args := make(map[string]string)
	for name, rawJSON := range fields {
		if string(rawJSON) == "null" {
			// Like a missing argument, instead of the string "null"
			// that a string argument would get
			continue
		}
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
