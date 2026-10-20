package httpfun_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/httpfun"
)

type ctxKey struct{}

func serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// recordingErrHandler records the errors it handles
// and writes them with status 418 to make them visible in the response.
type recordingErrHandler struct {
	errs []error
}

func (h *recordingErrHandler) HandleError(err error, response http.ResponseWriter, request *http.Request) bool {
	h.errs = append(h.errs, err)
	http.Error(response, "handled", http.StatusTeapot)
	return true
}

func TestHandler(t *testing.T) {
	setPrettyPrint(t, false)
	var (
		called bool
		gotCtx context.Context
	)
	calculate := func(ctx context.Context, op string, a, b int) (int, error) {
		called, gotCtx = true, ctx
		switch op {
		case "add":
			return a + b, nil
		case "missing":
			return 0, httperr.NotFound
		case "panic":
			panic("secret panic details")
		}
		return 0, errors.New("secret error details")
	}
	wrapper := function.MustReflectWrapper(calculate, "ctx", "op", "a", "b")
	handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON)

	t.Run("success", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/calculate?op=add&a=5&b=3", nil)
		request = request.WithContext(context.WithValue(request.Context(), ctxKey{}, "value"))

		response := serve(handler, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "8", response.Body.String())
		assert.Equal(t, "value", gotCtx.Value(ctxKey{}), "the function gets the request context")
	})

	t.Run("function error", func(t *testing.T) {
		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=div&a=5&b=0", nil))

		// Internal error details must not be leaked to the client
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "secret")
	})

	t.Run("function error with status", func(t *testing.T) {
		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=missing", nil))

		// Functions can return errors that are HTTP responses
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("panic", func(t *testing.T) {
		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=panic", nil))

		// A panicking function must not crash the server
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "secret")
	})

	t.Run("unparseable argument", func(t *testing.T) {
		called = false

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=add&a=five", nil))

		assert.False(t, called, "the function must not be called with unparsed arguments")
		// An invalid argument value is an error of the client, not of the server,
		// the response names the argument but not its value, which can be large or private
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid value for argument \"a\"\n", response.Body.String())
	})

	t.Run("unparseable argument with error key", func(t *testing.T) {
		// The function was not called, so an argument error
		// must not be written as 200 JSON result by RespondJSONObject
		handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSONObject("result", "error"))

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=add&a=five", nil))

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid value for argument \"a\"\n", response.Body.String())
	})

	t.Run("parse error of another function", func(t *testing.T) {
		// A function that parses data of the server with another wrapper
		// returns an argument parse error that is not caused by the request,
		// so it is an error of the server and the value must not be shown
		inner := function.MustReflectWrapper(func(n int) {}, "n")
		outer := function.MustReflectWrapper(func(ctx context.Context) error {
			_, err := inner.CallWithStrings(ctx, "s3cr3t-server-value")
			return err
		}, "ctx")
		handler := httpfun.Handler(nil, outer, httpfun.RespondJSON)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "s3cr3t-server-value")
	})

	t.Run("malformed request body", func(t *testing.T) {
		handler := httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, wrapper, httpfun.RespondJSON)

		response := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"op":`)))

		// A body that can't be read is an error of the client
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("getArgs error", func(t *testing.T) {
		called = false
		failingArgs := httpfun.RequestArgFromEnvVar("HTTPFUN_TEST_UNSET_ENV_VAR", "op")
		handler := httpfun.Handler(failingArgs, wrapper, httpfun.RespondJSON)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.False(t, called)
		// A missing environment variable is an error of the server configuration,
		// not of the request, and its details are not shown to the client
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "HTTPFUN_TEST_UNSET_ENV_VAR")

		errHandler := new(recordingErrHandler)
		handler = httpfun.Handler(failingArgs, wrapper, httpfun.RespondJSON, errHandler)

		response = serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.False(t, called)
		assert.Equal(t, http.StatusTeapot, response.Code)
		require.Len(t, errHandler.errs, 1)
		assert.ErrorContains(t, errHandler.errs[0], "HTTPFUN_TEST_UNSET_ENV_VAR")
	})

	t.Run("request body too large", func(t *testing.T) {
		called = false
		setMaxRequestBodySize(t, 16)
		handler := httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, wrapper, httpfun.RespondJSON)

		response := serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"op": "add", "a": 5, "b": 3}`)))

		assert.False(t, called)
		// The body is not malformed, so the client is told
		// that it has to send less instead of a generic 400
		assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	})

	t.Run("errHandlers", func(t *testing.T) {
		errHandler := new(recordingErrHandler)
		handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON, errHandler)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=div", nil))

		// An error handler that handles the error replaces HandleError
		assert.Equal(t, http.StatusTeapot, response.Code)
		require.Len(t, errHandler.errs, 1)
		assert.EqualError(t, errHandler.errs[0], "secret error details")
	})

	t.Run("errHandlers stop at the first handled error", func(t *testing.T) {
		// Only the first error handler that handles the error writes the response,
		// handlers that don't handle it like loggers pass it on
		var logged []error
		logger := httpfun.ErrorHandlerFunc(func(err error, _ http.ResponseWriter, _ *http.Request) bool {
			logged = append(logged, err)
			return false
		})
		first, second := new(recordingErrHandler), new(recordingErrHandler)
		handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON, logger, first, second)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=div", nil))

		assert.Equal(t, http.StatusTeapot, response.Code)
		assert.Len(t, logged, 1)
		assert.Len(t, first.errs, 1)
		assert.Empty(t, second.errs, "error was already handled")
	})

	t.Run("unhandled errors go to HandleError", func(t *testing.T) {
		// If no error handler handles the error, HandleError answers it,
		// so the error is not dropped with an empty 200 OK response
		logger := httpfun.ErrorHandlerFunc(func(error, http.ResponseWriter, *http.Request) bool { return false })
		handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON, logger)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=div", nil))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "secret")
	})

	t.Run("httperr handlers are ErrorHandlers", func(t *testing.T) {
		handler := httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON, httperr.DefaultHandler)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/calculate?op=div", nil))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("nil getArgs and resultsWriter", func(t *testing.T) {
		var gotA int
		handler := httpfun.Handler(nil, function.MustReflectWrapper(func(a int) { gotA = a }, "a"), nil)

		response := serve(handler, httptest.NewRequest(http.MethodGet, "/?a=5", nil))

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, response.Body.String())
		assert.Equal(t, 0, gotA, "without getArgs the function gets zero values")
	})
}

