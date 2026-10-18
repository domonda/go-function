package httpfun_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function/httpfun"
)

func TestDetectContentType(t *testing.T) {
	testdataDir := "testdata"

	scenarios := []struct {
		filename            string
		expectedContentType string
	}{
		{filename: "example.json", expectedContentType: "application/json"},
		{filename: "plain", expectedContentType: "text/plain"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.filename, func(t *testing.T) {
			// given
			file := path.Join(testdataDir, scenario.filename)
			content, err := os.ReadFile(file)
			require.NoError(t, err)

			// when
			contentType := httpfun.DetectContentType(content)

			// then
			t.Logf("Actual content type: '%s'", contentType)
			assert.True(t, strings.Contains(contentType, scenario.expectedContentType))
		})
	}
}

// writeResults calls writer like a Handler does after calling the function
// and returns the response and the error returned by the writer.
func writeResults(writer httpfun.ResultsWriter, results []any, resultErr error) (*httptest.ResponseRecorder, error) {
	response := httptest.NewRecorder()
	err := writer.WriteResults(results, resultErr, response, httptest.NewRequest(http.MethodGet, "/", nil))
	return response, err
}

// setPrettyPrint sets httpfun.PrettyPrint for the test
// and restores it when the test finishes.
func setPrettyPrint(t *testing.T, prettyPrint bool) {
	t.Helper()
	orig := httpfun.PrettyPrint
	httpfun.PrettyPrint = prettyPrint
	t.Cleanup(func() { httpfun.PrettyPrint = orig })
}

func TestRespondJSON(t *testing.T) {
	setPrettyPrint(t, false)

	response, err := writeResults(httpfun.RespondJSON, []any{map[string]int{"a": 1}}, nil)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, response.Body.String(), "a single result is not wrapped in an array")
	assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))

	response, err = writeResults(httpfun.RespondJSON, []any{1, "two"}, nil)
	require.NoError(t, err)
	assert.Equal(t, `[1,"two"]`, response.Body.String(), "multiple results are written as array")

	response, err = writeResults(httpfun.RespondJSON, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, response.Body.String(), "a function without results responds with an empty 200 OK")

	setPrettyPrint(t, true)
	response, err = writeResults(httpfun.RespondJSON, []any{map[string]int{"a": 1}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"a\": 1\n}", response.Body.String())
}

// TestResultsWriters_resultErr checks that the writers don't write anything
// for an error of the function but return it, so it can be handled
// by the next writer or the error handler with the right status code.
func TestResultsWriters_resultErr(t *testing.T) {
	writers := map[string]httpfun.ResultsWriter{
		"RespondJSON":              httpfun.RespondJSON,
		"RespondJSONObject":        httpfun.RespondJSONObject("result"),
		"RespondBinary":            httpfun.RespondBinary("application/pdf"),
		"RespondXML":               httpfun.RespondXML,
		"RespondPlaintext":         httpfun.RespondPlaintext,
		"RespondHTML":              httpfun.RespondHTML,
		"RespondDetectContentType": httpfun.RespondDetectContentType,
		"RespondContentType":       httpfun.RespondContentType("text/csv"),
		"RespondNothing":           httpfun.RespondNothing,
		"RespondStaticHTML":        httpfun.RespondStaticHTML("<p>ok</p>"),
		"RespondStaticXML":         httpfun.RespondStaticXML("<ok/>"),
		"RespondStaticJSON":        httpfun.RespondStaticJSON(`"ok"`),
		"RespondStaticPlaintext":   httpfun.RespondStaticPlaintext("ok"),
		"RespondRedirect":          httpfun.RespondRedirect("/ok"),
		"RespondRedirectFunc": httpfun.RespondRedirectFunc(func(*http.Request) (string, error) {
			return "/ok", nil
		}),
	}
	resultErr := errors.New("function failed")
	for name, writer := range writers {
		t.Run(name, func(t *testing.T) {
			response, err := writeResults(writer, []any{[]byte("result")}, resultErr)

			assert.Equal(t, resultErr, err)
			assert.Empty(t, response.Body.String())
			assert.Empty(t, response.Header())
		})
	}
}

