package function

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type HTTPRequestArgsGetter func(*http.Request) (map[string]string, error)

func HTTPRequestArgs(args map[string]string) HTTPRequestArgsGetter {
	return func(*http.Request) (map[string]string, error) {
		return args, nil
	}
}

func HTTPRequestArg(name, value string) HTTPRequestArgsGetter {
	return HTTPRequestArgs(map[string]string{name: value})
}

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
