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
	// so it can be handled by the error handlers of the Handler.
	// An error returned for a failed write is also passed to the error handlers.
	WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error
}

// ResultsWriterFunc is a function type that implements ResultsWriter.
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
// is marshalled as JSON string, or null if nil, and not returned.
//
// An error is returned if the number of results does not match the number of resultKeys
// or number of resultKeys minus one if the last result is an error.
func RespondJSONObject(resultKeys ...string) ResultsWriterFunc {
	// Compact a sorted copy to find all duplicates without modifying resultKeys
	if len(slices.Compact(slices.Sorted(slices.Values(resultKeys)))) != len(resultKeys) {
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
			var errStr any // JSON null for nil error
			if resultErr != nil {
				errStr = resultErr.Error()
			}
			r[resultKeys[len(resultKeys)-1]] = errStr
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

// RespondXML writes the results of a function call as XML to the response,
// multiple results separated by a newline.
// Any resultErr is not handled and will be returned by this method.
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
	response.Header().Set("Content-Type", contenttype.PlainText)
	_, err := response.Write(buf.Bytes())
	return err
}

// RespondHTML writes the results of a function call as HTML to the response,
// []byte results as they are and other results formatted with fmt.Fprint.
// The results are not escaped, so the function has to return valid
// and safe HTML, without unescaped user input.
// Any resultErr is not handled and will be returned by this method.
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
	response.Header().Set("Content-Type", contenttype.HTML)
	_, err := response.Write(buf.Bytes())
	return err
}

// RespondDetectContentType writes the single []byte result of a function call
// to the response with the content type detected by DetectContentType.
// An error is returned for other results.
// Any resultErr is not handled and will be returned by this method.
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

	response.Header().Set("Content-Type", DetectContentType(data))
	_, err := response.Write(data)
	return err
}

// RespondContentType returns a ResultsWriter that writes the single []byte result
// of a function call to the response with contentType.
// An error is returned for other results, see RespondBinary
// for multiple results and results of type string or io.Reader.
// Any resultErr is not handled and will be returned by the writer.
func RespondContentType(contentType string) ResultsWriterFunc {
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

		response.Header().Set("Content-Type", contentType)
		_, err := response.Write(data) //#nosec G705 -- contentType was chosen by the caller of this package
		return err
	})
}

// DetectContentType tries to detect the MIME content-type of data,
// or returns "application/octet-stream" if none could be identified.
func DetectContentType(data []byte) string {
	// JSON objects and arrays, but not scalars like 123 or true
	// that are more likely plain text
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
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

// Static content ResultsWriters also implement http.Handler
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

// RespondNothing is a ResultsWriter and http.Handler that writes nothing,
// so the response is 200 OK without body if nothing else is written.
// Any resultErr is not handled and will be returned by WriteResults.
var RespondNothing respondNothing

type respondNothing struct{}

func (respondNothing) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	return resultErr
}

func (respondNothing) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {}

// RespondStaticHTML is a ResultsWriter and http.Handler
// that responds with its HTML string, ignoring the results.
// Any resultErr is not handled and will be returned by WriteResults.
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
	response.Header().Set("Content-Type", contenttype.HTML)
	response.Write([]byte(html)) //#nosec G104,G705
}

// RespondStaticXML is a ResultsWriter and http.Handler
// that responds with its XML string, ignoring the results.
// Any resultErr is not handled and will be returned by WriteResults.
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
	response.Write([]byte(xml)) //#nosec G104,G705
}

// RespondStaticJSON is a ResultsWriter and http.Handler
// that responds with its JSON string, ignoring the results.
// Any resultErr is not handled and will be returned by WriteResults.
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
	response.Write([]byte(json)) //#nosec G104,G705
}

// RespondStaticPlaintext is a ResultsWriter and http.Handler
// that responds with its plain text string, ignoring the results.
// Any resultErr is not handled and will be returned by WriteResults.
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
	response.Header().Set("Content-Type", contenttype.PlainText)
	response.Write([]byte(text)) //#nosec G104,G705
}

// RespondRedirect is a ResultsWriter and http.Handler
// that redirects to its URL string, ignoring the results.
// As ResultsWriter it redirects requests with other methods than GET and HEAD
// with HTTP status code 303: See Other, so that clients get the URL with GET
// like after a form POST (Post/Redirect/Get),
// and as http.Handler, or for GET and HEAD, with 302: Found.
// Any resultErr is not handled and will be returned by WriteResults.
type RespondRedirect string

func (re RespondRedirect) WriteResults(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
	// Don't handle resultErr or context cancellation
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	http.Redirect(response, request, string(re), redirectStatusAfterCall(request))
	return nil
}

func (re RespondRedirect) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	http.Redirect(response, request, string(re), http.StatusFound)
}

// RespondRedirectFunc is a ResultsWriter and http.Handler
// that redirects to the URL returned by the function, ignoring the results.
// As ResultsWriter it redirects requests with other methods than GET and HEAD
// with HTTP status code 303: See Other, so that clients get the URL with GET
// like after a form POST (Post/Redirect/Get),
// and as http.Handler, or for GET and HEAD, with 302: Found.
// Any resultErr and an error of the function are returned by WriteResults,
// as http.Handler the error of the function is handled by HandleError.
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
	http.Redirect(response, request, url, redirectStatusAfterCall(request))
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

// redirectStatusAfterCall returns the HTTP status code of RespondRedirect
// and RespondRedirectFunc to redirect after the function was called:
// 303 See Other for requests with other methods than GET and HEAD,
// because browsers repeat PUT, PATCH, and DELETE requests
// at the URL of a 302 Found redirect, which is used for GET and HEAD.
func redirectStatusAfterCall(request *http.Request) int {
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		return http.StatusFound
	}
	return http.StatusSeeOther
}
