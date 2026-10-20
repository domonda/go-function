# HTTP status codes reference

This page lists every response status that [`httpfun.Handler`](https://pkg.go.dev/github.com/domonda/go-function/httpfun#Handler), `httpfun.HandlerNoWrapper`, and [`htmlform.Handler`](https://pkg.go.dev/github.com/domonda/go-htmlform#Handler) of go-htmlform produce, the body they write, and the cause. The bodies are plain text written with `http.Error`, unless a response writer or custom error handler writes something else.

## How an error becomes a response

```
error
  │
  ├─► error handlers passed to httpfun.Handler, in order,
  │   until one returns true            (htmlform.Handler has none)
  │
  └─► httpfun.HandleError               (default: httpfun.DefaultHandleError)
        │
        └─► httperr.DefaultHandler of github.com/ungerik/go-httpx
              1. the error, or an error it wraps, implements http.Handler
                 → that handler writes the response
              2. the error wraps a key of httperr.SentinelHandlers
                 → that handler writes the response
              3. anything else → 500 Internal Server Error
```

Errors that the handlers classify as caused by the request implement `http.Handler` internally, which is how they reach step 1 with a 4xx status. Your own errors can do the same: return an `httperr.Response` like `httperr.Errorf(http.StatusConflict, "...")` from the function, or any error type with a `ServeHTTP` method.

The body of a 500 response is `Internal Server Error`. It includes the error text only if `httperr.DebugShowInternalErrorsInResponse` is true, which is meant for development.

## httpfun.Handler

| Status | Body | Cause |
|---|---|---|
| 200 OK | written by the `ResultsWriter` | the function returned without error |
| 200 OK | empty | the `ResultsWriter` is nil, writes nothing (`RespondNothing`), or the function has no results and the writer is `RespondJSON` |
| 302 Found | redirect | `RespondRedirect` or `RespondRedirectFunc` after a call without error, for a GET or HEAD request |
| 303 See Other | redirect | `RespondRedirect` or `RespondRedirectFunc` after a call without error, for other methods like POST, PUT, or DELETE, so that the client gets the redirect URL with GET |
| 400 Bad Request | `invalid value for argument "a"` | `CallWithNamedStrings` couldn't parse the value of argument `a` of this function |
| 400 Bad Request | `invalid JSON for argument "a"` | an `ErrParseArgJSON` of this function, from a custom wrapper |
| 400 Bad Request | `invalid JSON object for arguments` | an `ErrParseArgsJSON` of this function, from a custom wrapper |
| 400 Bad Request | `Bad Request` | the `RequestArgsFunc` failed, like for a body that is not a JSON object or a broken multipart form |
| 413 Request Entity Too Large | `Request Entity Too Large` | the body exceeded `MaxRequestBodySize` (`RequestBodyAsArg`, `RequestBodyJSONFieldsAsArgs`) or `MaxMultipartFormSize` (`RequestMultipartFormArgs`), or the form fields of `RequestMultipartFormArgs` that aren't files exceeded the memory that `http.Request.ParseMultipartForm` allows for them |
| any | written by the error | the `RequestArgsFunc`, the function, or a panic returned an error that implements `http.Handler`, like `httperr.Unauthorized` |
| 404 Not Found | `Requested file not found` | the function's error wraps `os.ErrNotExist` |
| 404 Not Found | `Requested database row not found` | the function's error wraps `sql.ErrNoRows` |
| 500 Internal Server Error | `Internal Server Error` | any other error of the function |
| 500 Internal Server Error | `Internal Server Error` | the function panicked and `CatchHandlerPanics` is true (the default) |
| 500 Internal Server Error | `Internal Server Error` | the `ResultsWriter` failed, like `RespondJSONObject` with the wrong number of keys or a result `json.Marshal` can't encode |
| 500 Internal Server Error | `Internal Server Error` | an environment variable of `RequestArgFromEnvVar` is not set |
| 500 Internal Server Error | `Internal Server Error` | a temporary file for an upload couldn't be written |
| 500 Internal Server Error | `Internal Server Error` | the function returned an argument parse error of *another* wrapper it called, which is a bug of the server, not of the request |
| 500 Internal Server Error | `Internal Server Error` | an argument couldn't be parsed by a wrapper that `Handler` can't recognize, behind a function adapter like `function.CallWithNamedStringsWrapperFunc` or in a named struct field: embed the wrapper to get 400 |

Notes:

- Every error row passes through the error handlers and `HandleError`, so a custom handler can change its response. For the 400 parse-error rows, the error passed to the handlers wraps the parse error, so `errors.AsType[function.ErrParseArgString](err)` finds it.
- The 400 bodies name the argument but never contain its value or the parse error, because both can be large or private.
- With `CatchHandlerPanics` false, a panic is not recovered by the handler. `net/http` then logs it and aborts the response.
- A panic with `http.ErrAbortHandler` is never recovered, so `net/http` aborts the response without logging it, also by `htmlform.Handler`.
- If the request's context is canceled, the built-in `ResultsWriter`s write nothing.

`HandlerNoWrapper` produces the same responses for its function's errors, panics, and `ResultsWriter` errors. It has no arguments to parse.

## htmlform.Handler

The responses of `htmlform.Handler` of the separate module `github.com/domonda/go-htmlform` are listed in its [HTTP status codes reference](https://github.com/domonda/go-htmlform/blob/main/docs/reference-http-status-codes.md).

## Related

- [How to customize HTTP error responses](howto-http-errors.md)
- [Calling conventions reference](reference-calling-conventions.md#errors): the parse error types
- [Security model](explanation-security-model.md): why responses hide argument values and internal errors
