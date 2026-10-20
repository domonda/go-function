package htmlform_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ungerik/go-fs"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/htmlform"
	"github.com/domonda/go-function/httpfun"
)

// describedWrapper adds argument descriptions to a reflection based wrapper
// like gen-func-wrappers does from the function documentation.
type describedWrapper struct {
	function.Wrapper
	argDescriptions []string
}

func (w describedWrapper) ArgDescriptions() []string { return w.argDescriptions }

// formField returns the HTML tag of the form field named name,
// or fails the test if there is not exactly one such field.
func formField(t *testing.T, body, name string) string {
	t.Helper()
	tags := regexp.MustCompile(`<(input|select|textarea)\b[^>]*\bname="`+regexp.QuoteMeta(name)+`"[^>]*>`).FindAllString(body, -1)
	require.Len(t, tags, 1, "form field %q in:\n%s", name, body)
	return tags[0]
}

func isRequired(tag string) bool {
	return regexp.MustCompile(`\srequired\b`).MatchString(tag)
}

func get(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	return response.Body.String()
}

type ctxKey struct{}

func serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// postForm sends values and files as multipart form like a browser submitting the form.
func postForm(t *testing.T, handler http.Handler, values map[string]string, files map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(handler, newPostRequest(t, values, files))
}

// newPostRequest returns a request with values and files as multipart form
// like a browser submitting the form.
func newPostRequest(t *testing.T, values map[string]string, files map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range values {
		require.NoError(t, writer.WriteField(name, value))
	}
	for name, content := range files {
		part, err := writer.CreateFormFile(name, name+".txt")
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestHandler_GET(t *testing.T) {
	// One argument per Go kind that maps to a different input type or required default
	f := func(ctx context.Context, name string, age int, ratio float64, count uint8, active bool, file fs.File, nickname *string) error {
		return nil
	}
	wrapper := describedWrapper{
		Wrapper:         function.MustReflectWrapper(f, "ctx", "name", "age", "ratio", "count", "active", "file", "nickname"),
		argDescriptions: []string{"", "Full name", "", "", "", "", "", ""},
	}
	body := get(t, htmlform.MustNewHandler(wrapper, "Create <User>", httpfun.RespondNothing))

	// The title is HTML escaped because it's user provided text
	assert.Contains(t, body, "<title>Create &lt;User&gt;</title>")
	assert.Contains(t, body, "<h1>Create &lt;User&gt;</h1>")
	assert.Contains(t, body, "<button>Submit</button>")
	// The context is passed by the handler, not by the user
	assert.NotContains(t, body, `name="ctx"`)

	// The label is the argument description, or the argument name without description
	assert.Contains(t, body, `<label for="name">Full name:</label>`)
	assert.Contains(t, body, `<label for="age">age:</label>`)

	fields := []struct {
		name     string
		wantType string
		// Required is derived from the type: strings and pointers may be empty
		wantRequired bool
	}{
		{name: "name", wantType: "text", wantRequired: false},
		{name: "age", wantType: "number", wantRequired: true},
		{name: "ratio", wantType: "number", wantRequired: true},
		{name: "count", wantType: "number", wantRequired: true},
		{name: "file", wantType: "file", wantRequired: true},
		{name: "nickname", wantType: "text", wantRequired: false},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			tag := formField(t, body, field.name)
			assert.Contains(t, tag, `type="`+field.wantType+`"`)
			assert.Equal(t, field.wantRequired, isRequired(tag), tag)
		})
	}
	assert.Contains(t, formField(t, body, "active"), `type="checkbox"`)

	// Number inputs only accept integers with the default step of 1,
	// so float arguments allow any decimal value
	assert.Contains(t, formField(t, body, "ratio"), `step="any"`)
	assert.NotContains(t, formField(t, body, "age"), `step=`)
	assert.NotContains(t, formField(t, body, "count"), `step=`)
}

func TestHandler_GET_customized(t *testing.T) {
	f := func(ctx context.Context, name string, age int, color int, bio string, active bool) error {
		return nil
	}
	handler := htmlform.MustNewHandler(
		function.MustReflectWrapper(f, "ctx", "name", "age", "color", "bio", "active"),
		"Form",
		httpfun.RespondNothing,
	)
	handler.SetArgDefaultValue("name", "Alice")
	handler.SetArgRequired("name", true)
	handler.SetArgRequired("age", false)
	handler.SetArgInputType("age", "range")
	handler.SetArgOptions("color", []htmlform.Option{
		{Label: "Red", Value: 0},
		{Label: "Green", Value: 1},
		{Label: "Blue", Value: 2},
	})
	handler.SetArgDefaultValue("color", 1)
	handler.SetArgInputType("bio", "textarea")
	handler.SetArgDefaultValue("active", true)
	handler.SetSubmitButtonText("Create")

	body := get(t, handler)

	name := formField(t, body, "name")
	assert.Contains(t, name, `value="Alice"`)
	assert.True(t, isRequired(name), "SetArgRequired(true) overrides optional string type")

	age := formField(t, body, "age")
	assert.Contains(t, age, `type="range"`, "SetArgInputType overrides the type of the Go kind")
	assert.False(t, isRequired(age), "SetArgRequired(false) overrides required int type")

	assert.True(t, strings.HasPrefix(formField(t, body, "color"), "<select"))
	options := regexp.MustCompile(`<option value="(\d)"\s*(selected)?>(\w+)</option>`).FindAllStringSubmatch(body, -1)
	require.Len(t, options, 3)
	for i, label := range []string{"Red", "Green", "Blue"} {
		assert.Equal(t, fmt.Sprint(i), options[i][1])
		assert.Equal(t, label, options[i][3])
		// The default value selects the option
		assert.Equal(t, i == 1, options[i][2] == "selected", options[i][0])
	}

	assert.True(t, strings.HasPrefix(formField(t, body, "bio"), "<textarea"))
	assert.Contains(t, formField(t, body, "active"), "checked")
	assert.Contains(t, body, "<button>Create</button>")
}

