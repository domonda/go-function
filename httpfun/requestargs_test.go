package httpfun_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/httpfun"
)

func TestRequestArgsFuncs(t *testing.T) {
	t.Setenv("HTTPFUN_TEST_ENV_VAR", "from env")

	request := httptest.NewRequest(http.MethodGet, "/?name=Alice&tag=a&tag=b", nil)
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("User-Agent", "test")

	tests := []struct {
		name    string
		getArgs httpfun.RequestArgsFunc
		want    map[string]string
	}{
		{
			name:    "ConstRequestArgs",
			getArgs: httpfun.ConstRequestArgs(map[string]string{"a": "1", "b": "2"}),
			want:    map[string]string{"a": "1", "b": "2"},
		},
		{
			name:    "ConstRequestArg",
			getArgs: httpfun.ConstRequestArg("version", "1.0"),
			want:    map[string]string{"version": "1.0"},
		},
		{
			name:    "RequestQueryArg",
			getArgs: httpfun.RequestQueryArg("name"),
			want:    map[string]string{"name": "Alice"},
		},
		{
			// A missing query param is left out, so the argument gets its zero value
			// and MergeRequestArgs keeps the value of an earlier getter
			name:    "RequestQueryArg missing",
			getArgs: httpfun.RequestQueryArg("missing"),
			want:    map[string]string{},
		},
		{
			// Multiple values are passed like by RequestQueryArgs
			name:    "RequestQueryArg multiple values",
			getArgs: httpfun.RequestQueryArg("tag"),
			want:    map[string]string{"tag": `["a","b"]`},
		},
		{
			name:    "RequestQueryAsArg",
			getArgs: httpfun.RequestQueryAsArg("name", "userName"),
			want:    map[string]string{"userName": "Alice"},
		},
		{
			// Multiple values of a query param are passed as slice literal
			name:    "RequestQueryArgs",
			getArgs: httpfun.RequestQueryArgs,
			want:    map[string]string{"name": "Alice", "tag": `["a","b"]`},
		},
		{
			name:    "RequestHeaderArg",
			getArgs: httpfun.RequestHeaderArg("Authorization"),
			want:    map[string]string{"Authorization": "Bearer token"},
		},
		{
			name:    "RequestHeaderAsArg",
			getArgs: httpfun.RequestHeaderAsArg("Authorization", "token"),
			want:    map[string]string{"token": "Bearer token"},
		},
		{
			name:    "RequestHeadersAsArgs",
			getArgs: httpfun.RequestHeadersAsArgs(map[string]string{"Authorization": "token", "User-Agent": "userAgent"}),
			want:    map[string]string{"token": "Bearer token", "userAgent": "test"},
		},
		{
			// Unlike missing query params, missing headers give empty strings,
			// so a trusted header placed last always overwrites client values
			name:    "missing headers",
			getArgs: httpfun.MergeRequestArgs(httpfun.RequestHeaderArg("X-Missing"), httpfun.RequestHeaderAsArg("X-Missing", "x"), httpfun.RequestHeadersAsArgs(map[string]string{"X-Missing": "y"})),
			want:    map[string]string{"X-Missing": "", "x": "", "y": ""},
		},
		{
			name:    "RequestArgFromEnvVar",
			getArgs: httpfun.RequestArgFromEnvVar("HTTPFUN_TEST_ENV_VAR", "value"),
			want:    map[string]string{"value": "from env"},
		},
		{
			// Later getters overwrite the arguments of earlier ones
			name: "MergeRequestArgs",
			getArgs: httpfun.MergeRequestArgs(
				httpfun.ConstRequestArgs(map[string]string{"name": "default", "version": "1.0"}),
				httpfun.RequestQueryArg("name"),
			),
			want: map[string]string{"name": "Alice", "version": "1.0"},
		},
		{
			// A missing query param doesn't overwrite a default
			name: "MergeRequestArgs keeps defaults",
			getArgs: httpfun.MergeRequestArgs(
				httpfun.ConstRequestArg("limit", "10"),
				httpfun.RequestQueryArg("limit"),
			),
			want: map[string]string{"limit": "10"},
		},
		{
			// A trusted header placed last overwrites the value of the client
			// also if the header is missing, like when a proxy didn't set it
			name: "MergeRequestArgs trusted header last",
			getArgs: httpfun.MergeRequestArgs(
				httpfun.RequestQueryAsArg("name", "tenant"),
				httpfun.RequestHeaderAsArg("X-Tenant", "tenant"),
			),
			want: map[string]string{"tenant": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.getArgs(request)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequestArgFromEnvVar_unset(t *testing.T) {
	getArgs := httpfun.RequestArgFromEnvVar("HTTPFUN_TEST_UNSET_ENV_VAR", "value")

	// An unset variable is a configuration error, not an empty argument
	_, err := getArgs(httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Error(t, err)

	// Errors of any getter fail the merged getter
	_, err = httpfun.MergeRequestArgs(httpfun.RequestQueryArgs, getArgs)(httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Error(t, err)
}

func TestRequestBodyAsArg(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("raw body"))

	got, err := httpfun.RequestBodyAsArg("data")(request)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"data": "raw body"}, got)

	t.Run("MaxRequestBodySize", func(t *testing.T) {
		setMaxRequestBodySize(t, 1024)

		// Bodies above the limit are rejected before they can exhaust memory
		_, err := httpfun.RequestBodyAsArg("data")(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 2048))))

		var maxBytesErr *http.MaxBytesError
		assert.True(t, errors.As(err, &maxBytesErr), "error: %v", err)
	})

	t.Run("MaxRequestBodySize disabled", func(t *testing.T) {
		// A value <= 0 disables the limit like for MaxSliceLiteralElements
		// instead of rejecting every body
		setMaxRequestBodySize(t, 0)

		got, err := httpfun.RequestBodyAsArg("data")(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("raw body")))
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"data": "raw body"}, got)
	})
}