// TestResultsWriters_canceledContext checks that nothing is written
// for a request that was canceled by the client.
func TestResultsWriters_canceledContext(t *testing.T) {
	writers := map[string]httpfun.ResultsWriter{
		"RespondJSON":       httpfun.RespondJSON,
		"RespondJSONObject": httpfun.RespondJSONObject("result"),
		"RespondPlaintext":  httpfun.RespondPlaintext,
		"RespondStaticHTML": httpfun.RespondStaticHTML("<p>ok</p>"),
		"RespondRedirect":   httpfun.RespondRedirect("/ok"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, writer := range writers {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

			err := writer.WriteResults([]any{"result"}, nil, response, request)

			assert.NoError(t, err)
			assert.Empty(t, response.Body.String())
		})
	}
}

func TestRespondJSONObject(t *testing.T) {
	setPrettyPrint(t, false)

	response, err := writeResults(httpfun.RespondJSONObject("sum", "product"), []any{8, 15}, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"sum":8,"product":15}`, response.Body.String())
	assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))

	t.Run("error key with error", func(t *testing.T) {
		response, err := writeResults(httpfun.RespondJSONObject("sum", "error"), []any{0}, errors.New("overflow"))

		// The error is part of the response object and handled
		assert.NoError(t, err)
		assert.JSONEq(t, `{"sum":0,"error":"overflow"}`, response.Body.String())
	})

	t.Run("error key without error", func(t *testing.T) {
		response, err := writeResults(httpfun.RespondJSONObject("sum", "error"), []any{8}, nil)

		// A successful call of a function with error result must not fail
		require.NoError(t, err)
		assert.JSONEq(t, `{"sum":8,"error":null}`, response.Body.String())
	})

	t.Run("wrong number of results", func(t *testing.T) {
		response, err := writeResults(httpfun.RespondJSONObject("a", "b", "c"), []any{1}, nil)

		assert.Error(t, err)
		assert.Empty(t, response.Body.String())
	})

	t.Run("duplicate keys", func(t *testing.T) {
		// Duplicate keys would silently overwrite results in the JSON object
		assert.Panics(t, func() { httpfun.RespondJSONObject("a", "a") })
		assert.Panics(t, func() { httpfun.RespondJSONObject("a", "b", "a") })
	})
}

func TestRespondBinary(t *testing.T) {
	writer := httpfun.RespondBinary("application/octet-stream")

	response, err := writeResults(writer, []any{[]byte("a"), "b", strings.NewReader("c")}, nil)
	require.NoError(t, err)
	assert.Equal(t, "abc", response.Body.String(), "all results are concatenated")
	assert.Equal(t, "application/octet-stream", response.Header().Get("Content-Type"))

	response, err = writeResults(writer, []any{42}, nil)
	assert.Error(t, err)
	assert.Empty(t, response.Body.String(), "nothing is written for unsupported result types")
}

func TestRespondXML(t *testing.T) {
	setPrettyPrint(t, false)
	type Point struct{ X, Y int }

	response, err := writeResults(httpfun.RespondXML, []any{Point{1, 2}, Point{3, 4}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "<Point><X>1</X><Y>2</Y></Point>\n<Point><X>3</X><Y>4</Y></Point>", response.Body.String())
	assert.Equal(t, "application/xml", response.Header().Get("Content-Type"))
}

func TestRespondPlaintext(t *testing.T) {
	response, err := writeResults(httpfun.RespondPlaintext, []any{1, 2, "x", 3}, nil)
	require.NoError(t, err)
	// Formatted with fmt.Fprint
	assert.Equal(t, "1 2x3", response.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
}

func TestRespondHTML(t *testing.T) {
	response, err := writeResults(httpfun.RespondHTML, []any{[]byte("<p>"), 42, "</p>"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "<p>42</p>", response.Body.String())
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
}

func TestRespondDetectContentType(t *testing.T) {
	response, err := writeResults(httpfun.RespondDetectContentType, []any{[]byte(`{"a":1}`)}, nil)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, response.Body.String())
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))

	// The content type can only be detected from exactly one []byte result
	_, err = writeResults(httpfun.RespondDetectContentType, []any{"string"}, nil)
	assert.Error(t, err)
	_, err = writeResults(httpfun.RespondDetectContentType, []any{[]byte("a"), []byte("b")}, nil)
	assert.Error(t, err)
}

func TestRespondContentType(t *testing.T) {
	writer := httpfun.RespondContentType("text/csv")

	response, err := writeResults(writer, []any{[]byte("a,b\n")}, nil)
	require.NoError(t, err)
	assert.Equal(t, "a,b\n", response.Body.String())
	assert.Equal(t, "text/csv", response.Header().Get("Content-Type"))

	_, err = writeResults(writer, []any{"a,b"}, nil)
	assert.Error(t, err, "only a []byte result is supported")
}

func TestRespondStatic(t *testing.T) {
	writers := []struct {
		writer interface {
			httpfun.ResultsWriter
			http.Handler
		}
		wantBody        string
		wantContentType string
	}{
		{writer: httpfun.RespondStaticHTML("<p>ok</p>"), wantBody: "<p>ok</p>", wantContentType: "text/html; charset=utf-8"},
		{writer: httpfun.RespondStaticXML("<ok/>"), wantBody: "<ok/>", wantContentType: "application/xml"},
		{writer: httpfun.RespondStaticJSON(`"ok"`), wantBody: `"ok"`, wantContentType: "application/json; charset=utf-8"},
		{writer: httpfun.RespondStaticPlaintext("ok"), wantBody: "ok", wantContentType: "text/plain; charset=utf-8"},
	}
	for _, tt := range writers {
		t.Run(tt.wantContentType, func(t *testing.T) {
			// The static content replaces the results of the function
			response, err := writeResults(tt.writer, []any{"ignored"}, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, response.Body.String())
			assert.Equal(t, tt.wantContentType, response.Header().Get("Content-Type"))

			// Static writers can also be used as http.Handler
			response = httptest.NewRecorder()
			tt.writer.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Equal(t, tt.wantBody, response.Body.String())
		})
	}

	response, err := writeResults(httpfun.RespondNothing, []any{"ignored"}, nil)
	require.NoError(t, err)
	assert.Empty(t, response.Body.String())
}

func TestRespondRedirect(t *testing.T) {
	response, err := writeResults(httpfun.RespondRedirect("/done"), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/done", response.Header().Get("Location"))

	redirectFunc := httpfun.RespondRedirectFunc(func(request *http.Request) (string, error) {
		if request.URL.Query().Get("fail") != "" {
			return "", errors.New("secret details")
		}
		return "/user/" + request.URL.Query().Get("id"), nil
	})

	response = httptest.NewRecorder()
	err = redirectFunc.WriteResults(nil, nil, response, httptest.NewRequest(http.MethodGet, "/?id=42", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/user/42", response.Header().Get("Location"))

	response = httptest.NewRecorder()
	err = redirectFunc.WriteResults(nil, nil, response, httptest.NewRequest(http.MethodGet, "/?fail=1", nil))
	assert.EqualError(t, err, "secret details", "the error is returned to be handled")

	// As http.Handler the error is handled by HandleError without leaking details
	response = httptest.NewRecorder()
	redirectFunc.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?fail=1", nil))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
}

func TestDetectContentType_data(t *testing.T) {
	// JSON arrays are as much JSON as objects,
	// but scalars like 123 are more likely plain text
	assert.Equal(t, "application/json", httpfun.DetectContentType([]byte(` [1, 2] `)), "JSON array")
	assert.Equal(t, "application/json", httpfun.DetectContentType([]byte(`{"a":1}`)), "JSON object")
	assert.Equal(t, "text/plain; charset=utf-8", httpfun.DetectContentType([]byte(`123`)), "JSON number")
}

func TestResultsWriters_replaceContentType(t *testing.T) {
	// A Content-Type set before the results are written, for example by a middleware,
	// must be replaced because a response with two Content-Type headers is invalid
	writers := map[string]httpfun.ResultsWriter{
		"RespondPlaintext":         httpfun.RespondPlaintext,
		"RespondHTML":              httpfun.RespondHTML,
		"RespondDetectContentType": httpfun.RespondDetectContentType,
		"RespondContentType":       httpfun.RespondContentType("text/csv"),
	}
	for name, writer := range writers {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			response.Header().Set("Content-Type", "application/octet-stream")
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			err := writer.WriteResults([]any{[]byte("x")}, nil, response, request)
			require.NoError(t, err)
			assert.Len(t, response.Header().Values("Content-Type"), 1)
		})
	}
}

func TestRespondJSONObject_errorKey(t *testing.T) {
	// With a result key for the error, every error returned by the function
	// is written as JSON string, also HTTP errors like httperr.NotFound,
	// errors of the request are handled by httpfun.Handler before calling the function
	writer := httpfun.RespondJSONObject("error")
	for _, funcErr := range []error{errors.New("function failed"), httperr.NotFound} {
		response, err := writeResults(writer, nil, funcErr)
		require.NoError(t, err)
		assert.JSONEq(t, fmt.Sprintf(`{"error":%q}`, funcErr.Error()), response.Body.String())
	}
}