func TestHandler_POST(t *testing.T) {
	f := func(ctx context.Context, name string, age int, active bool, file fs.File) (string, error) {
		content, err := file.ReadAllString(ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s|%d|%t|%s|%s", name, age, active, file.Name(), content), nil
	}
	handler := htmlform.MustNewHandler(
		function.MustReflectWrapper(f, "ctx", "name", "age", "active", "file"),
		"Form",
		httpfun.RespondJSON,
	)

	// An unchecked checkbox is not submitted by browsers
	// and must be passed as false to the function
	response := postForm(t, handler,
		map[string]string{"name": "Alice", "age": "42"},
		map[string]string{"file": "Hello"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	// The ResultsWriter writes the parsed arguments returned by the function
	assert.JSONEq(t, `"Alice|42|false|file.txt|Hello"`, response.Body.String())
}

func TestHandler_POST_functionError(t *testing.T) {
	f := func(ctx context.Context, name string) error {
		return errors.New("secret database details")
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "name"), "Form", httpfun.RespondJSON)

	response := postForm(t, handler, map[string]string{"name": "Alice"}, nil)

	// Internal error details must not be leaked to the client
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
}

func TestHandler_POST_panic(t *testing.T) {
	f := func(ctx context.Context, name string) error {
		panic("secret panic details")
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "name"), "Form", httpfun.RespondJSON)

	// A panicking function must not crash the server
	response := postForm(t, handler, map[string]string{"name": "Alice"}, nil)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
}

func TestHandler_POST_ErrAbortHandler(t *testing.T) {
	// net/http aborts the response for a panic with http.ErrAbortHandler,
	// so it is not recovered and answered like other panics
	f := func(ctx context.Context, name string) error {
		panic(http.ErrAbortHandler)
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "name"), "Form", httpfun.RespondJSON)

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		postForm(t, handler, map[string]string{"name": "Alice"}, nil)
	})
}

func TestHandler_SetArgValidator(t *testing.T) {
	var (
		called       bool
		validatedAge any
		validatedOK  any
	)
	f := func(ctx context.Context, name string, age int, ok bool) error {
		called = true
		return nil
	}
	var writtenErr error
	handler := htmlform.MustNewHandler(
		function.MustReflectWrapper(f, "ctx", "name", "age", "ok"),
		"Form",
		httpfun.ResultsWriterFunc(func(results []any, resultErr error, response http.ResponseWriter, request *http.Request) error {
			writtenErr = resultErr
			return resultErr
		}),
	)
	handler.SetArgValidator("age", func(ctx context.Context, value any) error {
		validatedAge = value
		if value.(int) < 18 {
			return errors.New("must be 18 or older")
		}
		return nil
	})
	handler.SetArgValidator("ok", func(ctx context.Context, value any) error {
		validatedOK = value
		return nil
	})

	t.Run("valid", func(t *testing.T) {
		called, validatedAge, validatedOK, writtenErr = false, nil, nil, nil

		response := postForm(t, handler, map[string]string{"name": "Alice", "age": "42"}, nil)

		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, called)
		// Validators get the parsed value with the type of the argument, not the form string
		assert.Equal(t, 42, validatedAge)
		// Arguments missing in the form are validated as the zero value the function gets
		assert.Equal(t, false, validatedOK)
	})

	t.Run("invalid", func(t *testing.T) {
		called, validatedAge, validatedOK, writtenErr = false, nil, nil, nil

		response := postForm(t, handler, map[string]string{"name": "Alice", "age": "17"}, nil)

		// The function must never be called with values rejected by a validator
		assert.False(t, called)
		// Invalid user input is a client error that names the argument
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "age")
		assert.Contains(t, response.Body.String(), "must be 18 or older")
		// Like errors of the function, the error goes to the ResultsWriter first
		// so it can render it, for example as part of the form
		assert.Error(t, writtenErr)
	})

	t.Run("unparseable", func(t *testing.T) {
		called, validatedAge, validatedOK, writtenErr = false, nil, nil, nil

		response := postForm(t, handler, map[string]string{"name": "Alice", "age": "not a number"}, nil)

		assert.False(t, called)
		assert.Nil(t, validatedAge, "validator must not be called with an unparsed value")
		// An invalid form value is an error of the client, not of the server
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), `"age"`, "the error names the argument")

		// A validator must not change how parse errors are reported
		unvalidated := htmlform.MustNewHandler(
			function.MustReflectWrapper(f, "ctx", "name", "age", "ok"),
			"Form",
			httpfun.RespondNothing,
		)
		want := postForm(t, unvalidated, map[string]string{"name": "Alice", "age": "not a number"}, nil)
		assert.Equal(t, want.Code, response.Code)
		assert.Equal(t, want.Body.String(), response.Body.String())
	})
}

