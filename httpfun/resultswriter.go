package httpfun

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
	"github.com/ungerik/go-httpx/contenttype"
)

// ResultsWriter implementations write the results of a function call to an HTTP response.
type ResultsWriter interface {
	// WriteResults writes the results and optionally the resultErr to the response.
	// If the method does not handle the resultErr then it should return it
	// so it can be handled by the next writer in the chain.
	WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error
}

type ResultsWriterFunc func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error

func (f ResultsWriterFunc) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	return f(results, resultErr, response, request)
}

// RespondJSON writes the results of a function call as a JSON to the response.
// If the function returns one non-error result then it is marshalled as is.
// If the function returns multiple results then they are marshalled as a JSON array.
// If the function returns an resultErr then it is returned by this method,
// so it can be handled by the next writer in the chain.
// Any resultErr is not handled and will be returned by this method.
var RespondJSON ResultsWriterFunc = func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}

	// no results, just respond with HTTP status 200: OK
	if len(results) == 0 {
		return nil
	}

	var r any
	if len(results) == 1 {
		// only one result, write it as is
		r = results[0]
	} else {
		// multiple results, put them in a JSON array
		r = results
	}
	j, err := encodeJSON(r)
	if err != nil {
		return err
	}
	response.Header().Set("Content-Type", contenttype.JSON)
	_, err = response.Write(j)
	return err
}

// RespondJSONObject writes the results of a function call as a JSON object to the response.
// The resultKeys are the keys of the JSON object, naming the function results in order.
//
// If the last result is an error and the resultKeys don't have a key for it
// then the error is returned unhandled if not nil.
// If there is a result key for the error then the error
// is marshalled as JSON string and not returned.
//
// An error is returned if the number of results does not match the number of resultKeys
// or number of resultKeys minus one if the last result is an error.
func RespondJSONObject(resultKeys ...string) ResultsWriterFunc {
	if len(slices.Compact(resultKeys)) != len(resultKeys) {
		panic(fmt.Sprintf("RespondJSONObject resultKeys contains duplicates: %#v", resultKeys))
	}
	return func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
		// Early return on context cancellation
		if request.Context().Err() != nil {
			return resultErr
		}
		errorHasKey := len(resultKeys) == len(results)+1
		if !errorHasKey && resultErr != nil {
			return resultErr
		}
		if len(resultKeys) != len(results) && !errorHasKey {
			return fmt.Errorf("RespondJSONObject expects %d results for %v, got %d", len(resultKeys), resultKeys, len(results))
		}
		r := make(map[string]any)
		for i, result := range results {
			r[resultKeys[i]] = result
		}
		if errorHasKey {
			r[resultKeys[len(resultKeys)-1]] = resultErr.Error()
		}
		j, err := encodeJSON(r)
		if err != nil {
			return err
		}
		response.Header().Set("Content-Type", contenttype.JSON)
		_, err = response.Write(j)
		return err
	}
}

// RespondBinary responds with contentType using the binary data from results of type []byte, string, or io.Reader.
// Any resultErr is not handled and will be returned by this method.
func RespondBinary(contentType string) ResultsWriterFunc {
	return func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) (err error) {
		// Don't handle resultErr or context cancellation
		if resultErr != nil || request.Context().Err() != nil {
			return resultErr
		}
		var buf bytes.Buffer
		for _, result := range results {
			switch data := result.(type) {
			case []byte:
				_, err = buf.Write(data)
			case string:
				_, err = buf.WriteString(data)
			case io.Reader:
				_, err = io.Copy(&buf, data)
			default:
				return fmt.Errorf("RespondBinary does not support result type %T", result)
			}
			if err != nil {
				return err
			}
		}
		response.Header().Set("Content-Type", contentType)
		_, err = response.Write(buf.Bytes())
		return err
	}
}

var RespondXML ResultsWriterFunc = func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	var buf []byte
	for i, result := range results {
		if i > 0 {
			buf = append(buf, '\n')
		}
		b, err := encodeXML(result)
		if err != nil {
			return err
		}
		buf = append(buf, b...)
	}
	response.Header().Set("Content-Type", contenttype.XML)
	_, err := response.Write(buf)
	return err
}

// RespondPlaintext writes the results of a function call as a plaintext to the response
// using fmt.Fprint to format the results.
// Spaces are added between results when neither is a string.
// Any resultErr is not handled and will be returned by this method.
var RespondPlaintext ResultsWriterFunc = func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	var buf bytes.Buffer
	fmt.Fprint(&buf, results...)
	response.Header().Add("Content-Type", contenttype.PlainText)
	_, err := response.Write(buf.Bytes())
	return err
}

var RespondHTML ResultsWriterFunc = func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	var buf bytes.Buffer
	for _, result := range results {
		if b, ok := result.([]byte); ok {
			buf.Write(b)
		} else {
			fmt.Fprint(&buf, result)
		}
	}
	response.Header().Add("Content-Type", contenttype.HTML)
	_, err := response.Write(buf.Bytes())
	return err
}

