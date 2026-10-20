# HTTP status codes reference

This page lists every response status that [`httpfun.Handler`](https://pkg.go.dev/github.com/domonda/go-function/httpfun#Handler), `httpfun.HandlerNoWrapper`, and [`htmlform.Handler`](https://pkg.go.dev/github.com/domonda/go-function/htmlform#Handler) produce, the body they write, and the cause. The bodies are plain text written with `http.Error`, unless a response writer or custom error handler writes something else.

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
| 413 Request Entity Too Large | `Request Entity Too Large` | the body exceeded `MaxRequestBodySize` (`RequestBodyAsArg`, `RequestBodyJSONFieldsAsArgs`) or `MaxMultipartFormSize` (`RequestMultipartFormArgs`) |
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

| Status | Body | Cause |
|---|---|---|
| 200 OK | the HTML form | GET or HEAD |
| 200 OK | written by the `ResultsWriter` | POST, the function returned without error |
| 400 Bad Request | `invalid multipart form` | the POST body is not a valid `multipart/form-data` form, including URL-encoded forms |
| 400 Bad Request | `invalid value for argument "a"` | the value of field `a` couldn't be parsed |
| 400 Bad Request | `invalid argument "a": <validator error>` | the validator set with `SetArgValidator("a", ...)` returned an error |
| 400 Bad Request | `argument "a" must be an uploaded file` | a non-empty text value was sent for a file argument |
| 400 Bad Request | `argument "a" takes at most N files` | more files than an array argument `[N]fs.File` holds |
| 403 Forbidden | `Forbidden` | a browser sent the POST from another origin, see `SetCrossOriginProtection` |
| 200 OK | empty | POST, the function returned without error and the `ResultsWriter` is nil |
| 405 Method Not Allowed | `Method Not Allowed` | any method other than GET, HEAD, and POST, with the header `Allow: GET, HEAD, POST` |
| 413 Request Entity Too Large | `Request Entity Too Large` | the body exceeded the limit set by `SetMaxRequestBodySize`, default 100 MiB |
| any, 404, 500 | as for `httpfun.Handler` | the function returned an error, mapped like the `httpfun.Handler` rows above |
| 500 Internal Server Error | `Internal Server Error` | the function panicked, always recovered |
| 500 Internal Server Error | `Internal Server Error` | a temporary file for an upload couldn't be written, or the form template failed |

Notes:

- The 400 errors for parsing, validation, and files, and the errors of the function, go to the `ResultsWriter` first, as its `resultErr` argument. A custom writer can render them, for example by showing the form again with the message. The built-in writers return them, and `httpfun.HandleError` writes the response.
- The 400, 403, 405, and 413 responses for the form body itself (`invalid multipart form`, cross-origin, method, size) are written directly and skip the `ResultsWriter` and `HandleError`.
- The function is not called for the 400, 403, 405, and 413 responses in the table. It can still cause a 4xx response itself by returning an error like `httperr.NotFound` or one wrapping `os.ErrNotExist`.
- File arguments that uploads can't fill, like `fs.MemFile` or a file type in a struct field, are rejected by `NewHandler` before any request.

## Related

- [How to customize HTTP error responses](howto-http-errors.md)
- [Calling conventions reference](reference-calling-conventions.md#errors): the parse error types
- [Security model](explanation-security-model.md): why responses hide argument values and internal errors