func TestHandler_methodNotAllowed(t *testing.T) {
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func() {}), "Form", httpfun.RespondNothing)

	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/", nil))

		// Only GET shows and POST submits the form,
		// a 405 response lists the allowed methods like RFC 9110 requires
		assert.Equal(t, http.StatusMethodNotAllowed, response.Code, method)
		assert.Equal(t, "GET, HEAD, POST", response.Header().Get("Allow"), method)
	}

	// HEAD is answered like GET, which http.ServeMux routes to a "GET /path" handler
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
}

func TestHandler_concurrentGET(t *testing.T) {
	// A Handler serves concurrent requests, so rendering one form
	// must not change state shared with other requests.
	// Run with -race to detect unsynchronized access.
	f := func(ctx context.Context, a, b, c string, d int) error { return nil }
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "a", "b", "c", "d"), "Form", httpfun.RespondNothing)

	const numRequests = 50
	bodies := make([]string, numRequests)
	var wg sync.WaitGroup
	for i := range numRequests {
		wg.Go(func() {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			bodies[i] = response.Body.String()
		})
	}
	wg.Wait()

	for _, body := range bodies {
		// Every response has every field exactly once
		for _, name := range []string{"a", "b", "c", "d"} {
			assert.Len(t, regexp.MustCompile(`name="`+name+`"`).FindAllString(body, -1), 1)
		}
	}
}

func TestHandler_POST_tooLarge(t *testing.T) {
	// Uploads must not be accepted without limit,
	// larger requests are rejected before the function is called
	called := false
	wrapper := function.MustReflectWrapper(func(name string) { called = true }, "name")
	handler := htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing)
	handler.SetMaxRequestBodySize(1024)
	values := map[string]string{"name": strings.Repeat("x", 2048)}

	response := postForm(t, handler, values, nil)

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	assert.False(t, called)

	// The limit is a setting of the handler,
	// so other forms can accept larger submissions
	response = postForm(t, htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing), values, nil)

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.True(t, called)

	t.Run("size <= 0", func(t *testing.T) {
		called = false
		handler := htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing)
		handler.SetMaxRequestBodySize(0)

		response := postForm(t, handler, map[string]string{"name": "x"}, nil)

		// A limit computed as 0 by mistake must fail closed instead of disabling the limit
		assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
		assert.False(t, called)
	})
}

func TestHandler_GET_templateError(t *testing.T) {
	// Template errors name fields and types of the server code,
	// a 500 response must not leak them to the client
	defer func(formTemplate string) { htmlform.FormTemplate = formTemplate }(htmlform.FormTemplate)
	htmlform.FormTemplate = `{{.NoSuchField}}<form></form>`
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(name string) {}, "name"), "Form", httpfun.RespondNothing)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "NoSuchField")
	assert.NotContains(t, response.Body.String(), "can't evaluate")
}

func TestHandler_SetArgValidator_anyArg(t *testing.T) {
	// function.ReflectWrapper passes the unparsed string to arguments of type any,
	// so the validator must check that same string and not fail to parse it
	var (
		called    bool
		validated any
	)
	f := func(ctx context.Context, value any) error {
		called = true
		return nil
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "value"), "Form", httpfun.RespondNothing)
	handler.SetArgValidator("value", func(ctx context.Context, value any) error {
		validated = value
		return nil
	})

	response := postForm(t, handler, map[string]string{"value": "raw text"}, nil)

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.True(t, called)
	assert.Equal(t, "raw text", validated)
}

// setStringScanner registers scanner for destType in function.StringScanners
// until the end of the test.
func setStringScanner(t *testing.T, destType reflect.Type, scanner function.StringScannerFunc) {
	t.Helper()
	orig := function.StringScanners
	function.StringScanners = function.StringScanners.WithTypeScanner(destType, scanner)
	t.Cleanup(func() { function.StringScanners = orig })
}

func TestHandler_POST_fileArg(t *testing.T) {
	// Earlier versions recommended registering this scanner,
	// so applications may still have it
	setStringScanner(t, reflect.TypeFor[fs.FileReader](), func(sourceStr string, destPtr any) error {
		*destPtr.(*fs.FileReader) = fs.File(sourceStr)
		return nil
	})
	secretFile := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("server secret"), 0o600))

	var called bool
	readDocument := func(document fs.FileReader) (string, error) {
		called = true
		if document == nil {
			return "no document", nil
		}
		return document.ReadAllString(t.Context())
	}
	functions := []struct {
		name string
		f    any
	}{
		{
			name: "fs.File",
			f: func(ctx context.Context, document fs.File) (string, error) {
				if document == "" {
					return readDocument(nil)
				}
				return readDocument(document)
			},
		},
		{
			name: "fs.FileReader",
			f: func(ctx context.Context, document fs.FileReader) (string, error) {
				return readDocument(document)
			},
		},
	}
	for _, fn := range functions {
		handler := htmlform.MustNewHandler(function.MustReflectWrapper(fn.f, "ctx", "document"), "Form", httpfun.RespondJSON)

		t.Run(fn.name, func(t *testing.T) {
			t.Run("text value", func(t *testing.T) {
				called = false

				// A browser only sends uploads for file inputs,
				// but a client can send any field as text
				response := postForm(t, handler, map[string]string{"document": secretFile}, nil)

				assert.False(t, called, "the function must not read a file at a path chosen by the client")
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Contains(t, response.Body.String(), "document", "the error names the argument")
				assert.NotContains(t, response.Body.String(), "server secret")
			})

			t.Run("text value with upload", func(t *testing.T) {
				called = false

				response := postForm(t, handler, map[string]string{"document": secretFile}, map[string]string{"document": "uploaded content"})

				assert.False(t, called)
				assert.Equal(t, http.StatusBadRequest, response.Code)
			})

			t.Run("upload", func(t *testing.T) {
				response := postForm(t, handler, nil, map[string]string{"document": "uploaded content"})

				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.JSONEq(t, `"uploaded content"`, response.Body.String())
			})

			t.Run("no file selected", func(t *testing.T) {
				// Browsers send an empty text value for a file input without a selected file,
				// which must not fail an optional file argument
				response := postForm(t, handler, map[string]string{"document": ""}, nil)

				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.JSONEq(t, `"no document"`, response.Body.String())
			})
		})
	}
}