var RespondDetectContentType ResultsWriterFunc = func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	if len(results) != 1 {
		return fmt.Errorf("RespondDetectContentType needs 1 result, got %d", len(results))
	}
	data, ok := results[0].([]byte)
	if !ok {
		return fmt.Errorf("RespondDetectContentType needs []byte result, got %T", results[0])
	}

	response.Header().Add("Content-Type", DetectContentType(data))
	_, err := response.Write(data)
	return err
}

func RespondContentType(contentType string) ResultsWriter {
	return ResultsWriterFunc(func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
		// Don't handle resultErr or context cancellation
		if resultErr != nil || request.Context().Err() != nil {
			return resultErr
		}
		if len(results) != 1 {
			return fmt.Errorf("RespondContentType(%s) needs 1 result, got %d: %#v", contentType, len(results), results)
		}
		data, ok := results[0].([]byte)
		if !ok {
			return fmt.Errorf("RespondContentType(%s)  needs []byte result, got %T", contentType, results[0])
		}

		response.Header().Add("Content-Type", contentType)
		_, err := response.Write(data)
		return err
	})
}

// DetectContentType tries to detect the MIME content-type of data,
// or returns "application/octet-stream" if none could be identified.
func DetectContentType(data []byte) string {
	jsonData := map[string]any{}
	jsonErr := json.Unmarshal(data, &jsonData)
	if jsonErr == nil {
		return "application/json"
	}

	kind, _ := filetype.Match(data)
	if kind == types.Unknown {
		return http.DetectContentType(data)
	}
	return kind.MIME.Value
}

func encodeJSON(response any) ([]byte, error) {
	if PrettyPrint {
		return json.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return json.Marshal(response)
}

func encodeXML(response any) ([]byte, error) {
	if PrettyPrint {
		return xml.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return xml.Marshal(response)
}

// Static content HTTPResultsWriter also implement http.Handler
var (
	_ ResultsWriter = RespondNothing
	_ ResultsWriter = RespondStaticHTML("")
	_ ResultsWriter = RespondStaticXML("")
	_ ResultsWriter = RespondStaticJSON("")
	_ ResultsWriter = RespondStaticPlaintext("")
	_ ResultsWriter = RespondRedirect("")
	_ ResultsWriter = RespondRedirectFunc(nil)

	_ http.Handler = RespondNothing
	_ http.Handler = RespondStaticHTML("")
	_ http.Handler = RespondStaticXML("")
	_ http.Handler = RespondStaticJSON("")
	_ http.Handler = RespondStaticPlaintext("")
	_ http.Handler = RespondRedirect("")
	_ http.Handler = RespondRedirectFunc(nil)
)

var RespondNothing respondNothing

type respondNothing struct{}

func (respondNothing) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	return resultErr
}

func (respondNothing) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {}

type RespondStaticHTML string

func (html RespondStaticHTML) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	html.ServeHTTP(response, request)
	return nil
}

func (html RespondStaticHTML) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	response.Header().Add("Content-Type", contenttype.HTML)
	response.Write([]byte(html)) //#nosec G104
}

type RespondStaticXML string

func (xml RespondStaticXML) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	xml.ServeHTTP(response, request)
	return nil
}

func (xml RespondStaticXML) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", contenttype.XML)
	response.Write([]byte(xml)) //#nosec G104
}

type RespondStaticJSON string

func (json RespondStaticJSON) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	json.ServeHTTP(response, request)
	return nil
}

func (json RespondStaticJSON) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", contenttype.JSON)
	response.Write([]byte(json)) //#nosec G104
}

type RespondStaticPlaintext string

func (text RespondStaticPlaintext) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	text.ServeHTTP(response, request)
	return nil
}

func (text RespondStaticPlaintext) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	response.Header().Add("Content-Type", contenttype.PlainText)
	response.Write([]byte(text)) //#nosec G104
}

// RespondRedirect implements HTTPResultsWriter and http.Handler
// with for a redirect URL string.
// The redirect will be done with HTTP status code 302: Found.
type RespondRedirect string

func (re RespondRedirect) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	re.ServeHTTP(response, request)
	return nil
}

func (re RespondRedirect) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	http.Redirect(response, request, string(re), http.StatusFound)
}

// RespondRedirectFunc implements HTTPResultsWriter and http.Handler
// with a function that returns the redirect URL.
// The redirect will be done with HTTP status code 302: Found.
type RespondRedirectFunc func(request *http.Request) (url string, err error)

func (f RespondRedirectFunc) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	url, err := f(request)
	if err != nil {
		return err
	}
	http.Redirect(response, request, url, http.StatusFound)
	return nil
}

func (f RespondRedirectFunc) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	url, err := f(request)
	if err != nil {
		HandleError(err, response, request)
		return
	}
	http.Redirect(response, request, url, http.StatusFound)
}
