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
	WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error
}

type HTTPResultsWriterFunc func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error

func (f HTTPResultsWriterFunc) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	return f(results, resultErr, writer, request)
}

var RespondJSON HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
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

// RespondBinary responds with contentType using the binary data from results of type []byte, string, or io.Reader.
func RespondBinary(contentType string) HTTPResultsWriterFunc {
	return func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err() // Don't respond to cancelled request
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

func RespondJSONField(fieldName string) HTTPResultsWriterFunc {
	return func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) (err error) {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err() // Don't respond to cancelled request
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

type RespondStaticJSON string

func (json RespondStaticJSON) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
	}

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Write([]byte(json))
	return nil
}

var RespondXML HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
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

type RespondStaticXML string

func (xml RespondStaticXML) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
	}

	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.Write([]byte(xml))
	return nil
}

var RespondPlaintext HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
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

type RespondStaticPlaintext string

func (text RespondStaticPlaintext) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
	}

	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.Write([]byte(text))
	return nil
}

var RespondHTML HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
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

type RespondStaticHTML string

func (html RespondStaticHTML) WriteResults(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
	}

	writer.Header().Add("Content-Type", "text/html; charset=utf-8")
	writer.Write([]byte(html))
	return nil
}

var RespondDetectContentType HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
	if resultErr != nil {
		return resultErr
	}
	if request.Context().Err() != nil {
		return request.Context().Err() // Don't respond to cancelled request
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

func RespondContentType(contentType string) HTTPResultsWriter {
	return HTTPResultsWriterFunc(func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
		if resultErr != nil {
			return resultErr
		}
		if request.Context().Err() != nil {
			return request.Context().Err() // Don't respond to cancelled request
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

var RespondNothing HTTPResultsWriterFunc = func(results []interface{}, resultErr error, writer http.ResponseWriter, request *http.Request) error {
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

func encodeJSON(response interface{}) ([]byte, error) {
	if PrettyPrint {
		return json.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return json.Marshal(response)
}

func encodeXML(response interface{}) ([]byte, error) {
	if PrettyPrint {
		return xml.MarshalIndent(response, "", PrettyPrintIndent)
	}
	return xml.Marshal(response)
}