func TestHandler_CatchHandlerPanics(t *testing.T) {
	orig := httpfun.CatchHandlerPanics
	httpfun.CatchHandlerPanics = false
	t.Cleanup(func() { httpfun.CatchHandlerPanics = orig })

	handler := httpfun.Handler(nil, function.MustReflectWrapper(func() { panic("boom") }), nil)

	// Panics can be passed through for debugging
	assert.PanicsWithValue(t, "boom", func() {
		serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestHandler_ErrAbortHandler(t *testing.T) {
	// net/http aborts the response for a panic with http.ErrAbortHandler,
	// so it is not recovered and answered like other panics
	for name, handler := range map[string]http.HandlerFunc{
		"Handler":          httpfun.Handler(nil, function.MustReflectWrapper(func() { panic(http.ErrAbortHandler) }), nil),
		"HandlerNoWrapper": httpfun.HandlerNoWrapper(func(context.Context) ([]byte, error) { panic(http.ErrAbortHandler) }, nil),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
				handler(response, httptest.NewRequest(http.MethodGet, "/", nil))
			})
			assert.False(t, response.Flushed)
			assert.Empty(t, response.Body.String(), "no error response written")
		})
	}
}

func TestHandlerNoWrapper(t *testing.T) {
	handler := httpfun.HandlerNoWrapper(
		func(ctx context.Context) ([]byte, error) {
			if ctx.Value(ctxKey{}) == nil {
				return nil, errors.New("secret error details")
			}
			return []byte("a,b\n"), nil
		},
		httpfun.RespondContentType("text/csv"),
	)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := serve(handler, request.WithContext(context.WithValue(request.Context(), ctxKey{}, "value")))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "a,b\n", response.Body.String())
	assert.Equal(t, "text/csv", response.Header().Get("Content-Type"))

	response = serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "secret")

	t.Run("errHandlers", func(t *testing.T) {
		// Like Handler, an error that no error handler handled goes to HandleError,
		// and the first error handler that handles it writes the response
		failing := func(context.Context) ([]byte, error) { return nil, errors.New("secret error details") }
		var logged int
		logger := httpfun.ErrorHandlerFunc(func(error, http.ResponseWriter, *http.Request) bool {
			logged++
			return false
		})

		response := serve(httpfun.HandlerNoWrapper(failing, nil, logger), httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, 1, logged)

		recording := new(recordingErrHandler)
		response = serve(httpfun.HandlerNoWrapper(failing, nil, logger, recording), httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusTeapot, response.Code)
		assert.Len(t, recording.errs, 1)
	})
}