// TestHandler_POST_fileArgNotAssignable checks that an upload for a file argument type
// that an fs.File can't be assigned to is answered as error of the server
// without calling the function, instead of passing a zero or unrelated value.
func TestNewHandler_fileArgNotUploadable(t *testing.T) {
	// File arguments that an uploaded fs.File can't be assigned to
	// would show an upload field whose uploads always fail,
	// so they are an error of NewHandler instead of every request
	for name, f := range map[string]any{
		"fs.MemFile":         func(ctx context.Context, document fs.MemFile) error { return nil },
		"pointer to slice":   func(ctx context.Context, document *[]fs.File) error { return nil },
		"slice of pointers":  func(ctx context.Context, document []*fs.File) error { return nil },
		"pointer to pointer": func(ctx context.Context, document **fs.File) error { return nil },
		"slice of slices":    func(ctx context.Context, document [][]fs.File) error { return nil },
	} {
		_, err := htmlform.NewHandler(function.MustReflectWrapper(f, "ctx", "document"), "Form", httpfun.RespondNothing)
		assert.ErrorContains(t, err, `can't pass uploaded files as argument "document"`, name)
	}
}

type (
	// recursiveSlice and recursivePointer are valid Go types
	// whose element types never end
	recursiveSlice   []recursiveSlice
	recursivePointer *recursivePointer
)

func TestHandler_recursiveArgTypes(t *testing.T) {
	// Following the element types of recursive types must not loop forever,
	// which would block every request
	f := func(tree recursiveSlice, ptr recursivePointer) {}
	handler, err := htmlform.NewHandler(function.MustReflectWrapper(f, "tree", "ptr"), "Form", httpfun.RespondNothing)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.Contains(t, get(t, handler), `name="tree"`)
		postForm(t, handler, map[string]string{"tree": "[]"}, nil)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler doesn't return for recursive argument types")
	}
}

func TestNewHandler_nestedFileTypes(t *testing.T) {
	// A text value like {"F":"/etc/hosts"} for a struct with a file field
	// would be the path of a file on the server, and uploads can't fill it
	type withFile struct{ F fs.File }
	type embedding struct{ withFile }
	for name, f := range map[string]any{
		"struct field":          func(arg withFile) {},
		"pointer to struct":     func(arg *withFile) {},
		"slice of structs":      func(arg []withFile) {},
		"embedded struct":       func(arg embedding) {},
		"map value":             func(arg map[string]fs.File) {},
		"map key":               func(arg map[fs.File]int) {},
		"struct field of slice": func(arg struct{ Files []*fs.File }) {},
	} {
		_, err := htmlform.NewHandler(function.MustReflectWrapper(f, "arg"), "Form", httpfun.RespondNothing)
		assert.ErrorContains(t, err, "contains the file type", name)
	}

	// Unexported fields can't be set from a text value
	_, err := htmlform.NewHandler(function.MustReflectWrapper(func(arg struct{ f fs.File }) {}, "arg"), "Form", httpfun.RespondNothing)
	assert.NoError(t, err)
}

func TestHandler_SetArgValidator_parsedOnce(t *testing.T) {
	type token string
	parses := 0
	// A scanner with state, like one generating IDs or reading the clock,
	// returns another value when called again
	setStringScanner(t, reflect.TypeFor[token](), func(sourceStr string, destPtr any) error {
		parses++
		*destPtr.(*token) = token(fmt.Sprintf("%s#%d", sourceStr, parses))
		return nil
	})
	var validated, called any
	f := func(ctx context.Context, tok token) {
		called = tok
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "tok"), "Form", httpfun.RespondNothing)
	handler.SetArgValidator("tok", func(ctx context.Context, value any) error {
		validated = value
		return nil
	})

	response := postForm(t, handler, map[string]string{"tok": "abc"}, nil)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, 1, parses)
	assert.Equal(t, token("abc#1"), validated)
	assert.Equal(t, validated, called, "the function gets the value the validator checked")
}

