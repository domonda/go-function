package function

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
)

type ResultsHTTPWriter interface {
	WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error
}

type ResultsHTTPWriterFunc func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error

func (f ResultsHTTPWriterFunc) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	return f(results, resultErr, writer, request)
}

func encodeJSON(response interface{}) ([]byte, error) {
	if PrettyPrint {
		return json.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return json.Marshal(response)
}

var HTTPRespondJSON ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	var buf []byte
	for _, result := range results {
		b, err := encodeJSON(result)
		if err != nil {
			return err
		}
		buf = append(buf, b...)
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write(buf)
	return nil
}

// HTTPRespondBinary responds with contentType using the binary data from results of type []byte, string, or io.Reader.
func HTTPRespondBinary(contentType string) ResultsHTTPWriterFunc {
	return func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err()
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
		writer.Header().Set("Content-Type", contentType)
		writer.Write(buf.Bytes())
		return nil
	}
}

func HTTPRespondJSONField(fieldName string) ResultsHTTPWriterFunc {
	return func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err()
		}
		var buf []byte
		m := make(map[string]interface{})
		if len(results) > 0 {
			m[fieldName] = results[0]
		}
		buf, err = encodeJSON(m)
		if err != nil {
			return err
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Write(buf)
		return nil
	}
}

func encodeXML(response interface{}) ([]byte, error) {
	if PrettyPrint {
		return xml.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return xml.Marshal(response)
}

var HTTPRespondXML ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	var buf []byte
	for _, result := range results {
		b, err := encodeXML(result)
		if err != nil {
			return err
		}
		buf = append(buf, b...)
	}
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.Write(buf)
	return nil
}

var HTTPRespondPlaintext ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	var buf bytes.Buffer
	for _, result := range results {
		if b, ok := result.([]byte); ok {
			buf.Write(b)
		} else {
			fmt.Fprint(&buf, result)
		}
	}
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.Write(buf.Bytes())
	return nil
}

var HTTPRespondHTML ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	var buf bytes.Buffer
	for _, result := range results {
		if b, ok := result.([]byte); ok {
			buf.Write(b)
		} else {
			fmt.Fprint(&buf, result)
		}
	}
	writer.Header().Add("Content-Type", "text/html; charset=utf-8")
	writer.Write(buf.Bytes())
	return nil
}

var HTTPRespondDetectContentType ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	if len(results) != 1 {
		return fmt.Errorf("RespondDetectContentType needs 1 result, got %d", len(results))
	}
	data, ok := results[0].([]byte)
	if !ok {
		return fmt.Errorf("RespondDetectContentType needs []byte result, got %T", results[0])
	}

	writer.Header().Add("Content-Type", DetectContentType(data))
	writer.Write(data)
	return nil
}

func HTTPRespondContentType(contentType string) ResultsHTTPWriter {
	return ResultsHTTPWriterFunc(func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err()
		}
		if len(results) != 1 {
			return fmt.Errorf("RespondDetectContentType needs 1 result, got %d", len(results))
		}
		data, ok := results[0].([]byte)
		if !ok {
			return fmt.Errorf("RespondDetectContentType needs []byte result, got %T", results[0])
		}

		writer.Header().Add("Content-Type", contentType)
		writer.Write(data)
		return nil
	})
}

var HTTPRespondNothing ResultsHTTPWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	return resultErr
}

// DetectContentType tries to detect the MIME content-type of data,
// or returns "application/octet-stream" if none could be identified.
func DetectContentType(data []byte) string {
	kind, _ := filetype.Match(data)
	if kind == types.Unknown {
		return http.DetectContentType(data)
	}
	return kind.MIME.Value
}
