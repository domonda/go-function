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

type HTTPResultsWriter interface {
	WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error
}

type HTTPResultsWriterFunc func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error

func (f HTTPResultsWriterFunc) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	return f(results, resultErr, writer, request)
}

var RespondJSON HTTPResultsWriterFunc = func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}

	// no results, just OK
	if len(results) == 0 {
		return nil
	}

	// content-type json is relevant only if there's content
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	// only one result, write it as is
	if len(results) == 1 {
		b, err := encodeJSON(results[0])
		if err != nil {
			return err
		}
		_, err = writer.Write(b)
		return err
	}

	// multiple results, put them in a JSON array
	b, err := encodeJSON(results)
	if err != nil {
		return err
	}
	_, err = writer.Write(b)
	return err
}

// RespondBinary responds with contentType using the binary data from results of type []byte, string, or io.Reader.
func RespondBinary(contentType string) HTTPResultsWriterFunc {
	return func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
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
		writer.Header().Set("Content-Type", contentType)
		_, err = writer.Write(buf.Bytes())
		return err
	}
}

func RespondJSONField(fieldName string) HTTPResultsWriterFunc {
	return func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
		if resultErr != nil || request.Context().Err() != nil {
			return resultErr
		}
		var buf []byte
		m := make(map[string]any)
		if len(results) > 0 {
			m[fieldName] = results[0]
		}
		buf, err = encodeJSON(m)
		if err != nil {
			return err
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, err = writer.Write(buf)
		return err
	}
}

var RespondXML HTTPResultsWriterFunc = func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
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
	_, err := writer.Write(buf)
	return err
}

var RespondPlaintext HTTPResultsWriterFunc = func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
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
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	_, err := writer.Write(buf.Bytes())
	return err
}

var RespondHTML HTTPResultsWriterFunc = func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
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
	writer.Header().Add("Content-Type", "text/html; charset=utf-8")
	_, err := writer.Write(buf.Bytes())
	return err
}

var RespondDetectContentType HTTPResultsWriterFunc = func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
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

	writer.Header().Add("Content-Type", DetectContentType(data))
	_, err := writer.Write(data)
	return err
}

func RespondContentType(contentType string) HTTPResultsWriter {
	return HTTPResultsWriterFunc(func(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
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

		writer.Header().Add("Content-Type", contentType)
		_, err := writer.Write(data)
		return err
	})
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
	_ HTTPResultsWriter = RespondNothing
	_ HTTPResultsWriter = RespondStaticHTML("")
	_ HTTPResultsWriter = RespondStaticXML("")
	_ HTTPResultsWriter = RespondStaticJSON("")
	_ HTTPResultsWriter = RespondStaticPlaintext("")

	_ http.Handler = RespondNothing
	_ http.Handler = RespondStaticHTML("")
	_ http.Handler = RespondStaticXML("")
	_ http.Handler = RespondStaticJSON("")
	_ http.Handler = RespondStaticPlaintext("")
)

var RespondNothing respondNothing

type respondNothing struct{}

func (respondNothing) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	return resultErr
}

func (respondNothing) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {}

type RespondStaticHTML string

func (html RespondStaticHTML) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	html.ServeHTTP(writer, request)
	return nil
}

func (html RespondStaticHTML) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Add("Content-Type", "text/html; charset=utf-8")
	writer.Write([]byte(html)) //#nosec G104
}

type RespondStaticXML string

func (xml RespondStaticXML) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	xml.ServeHTTP(writer, request)
	return nil
}

func (xml RespondStaticXML) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.Write([]byte(xml)) //#nosec G104
}

type RespondStaticJSON string

func (json RespondStaticJSON) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	json.ServeHTTP(writer, request)
	return nil
}

func (json RespondStaticJSON) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write([]byte(json)) //#nosec G104
}

type RespondStaticPlaintext string

func (text RespondStaticPlaintext) WriteResults(results []any, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil || request.Context().Err() != nil {
		return resultErr
	}
	text.ServeHTTP(writer, request)
	return nil
}

func (text RespondStaticPlaintext) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.Write([]byte(text)) //#nosec G104
}