// TestHandler_SetArgValidator_context checks that validators get the request context,
// so they can be canceled and use request scoped values like the authenticated user.
func TestHandler_SetArgValidator_context(t *testing.T) {
	var validatorCtx context.Context
	f := func(ctx context.Context, name string) {}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "name"), "Form", httpfun.RespondNothing)
	handler.SetArgValidator("name", func(ctx context.Context, value any) error {
		validatorCtx = ctx
		return nil
	})
	request := newPostRequest(t, map[string]string{"name": "Alice"}, nil)
	request = request.WithContext(context.WithValue(request.Context(), ctxKey{}, "value"))

	response := serve(handler, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NotNil(t, validatorCtx)
	assert.Equal(t, "value", validatorCtx.Value(ctxKey{}))
}

// TestHandler_POST_largeUpload checks that uploads larger than httpfun.MaxMultipartFormMemory
// are buffered in temporary files instead of memory, that these files are removed
// after the request, and that a failure to create them is an error of the server.
func TestHandler_POST_largeUpload(t *testing.T) {
	orig := httpfun.MaxMultipartFormMemory
	httpfun.MaxMultipartFormMemory = 1024
	t.Cleanup(func() { httpfun.MaxMultipartFormMemory = orig })
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	var tempFilesDuringCall []os.DirEntry
	f := func(ctx context.Context, document fs.File) (int, error) {
		var err error
		tempFilesDuringCall, err = os.ReadDir(tempDir)
		if err != nil {
			return 0, err
		}
		data, err := document.ReadAll(ctx)
		return len(data), err
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "document"), "Form", httpfun.RespondJSON)

	response := postForm(t, handler, nil, map[string]string{"document": strings.Repeat("x", 4096)})

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.JSONEq(t, `4096`, response.Body.String())
	assert.NotEmpty(t, tempFilesDuringCall, "the upload is buffered in a temporary file instead of memory")
	tempFilesAfterRequest, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, tempFilesAfterRequest, "temporary files are removed after the request")

	t.Run("temp file error", func(t *testing.T) {
		missingDir := filepath.Join(tempDir, "missing")
		t.Setenv("TMPDIR", missingDir)

		response := postForm(t, handler, nil, map[string]string{"document": strings.Repeat("x", 4096)})

		// The request is valid, the server failed to store it,
		// and paths of the server must not be shown to the client
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), missingDir)
	})
}

// TestHandler_SetCrossOriginProtection checks that form submissions of other sites
// in the browser of a user are rejected before the function is called,
// while non-browser and same-origin submissions work,
// and that the protection can be configured or disabled.
func TestHandler_SetCrossOriginProtection(t *testing.T) {
	called := false
	wrapper := function.MustReflectWrapper(func(name string) { called = true }, "name")
	newRequest := func(t *testing.T, header map[string]string) *http.Request {
		request := newPostRequest(t, map[string]string{"name": "Alice"}, nil)
		for key, value := range header {
			request.Header.Set(key, value)
		}
		return request
	}
	tests := []struct {
		name     string
		header   map[string]string
		wantCode int
	}{
		{name: "cross-site browser", header: map[string]string{"Sec-Fetch-Site": "cross-site"}, wantCode: http.StatusForbidden},
		{name: "foreign origin", header: map[string]string{"Origin": "https://attacker.example"}, wantCode: http.StatusForbidden},
		{name: "same-origin browser", header: map[string]string{"Sec-Fetch-Site": "same-origin"}, wantCode: http.StatusOK},
		{name: "same origin", header: map[string]string{"Origin": "http://example.com"}, wantCode: http.StatusOK},
		{name: "non-browser client", header: nil, wantCode: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			handler := htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing)

			response := serve(handler, newRequest(t, tt.header))

			assert.Equal(t, tt.wantCode, response.Code, response.Body.String())
			assert.Equal(t, tt.wantCode == http.StatusOK, called, "the function is only called for accepted requests")
		})
	}

	t.Run("trusted origin", func(t *testing.T) {
		called = false
		protection := http.NewCrossOriginProtection()
		require.NoError(t, protection.AddTrustedOrigin("https://partner.example"))
		handler := htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing)
		handler.SetCrossOriginProtection(protection)

		response := serve(handler, newRequest(t, map[string]string{"Origin": "https://partner.example", "Sec-Fetch-Site": "cross-site"}))

		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, called)
	})

	t.Run("nil disables", func(t *testing.T) {
		called = false
		handler := htmlform.MustNewHandler(wrapper, "Form", httpfun.RespondNothing)
		handler.SetCrossOriginProtection(nil)

		response := serve(handler, newRequest(t, map[string]string{"Sec-Fetch-Site": "cross-site"}))

		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, called)
	})
}

func TestHandler_POST_invalidFormRemovesTempFiles(t *testing.T) {
	// Parsing a form can buffer an upload in a temporary file and still fail,
	// net/http only removes the temporary files of the original request,
	// so a request cloned by a middleware would leave them on disk
	// and a client could fill the disk with failing uploads
	orig := httpfun.MaxMultipartFormMemory
	httpfun.MaxMultipartFormMemory = 1024
	t.Cleanup(func() { httpfun.MaxMultipartFormMemory = orig })
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	called := false
	handler := htmlform.MustNewHandler(
		function.MustReflectWrapper(func(document fs.File) { called = true }, "document"),
		"Form",
		httpfun.RespondNothing,
	)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("document", "document.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte(strings.Repeat("x", 4096)))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	// An invalid URL query fails parsing after the upload was buffered
	request := httptest.NewRequest(http.MethodPost, "/form?a;b", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(context.Background()) // cloned like by a middleware
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "invalid multipart form\n", response.Body.String(), "parse errors can contain server details")
	assert.False(t, called)
	tempFiles, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, tempFiles, "temporary files of the failed form are removed")
}