func setMaxRequestBodySize(t *testing.T, size int64) {
	t.Helper()
	orig := httpfun.MaxRequestBodySize
	httpfun.MaxRequestBodySize = size
	t.Cleanup(func() { httpfun.MaxRequestBodySize = orig })
}

func TestRequestBodyJSONFieldsAsArgs(t *testing.T) {
	body := `{"name": "Alice \"A\"", "age": 42, "tags": ["a", "b"], "address": {"city": "Vienna"}, "nickname": null}`
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	got, err := httpfun.RequestBodyJSONFieldsAsArgs(request)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		// JSON strings are unescaped to be used as string arguments
		"name": `Alice "A"`,
		// Other JSON values are passed as JSON to be scanned into the argument types
		"age":     "42",
		"tags":    `["a", "b"]`,
		"address": `{"city": "Vienna"}`,
		// null is left out like a missing field instead of the string "null"
	}, got)

	_, err = httpfun.RequestBodyJSONFieldsAsArgs(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`["not", "an", "object"]`)))
	assert.Error(t, err)

	t.Run("MaxRequestBodySize", func(t *testing.T) {
		setMaxRequestBodySize(t, 1024)

		// Bodies above the limit are rejected before they can exhaust memory
		body := `{"name": "` + strings.Repeat("x", 2048) + `"}`
		_, err := httpfun.RequestBodyJSONFieldsAsArgs(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

		var maxBytesErr *http.MaxBytesError
		assert.True(t, errors.As(err, &maxBytesErr), "error: %v", err)
	})
}

func TestRequestMultipartFormArgs(t *testing.T) {
	newRequest := func(t *testing.T, size int) *http.Request {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("name", "Alice"))
		require.NoError(t, writer.WriteField("tag", "a"))
		require.NoError(t, writer.WriteField("tag", "b"))
		require.NoError(t, writer.WriteField("padding", strings.Repeat("x", size)))
		require.NoError(t, writer.Close())
		request := httptest.NewRequest(http.MethodPost, "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		return request
	}

	got, err := httpfun.RequestMultipartFormArgs(newRequest(t, 0))

	require.NoError(t, err)
	// Multiple values of a form field are passed as slice literal
	assert.Equal(t, map[string]string{"name": "Alice", "tag": `["a","b"]`, "padding": ""}, got)

	t.Run("slice argument", func(t *testing.T) {
		// Multiple values of a form field are the elements of a slice argument
		var gotTags []string
		wrapper := function.MustReflectWrapper(func(name string, tags []string) { gotTags = tags }, "name", "tags")
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("tags", "a, b"))
		require.NoError(t, writer.WriteField("tags", ""))
		require.NoError(t, writer.Close())
		request := httptest.NewRequest(http.MethodPost, "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())

		response := serve(httpfun.Handler(httpfun.RequestMultipartFormArgs, wrapper, nil), request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, []string{"a, b", ""}, gotTags)
	})

	t.Run("MaxMultipartFormSize", func(t *testing.T) {
		orig := httpfun.MaxMultipartFormSize
		httpfun.MaxMultipartFormSize = 1024
		t.Cleanup(func() { httpfun.MaxMultipartFormSize = orig })

		// Bodies above the limit are rejected before they can exhaust memory
		_, err := httpfun.RequestMultipartFormArgs(newRequest(t, 2048))

		var maxBytesErr *http.MaxBytesError
		assert.True(t, errors.As(err, &maxBytesErr), "error: %v", err)
	})
}

