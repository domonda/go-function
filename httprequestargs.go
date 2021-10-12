package function

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type HTTPRequestArgsGetter func(*http.Request) (map[string]string, error)

func HTTPRequestBodyAsArg(name string) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		defer request.Body.Close()
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		return map[string]string{name: string(body)}, nil
	}
}

func MergeHTTPRequestArgs(getters ...HTTPRequestArgsGetter) HTTPRequestArgsGetter {
	return func(request *http.Request) (map[string]string, error) {
		args := make(map[string]string)
		for _, getArgs := range getters {
			a, err := getArgs(request)
			if err != nil {
				return nil, err
			}
			for name, value := range a {
				args[name] = value
			}
		}
		return args, nil
	}
}

func HTTPRequestQueryArgs(request *http.Request) (map[string]string, error) {
	args := make(map[string]string)
	for name, values := range request.URL.Query() {
		args[name] = strings.Join(values, ";")
	}
	return args, nil
}

func HTTPRequestBodyJSONFieldsAsArgs(request *http.Request) (map[string]string, error) {
	defer request.Body.Close()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	err = json.Unmarshal(body, &fields)
	if err != nil {
		return nil, err
	}
	args := make(map[string]string)
	for name, rawJSON := range fields {
		if len(rawJSON) == 0 {
			// should never happen with well formed JSON
			return nil, fmt.Errorf("JSON body field %q is empty", name)
		}
		if rawJSON[0] == '"' {
			// Unescape JSON string
			var str string
			err = json.Unmarshal(rawJSON, &str)
			if err != nil {
				return nil, fmt.Errorf("can't unmarshal JSON body field %q as string because of: %w", name, err)
			}
			args[name] = str
			continue
		}
		args[name] = string(rawJSON)
	}
	return args, nil
}