func TestHandler_POST_invalidValueNotEchoed(t *testing.T) {
	// The 400 response names the argument but doesn't show its value,
	// which can be large or contain private data
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(age int) {}, "age"), "Form", httpfun.RespondNothing)

	response := postForm(t, handler, map[string]string{"age": "s3cr3t" + strings.Repeat("x", 1000)}, nil)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "invalid value for argument \"age\"\n", response.Body.String())
}

func TestHandler_POST_nestedFileArgs(t *testing.T) {
	// Pointers, slices and arrays of files are file arguments too:
	// a text value would be the path of a file on the server,
	// so they only take uploaded files
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("server secret"), 0600))

	var gotSlice []string
	var gotPtr string
	f := func(files []fs.File, single *fs.File) error {
		gotSlice = nil
		for _, file := range files {
			data, err := file.ReadAllString(t.Context())
			if err != nil {
				return err
			}
			gotSlice = append(gotSlice, data)
		}
		gotPtr = ""
		if single != nil {
			data, err := single.ReadAllString(t.Context())
			if err != nil {
				return err
			}
			gotPtr = data
		}
		return nil
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "files", "single"), "Form", httpfun.RespondNothing)

	for _, argName := range []string{"files", "single"} {
		gotSlice, gotPtr = nil, ""
		response := postForm(t, handler, map[string]string{argName: secret}, nil)
		assert.Equal(t, http.StatusBadRequest, response.Code, "text value for %s", argName)
		assert.NotContains(t, gotSlice, "server secret")
		assert.NotEqual(t, "server secret", gotPtr)
	}

	// Several files uploaded with the same field name fill a slice
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, content := range []string{"first", "second"} {
		part, err := writer.CreateFormFile("files", content+".txt")
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	part, err := writer.CreateFormFile("single", "single.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("single"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response := serve(handler, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []string{"first", "second"}, gotSlice)
	assert.Equal(t, "single", gotPtr)

	// The form lets browsers upload multiple files for a slice
	assert.Contains(t, get(t, handler), `name="files" value="" size="40" required multiple/>`)
}

func TestHandler_SetArg_unknownArg(t *testing.T) {
	// A typo or a renamed function argument must not silently
	// remove a validator or another setting of the form
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(ctx context.Context, age int) {}, "ctx", "age"), "Form", httpfun.RespondNothing)
	setters := map[string]func(arg string){
		"SetArgValidator":    func(arg string) { handler.SetArgValidator(arg, func(context.Context, any) error { return nil }) },
		"SetArgRequired":     func(arg string) { handler.SetArgRequired(arg, true) },
		"SetArgOptions":      func(arg string) { handler.SetArgOptions(arg, nil) },
		"SetArgDefaultValue": func(arg string) { handler.SetArgDefaultValue(arg, 1) },
		"SetArgInputType":    func(arg string) { handler.SetArgInputType(arg, "range") },
	}
	for name, set := range setters {
		assert.NotPanics(t, func() { set("age") }, name)
		assert.Panics(t, func() { set("agee") }, name)
		assert.Panics(t, func() { set("ctx") }, "%s: the context argument is not a form field", name)
	}
}

func TestHandler_POST_requestErrorText(t *testing.T) {
	// A ResultsWriter can show errors of the request to the client,
	// like RespondJSONObject with an error key, so their text
	// must not contain the submitted value or details of the server
	f := func(ctx context.Context, age int) error { return nil }
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "ctx", "age"), "Form", httpfun.RespondJSONObject("error"))

	response := postForm(t, handler, map[string]string{"age": "s3cr3t-value"}, nil)

	assert.NotContains(t, response.Body.String(), "s3cr3t-value")
	assert.NotContains(t, response.Body.String(), "func(", "the function signature is a detail of the server")
	assert.Contains(t, response.Body.String(), `invalid value for argument \"age\"`)
}