// decoratedWrapper is a function.Wrapper that embeds another wrapper,
// like a wrapper that overrides some of its descriptions.
type decoratedWrapper struct {
	function.Wrapper
}

func TestHandler_requestErrorsOfWrappers(t *testing.T) {
	// An argument parse error of the wrapper passed to Handler is an error
	// of the request also if the wrapper is decorated or passed as pointer
	inner := function.MustReflectWrapper(func(a int) {}, "a")
	for name, wrapper := range map[string]function.CallWithNamedStringsWrapper{
		"decorated": decoratedWrapper{inner},
		"pointer":   &decoratedWrapper{inner},
	} {
		t.Run(name, func(t *testing.T) {
			response := serve(httpfun.Handler(httpfun.RequestQueryArgs, wrapper, httpfun.RespondJSON), httptest.NewRequest(http.MethodGet, "/?a=five", nil))

			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestHandler_getArgsErrors(t *testing.T) {
	wrapper := function.MustReflectWrapper(func(a string) {}, "a")

	t.Run("error with response", func(t *testing.T) {
		// A RequestArgsFunc that checks the request, like a session,
		// can return an error that carries its own response
		getArgs := func(*http.Request) (map[string]string, error) { return nil, httperr.Unauthorized }

		response := serve(httpfun.Handler(getArgs, wrapper, httpfun.RespondJSON), httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, response.Code)

		response = serve(httpfun.Handler(getArgs, wrapper, httpfun.RespondJSON, httperr.DefaultHandler), httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, response.Code, "also with error handlers")
	})

	t.Run("file system error", func(t *testing.T) {
		// Failing to buffer an upload in a temporary file, like on a full disk,
		// is an error of the server, not a bad request,
		// and a missing temporary directory is no 404 Not Found
		getArgs := func(*http.Request) (map[string]string, error) {
			return nil, &os.PathError{Op: "open", Path: "/tmp/multipart-1", Err: fs.ErrNotExist}
		}

		response := serve(httpfun.Handler(getArgs, wrapper, httpfun.RespondJSON), httptest.NewRequest(http.MethodPost, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("body too large with error handlers", func(t *testing.T) {
		// Error handlers get the error marked as error of the request,
		// so httperr answers it with 413 like DefaultHandleError
		setMaxRequestBodySize(t, 10)
		errHandler := new(recordingErrHandler)
		handler := httpfun.Handler(httpfun.RequestBodyAsArg("a"), wrapper, httpfun.RespondJSON, errHandler)

		serve(handler, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 100))))

		require.Len(t, errHandler.errs, 1)
		response := httptest.NewRecorder()
		httperr.DefaultHandler.HandleError(errHandler.errs[0], response, httptest.NewRequest(http.MethodPost, "/", nil))
		assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	})
}
