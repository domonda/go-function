package httpfun_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/httpfun"
)

func TestDefaultHandleError(t *testing.T) {
	// Only errors that Handler marked as errors of the request are answered with 4xx,
	// other errors are errors of the server and their text is not shown to the client
	ctx := context.Background()
	wrapper := function.MustReflectWrapper(func(a int) {}, "a")
	_, errParse := wrapper.CallWithStrings(ctx, "s3cr3t")
	require.ErrorAs(t, errParse, new(function.ErrParseArgString))

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "other error", err: errors.New("secret error details"), wantCode: http.StatusInternalServerError},
		// A wrapped function that parses data of the server with another wrapper
		// returns an argument parse error that is not caused by the request
		{name: "parse error not marked by Handler", err: fmt.Errorf("handling data: %w", errParse), wantCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()

			handled := httpfun.DefaultHandleError(tt.err, response, httptest.NewRequest(http.MethodPost, "/", nil))

			assert.True(t, handled, "every error is answered")
			assert.Equal(t, tt.wantCode, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), "secret", "internal error details must not be leaked")
			assert.NotContains(t, response.Body.String(), "s3cr3t", "values of the server must not be leaked")
		})
	}

	t.Run("nil error", func(t *testing.T) {
		response := httptest.NewRecorder()

		handled := httpfun.DefaultHandleError(nil, response, httptest.NewRequest(http.MethodPost, "/", nil))

		// Nothing is written, so the handler can still write a response
		assert.False(t, handled)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, response.Body.String())
	})
}

func TestDefaultHandleError_delegation(t *testing.T) {
	// A custom HandleError that only adds logging keeps the 400 answer
	// for invalid arguments by delegating to DefaultHandleError,
	// like the documentation recommends
	var logged []error
	orig := httpfun.HandleError
	httpfun.HandleError = func(err error, response http.ResponseWriter, request *http.Request) bool {
		logged = append(logged, err)
		return httpfun.DefaultHandleError(err, response, request)
	}
	t.Cleanup(func() { httpfun.HandleError = orig })
	handler := httpfun.Handler(httpfun.RequestQueryArgs, function.MustReflectWrapper(func(a int) {}, "a"), nil)

	response := serve(handler, httptest.NewRequest(http.MethodGet, "/?a=five", nil))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "invalid value for argument \"a\"\n", response.Body.String())
	require.Len(t, logged, 1, "the custom HandleError is called")
	assert.ErrorAs(t, logged[0], new(function.ErrParseArgString), "the logged error keeps the details")
	// The message of the logged error has the cause that the response leaves out
	assert.ErrorContains(t, logged[0], `can't parse "five" as int`)
}

func TestDefaultHandleError_requestBodyTooLarge(t *testing.T) {
	// Custom HandleError implementations delegating to DefaultHandleError
	// answer request bodies over the limit with 413 like httpfun.Handler does
	setMaxRequestBodySize(t, 10)
	orig := httpfun.HandleError
	httpfun.HandleError = func(err error, response http.ResponseWriter, request *http.Request) bool {
		return httpfun.DefaultHandleError(err, response, request)
	}
	t.Cleanup(func() { httpfun.HandleError = orig })
	handler := httpfun.Handler(httpfun.RequestBodyAsArg("a"), function.MustReflectWrapper(func(a string) {}, "a"), nil)

	response := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 100))))

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)

	t.Run("limit of the function", func(t *testing.T) {
		// A function that reads another stream with a limit, like an upstream response,
		// fails because of the server and not because of the request body
		_, err := io.ReadAll(http.MaxBytesReader(nil, io.NopCloser(strings.NewReader(strings.Repeat("x", 100))), 10))
		require.Error(t, err)
		response := httptest.NewRecorder()

		httpfun.DefaultHandleError(fmt.Errorf("reading upstream: %w", err), response, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