func TestHandler_POST_fileArgEdgeCases(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("server secret"), 0600))
	called := false

	t.Run("nested file types reject text values", func(t *testing.T) {
		// Any nesting of file types must not take a server path from a text value
		for name, f := range map[string]any{
			"pointer":              func(file *fs.File) { called = true },
			"pointer to interface": func(file *fs.FileReader) { called = true },
			"array":                func(files [2]fs.File) { called = true },
		} {
			called = false
			handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "files"), "Form", httpfun.RespondNothing)

			response := postForm(t, handler, map[string]string{"files": secret}, nil)

			assert.Equal(t, http.StatusBadRequest, response.Code, name)
			assert.False(t, called, name)
		}
	})

	uploadFiles := func(t *testing.T, handler http.Handler, field string, names ...string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for i, name := range names {
			part, err := writer.CreateFormFile(field, name)
			require.NoError(t, err)
			_, err = fmt.Fprintf(part, "content %d", i)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		request := httptest.NewRequest(http.MethodPost, "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		return serve(handler, request)
	}

	t.Run("more files than an array holds", func(t *testing.T) {
		called = false
		handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(files [1]fs.File) { called = true }, "files"), "Form", httpfun.RespondNothing)

		response := uploadFiles(t, handler, "files", "a.txt", "b.txt")

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.False(t, called)
	})

	t.Run("same file name twice", func(t *testing.T) {
		// go-fs gives every upload its own file, also with the same file name,
		// so the second file is not read as the first one
		var contents, names []string
		f := func(files []fs.File) error {
			for _, file := range files {
				content, err := file.ReadAllString(t.Context())
				if err != nil {
					return err
				}
				contents = append(contents, content)
				names = append(names, file.Name())
			}
			return nil
		}
		handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "files"), "Form", httpfun.RespondNothing)

		response := uploadFiles(t, handler, "files", "scan.pdf", "scan.pdf")

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, []string{"content 0", "content 1"}, contents)
		assert.Equal(t, []string{"scan.pdf", "scan (2).pdf"}, names)
	})

	t.Run("file names with URL escapes", func(t *testing.T) {
		// File names are not URL-decoded by go-fs,
		// so they open their own upload instead of no file or the upload of another field
		var docContent, docName, otherContent string
		f := func(doc, other fs.File) error {
			var err error
			docName = doc.Name()
			if docContent, err = doc.ReadAllString(t.Context()); err != nil {
				return err
			}
			if other != "" {
				otherContent, err = other.ReadAllString(t.Context())
			}
			return err
		}
		handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "doc", "other"), "Form", httpfun.RespondNothing)

		for _, name := range []string{"report%201.pdf", "100%25.txt", `say "hi".txt`, "a%2Fb.txt"} {
			response := uploadFiles(t, handler, "doc", name)
			require.Equal(t, http.StatusOK, response.Code, "%s: %s", name, response.Body.String())
			assert.Equal(t, "content 0", docContent, name)
			assert.Equal(t, name, docName)
		}

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for field, upload := range map[string][2]string{"doc": {"..%2Fother%2Fx.txt", "doc content"}, "other": {"x.txt", "other content"}} {
			part, err := writer.CreateFormFile(field, upload[0])
			require.NoError(t, err)
			_, err = part.Write([]byte(upload[1]))
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		request := httptest.NewRequest(http.MethodPost, "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())

		response := serve(handler, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, "doc content", docContent)
		assert.Equal(t, "other content", otherContent)
	})
}

func TestHandler_GET_inputTypesOfTypesWithOwnParsing(t *testing.T) {
	// Number inputs can't submit strings like "5s" or "INFO",
	// so types that parse their own strings get text fields
	var (
		gotTimeout time.Duration
		gotLevel   slog.Level
	)
	f := func(timeout time.Duration, level slog.Level, count int) {
		gotTimeout, gotLevel = timeout, level
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "timeout", "level", "count"), "Form", httpfun.RespondNothing)

	body := get(t, handler)
	assert.Contains(t, formField(t, body, "timeout"), `type="text"`)
	assert.Contains(t, formField(t, body, "level"), `type="text"`)
	assert.Contains(t, formField(t, body, "count"), `type="number"`)

	response := postForm(t, handler, map[string]string{"timeout": "5s", "level": "WARN", "count": "1"}, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, 5*time.Second, gotTimeout)
	assert.Equal(t, slog.LevelWarn, gotLevel)
}

func TestHandler_SetArgDefaultValue_roundTrip(t *testing.T) {
	// Default values are formatted so that submitting the form unchanged,
	// like a browser does, calls the function with the default values
	type point struct{ X, Y int }
	nickname := "bob"
	when := time.Date(2026, 10, 20, 14, 30, 0, 0, function.TimeLocation)
	whenWithSeconds := time.Date(2026, 10, 20, 14, 30, 5, 0, function.TimeLocation)
	var got []any
	f := func(when, whenSec, day time.Time, nickname, none *string, tags []string, nums []int, data []byte, p point, addr netip.Addr) {
		got = []any{when, whenSec, day, nickname, none, tags, nums, data, p, addr}
	}
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "when", "whenSec", "day", "nickname", "none", "tags", "nums", "data", "p", "addr"), "Form", httpfun.RespondNothing)
	handler.SetArgInputType("when", "datetime-local")
	handler.SetArgInputType("whenSec", "datetime-local")
	handler.SetArgInputType("day", "date")
	defaults := map[string]any{
		"when":     when,
		"whenSec":  whenWithSeconds,
		"day":      when,
		"nickname": &nickname,
		"none":     (*string)(nil),
		"tags":     []string{"a b", "c"},
		"nums":     []int{1, 2, 3},
		"data":     []byte("hi"),
		"p":        point{X: 1, Y: 2},
		"addr":     netip.MustParseAddr("192.0.2.1"),
	}
	for arg, value := range defaults {
		handler.SetArgDefaultValue(arg, value)
	}

	body := get(t, handler)
	assert.Contains(t, formField(t, body, "when"), `value="2026-10-20T14:30"`)
	assert.Contains(t, formField(t, body, "whenSec"), `value="2026-10-20T14:30:05"`)
	assert.Contains(t, formField(t, body, "whenSec"), `step="1"`, "the default step of 60s rejects seconds")
	assert.Contains(t, formField(t, body, "day"), `value="2026-10-20"`)
	assert.Contains(t, formField(t, body, "none"), `value=""`)

	// Submit the values of the form like a browser
	values := make(map[string]string)
	for arg := range defaults {
		tag := formField(t, body, arg)
		value := regexp.MustCompile(`value="([^"]*)"`).FindStringSubmatch(tag)[1]
		values[arg] = html.UnescapeString(value)
	}
	response := postForm(t, handler, values, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, function.TimeLocation)
	assert.Equal(t, []any{when, whenWithSeconds, day, &nickname, (*string)(nil), []string{"a b", "c"}, []int{1, 2, 3}, []byte("hi"), point{X: 1, Y: 2}, netip.MustParseAddr("192.0.2.1")}, got)
}