// newUploadRequest returns a multipart form request for target
// with a form value and an upload of 4096 bytes.
func newUploadRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("name", "Alice"))
	part, err := writer.CreateFormFile("upload", "upload.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte(strings.Repeat("x", 4096)))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// bufferUploadsInTempDir makes multipart forms buffer the uploads of newUploadRequest
// in temporary files of the returned directory.
func bufferUploadsInTempDir(t *testing.T) (tempDir string) {
	t.Helper()
	orig := httpfun.MaxMultipartFormMemory
	httpfun.MaxMultipartFormMemory = 1024
	t.Cleanup(func() { httpfun.MaxMultipartFormMemory = orig })
	tempDir = t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	return tempDir
}

func TestRequestMultipartFormArgs_removesTempFiles(t *testing.T) {
	// Only form values are used as arguments, so buffered uploads
	// must not remain on disk after the request was handled,
	// also when parsing failed after buffering them,
	// because net/http doesn't remove them for requests cloned by middleware
	requireNoTempFiles := func(t *testing.T, tempDir string) {
		t.Helper()
		tempFiles, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Empty(t, tempFiles, "no temporary files remain")
	}

	t.Run("handled", func(t *testing.T) {
		tempDir := bufferUploadsInTempDir(t)
		handler := httpfun.Handler(httpfun.RequestMultipartFormArgs, function.MustReflectWrapper(func(name string) {}, "name"), nil)

		response := serve(handler, newUploadRequest(t, "/").WithContext(context.Background()))

		require.Equal(t, http.StatusOK, response.Code)
		requireNoTempFiles(t, tempDir)
	})

	t.Run("parse error", func(t *testing.T) {
		tempDir := bufferUploadsInTempDir(t)

		_, err := httpfun.RequestMultipartFormArgs(newUploadRequest(t, "/?a;b").WithContext(context.Background()))

		require.Error(t, err)
		requireNoTempFiles(t, tempDir)
	})
}

func TestRequestMultipartFormArgs_uploadsStayReadable(t *testing.T) {
	// The uploads buffered in temporary files are removed after the response,
	// not while code that reads them is still running
	bufferUploadsInTempDir(t)
	readUploadSize := func(request *http.Request) (map[string]string, error) {
		file, _, err := request.FormFile("upload")
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		return map[string]string{"size": strconv.Itoa(len(data))}, nil
	}

	t.Run("by later getters", func(t *testing.T) {
		args, err := httpfun.MergeRequestArgs(httpfun.RequestMultipartFormArgs, readUploadSize)(newUploadRequest(t, "/"))

		require.NoError(t, err)
		assert.Equal(t, "4096", args["size"])
	})

	t.Run("by middleware after the handler", func(t *testing.T) {
		// A middleware that parsed the form before the handler is responsible for its files
		request := newUploadRequest(t, "/")
		require.NoError(t, request.ParseMultipartForm(httpfun.MaxMultipartFormMemory))
		t.Cleanup(func() { _ = request.MultipartForm.RemoveAll() })
		handler := httpfun.Handler(httpfun.RequestMultipartFormArgs, function.MustReflectWrapper(func(name string) {}, "name"), nil)

		serve(handler, request)

		args, err := readUploadSize(request)
		require.NoError(t, err)
		assert.Equal(t, "4096", args["size"])
	})
}

func TestRequestQueryArgs_sliceArguments(t *testing.T) {
	// Multiple values of a query param are the elements of a slice argument,
	// also values that would be zero values or contain JSON syntax,
	// and a single value is passed unchanged
	var (
		gotInts    []int
		gotStrings []string
		gotName    string
	)
	wrapper := function.MustReflectWrapper(func(ints []int, strs []string, name string) {
		gotInts, gotStrings, gotName = ints, strs, name
	}, "ints", "strs", "name")
	handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, nil)
	query := url.Values{
		"ints": {"1", "2"},
		"strs": {"", "null", `a "quoted", <b>`, "[x]"},
		"name": {`["single"]`},
	}

	response := serve(handler, httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []int{1, 2}, gotInts)
	assert.Equal(t, []string{"", "null", `a "quoted", <b>`, "[x]"}, gotStrings)
	assert.Equal(t, `["single"]`, gotName)
}

func TestRequestBodyJSONFieldsAsArgs_null(t *testing.T) {
	// A null field is a missing argument like with CallWithJSON,
	// so a string argument is "" and not "null"
	var (
		gotStr string
		gotPtr *string
	)
	wrapper := function.MustReflectWrapper(func(s string, p *string) { gotStr, gotPtr = s, p }, "s", "p")
	handler := httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, wrapper, nil)

	response := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"s": null, "p": null}`)))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "", gotStr)
	assert.Nil(t, gotPtr)
}