func TestNewHandler_nilResultsWriter(t *testing.T) {
	// Like httpfun.Handler, a nil ResultsWriter writes nothing
	// for a successful call instead of panicking after the call
	var called bool
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(fail bool) error {
		called = true
		if fail {
			return errors.New("secret error")
		}
		return nil
	}, "fail"), "Form", nil)

	response := postForm(t, handler, map[string]string{"fail": "false"}, nil)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Body.String())

	response = postForm(t, handler, map[string]string{"fail": "true"}, nil)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")
}

func TestHandler_SetArgInputType_radio(t *testing.T) {
	var got string
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(size string) { got = size }, "size"), "Form", httpfun.RespondNothing)
	handler.SetArgOptions("size", []htmlform.Option{{Label: "Small", Value: "S"}, {Label: "Large", Value: "L"}})
	handler.SetArgInputType("size", "radio")
	handler.SetArgDefaultValue("size", "L")

	body := get(t, handler)
	assert.Regexp(t, `<input type="radio" id="size-0" name="size" value="S"\s*/>`, body)
	assert.Regexp(t, `<input type="radio" id="size-1" name="size" value="L" checked\s*/>`, body)
	assert.Contains(t, body, `<label style="display: inline" for="size-1">Large</label>`)

	response := postForm(t, handler, map[string]string{"size": "S"}, nil)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "S", got)
}

func TestHandler_SetArgInputType_fileForNonFileArg(t *testing.T) {
	// An upload field for an argument that doesn't take files
	// would silently ignore the uploaded file
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(data []byte, file fs.File) {}, "data", "file"), "Form", httpfun.RespondNothing)
	assert.Panics(t, func() { handler.SetArgInputType("data", "file") })
	assert.NotPanics(t, func() { handler.SetArgInputType("file", "file") })
}

func TestHandler_GET_customTemplateContentType(t *testing.T) {
	// The form is HTML also if the template doesn't start with a tag
	// that http.DetectContentType recognizes
	orig := htmlform.FormTemplate
	htmlform.FormTemplate = `<form method="post">{{range .Fields}}<input name="{{.Name}}">{{end}}</form>`
	t.Cleanup(func() { htmlform.FormTemplate = orig })
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(a string) {}, "a"), "Form", httpfun.RespondNothing)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Equal(t, `<form method="post"><input name="a"></form>`, response.Body.String())
}

func TestHandler_blankArgs(t *testing.T) {
	// Arguments named _ are not used by the function,
	// so they get no form field and their zero value
	var gotA string
	var gotB int
	f := func(a string, _ string, b int, _ int) { gotA, gotB = a, b }
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "a", "_", "b", "_"), "Form", httpfun.RespondNothing)

	assert.NotContains(t, get(t, handler), `name="_"`)
	response := postForm(t, handler, map[string]string{"a": "x", "b": "2"}, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "x", gotA)
	assert.Equal(t, 2, gotB)
	assert.Panics(t, func() { handler.SetArgRequired("_", true) })
}

func TestHandler_POST_multipleValuesForSlice(t *testing.T) {
	// Multiple values of a field, like of a select with the multiple attribute
	// in a custom template, are the elements of a slice argument
	var gotTags []string
	var gotNums [2]int
	f := func(tags []string, nums [2]int) { gotTags, gotNums = tags, nums }
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(f, "tags", "nums"), "Form", httpfun.RespondNothing)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range [][2]string{{"tags", "a, b"}, {"tags", "c"}, {"nums", "1"}, {"nums", "2"}} {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response := serve(handler, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []string{"a, b", "c"}, gotTags)
	assert.Equal(t, [2]int{1, 2}, gotNums)
}

func TestHandler_GET_fileArrayMultiple(t *testing.T) {
	// A file array of length 1 takes one file, so its field doesn't allow selecting more
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(one [1]fs.File, two [2]fs.File) {}, "one", "two"), "Form", httpfun.RespondNothing)
	body := get(t, handler)
	assert.NotContains(t, formField(t, body, "one"), "multiple")
	assert.Contains(t, formField(t, body, "two"), "multiple")
}

func TestHandler_GET_variadicNotRequired(t *testing.T) {
	// Like in Go, a variadic argument can have no values
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(count int, names ...int) {}, "count", "names"), "Form", httpfun.RespondNothing)
	body := get(t, handler)
	assert.True(t, isRequired(formField(t, body, "count")))
	assert.False(t, isRequired(formField(t, body, "names")))
}

func TestHandler_SetArgValidator_nil(t *testing.T) {
	// A nil validator removes the validator instead of failing every submission
	handler := htmlform.MustNewHandler(function.MustReflectWrapper(func(a int) {}, "a"), "Form", httpfun.RespondNothing)
	handler.SetArgValidator("a", func(context.Context, any) error { return errors.New("rejected") })
	handler.SetArgValidator("a", nil)

	response := postForm(t, handler, map[string]string{"a": "1"}, nil)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}
