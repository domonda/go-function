# How to customize HTTP error responses

This guide changes the status and body that `httpfun.Handler` and `htmlform.Handler` send for errors: map your own errors to 4xx statuses, log every error, reject requests before the function runs, and show error details during development.

By default, an error returned by your function becomes `500 Internal Server Error`, and invalid arguments become `400 Bad Request`. The [HTTP status codes reference](reference-http-status-codes.md) lists every default.

## Prerequisites

- go-function v1.0.0 or later
- An endpoint built with `httpfun.Handler` or `htmlform.Handler`
- For steps 4 to 6, the `httperr` package: `go get github.com/ungerik/go-httpx`

The examples use this function, which wraps a sentinel error:

```go
var ErrUnknownUnit = errors.New("unknown unit")

func Convert(ctx context.Context, degrees float64, to string) (float64, error) {
	switch strings.ToUpper(to) {
	case "C":
		return (degrees - 32) * 5 / 9, nil
	case "F":
		return degrees*9/5 + 32, nil
	default:
		return 0, fmt.Errorf("%w %q, use C or F", ErrUnknownUnit, to)
	}
}

var convert = function.MustReflectWrapper(Convert, "ctx", "degrees", "to")
```

## Steps

### 1. Map an error to a status for one endpoint

Write an `httpfun.ErrorHandler` and pass it to `httpfun.Handler` after the results writer. Return `true` if it wrote a response, `false` to pass the error on:

```go
unknownUnit := httpfun.ErrorHandlerFunc(func(err error, w http.ResponseWriter, r *http.Request) bool {
	if !errors.Is(err, ErrUnknownUnit) {
		return false // Not handled: try the next handler, then httpfun.HandleError
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
	return true
})

mux.Handle("GET /convert", httpfun.Handler(httpfun.RequestQueryArgs, convert, httpfun.RespondJSON, unknownUnit))
```

`GET /convert?degrees=1&to=K` now answers `400` with `unknown unit "K", use C or F`. Other errors still reach `httpfun.HandleError`, so `?degrees=hot` keeps its `400 invalid value for argument "degrees"`.

Only write `err.Error()` for errors whose text you wrote for clients, like this one. See [What reaches the client](explanation-security-model.md#what-reaches-the-client).

### 2. Log every error and keep the default responses

Error handlers run in order until one returns `true`. A handler that logs and returns `false` sees every error:

```go
logErrors := httpfun.ErrorHandlerFunc(func(err error, w http.ResponseWriter, r *http.Request) bool {
	log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	return false
})

httpfun.Handler(httpfun.RequestQueryArgs, convert, httpfun.RespondJSON, logErrors, unknownUnit)
```

The log contains the full error, including the parse error that the 400 response leaves out:

```
GET /convert: unknown unit "K", use C or F
GET /convert: can't parse string value of argument 'degrees' of function main.Convert(ctx context.Context, degrees float64, to string) (float64, error): can't parse "hot" as float because of: invalid syntax
```

### 3. Change the responses of every endpoint, including forms

`htmlform.Handler` takes no error handlers. It passes errors to its results writer and then to the global `httpfun.HandleError`, which also answers every error that the handlers of an `httpfun.Handler` didn't. Replace it once at program start, before serving requests, and delegate to `httpfun.DefaultHandleError` for everything else:

```go
httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) bool {
	log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	if errors.Is(err, ErrUnknownUnit) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	return httpfun.DefaultHandleError(err, w, r)
}
```

Delegating keeps the default statuses: 400 for invalid arguments, 413 for oversized bodies, 404 for `os.ErrNotExist`, and 500 without details for the rest. A `HandleError` that writes every error itself loses them.

`HandleError` is a package-level variable, so it isn't safe to change while requests are being handled.

### 4. Choose the status in the function

An error that implements `http.Handler` writes its own response. The `httperr` package of go-httpx provides such errors:

```go
import "github.com/ungerik/go-httpx/httperr"

return 0, httperr.Errorf(http.StatusUnprocessableEntity, "unknown unit %q, use C or F", to)
```

`GET /convert?degrees=1&to=K` now answers `422` with `unknown unit "K", use C or F`, through `httpfun.Handler` and `htmlform.Handler`, without any error handler. Predefined values like `httperr.NotFound`, `httperr.Forbidden`, and `httperr.Unauthorized` work the same way.

This ties the function to HTTP. To keep it free of HTTP, map a sentinel error globally instead:

```go
func init() {
	httperr.SentinelHandlers[ErrUnknownUnit] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unknown unit, use C or F", http.StatusBadRequest)
	})
}
```

Every error that wraps `ErrUnknownUnit` now answers `400 unknown unit, use C or F`, for `httpfun` and `htmlform` handlers and for anything else in the process that uses `httperr`. The sentinel handler doesn't get the error, so the body is fixed text. `os.ErrNotExist` and `sql.ErrNoRows` are mapped to 404 this way by default.

### 5. Reject a request before the function runs

A `RequestArgsFunc` can return an error that carries its response. Merge it before the sources of the arguments:

```go
requireAPIKey := func(r *http.Request) (map[string]string, error) {
	if r.Header.Get("X-API-Key") != apiKey {
		return nil, httperr.Unauthorized
	}
	return nil, nil
}

httpfun.Handler(httpfun.MergeRequestArgs(requireAPIKey, httpfun.RequestQueryArgs), convert, httpfun.RespondJSON)
```

Without the header the response is `401 Unauthorized` and `Convert` is not called. Most other errors of a `RequestArgsFunc` become `400 Bad Request`: bodies over the size limit get 413, and a missing environment variable of `RequestArgFromEnvVar` or a failed temporary upload file gets 500. For checks shared by many endpoints, ordinary `net/http` middleware is usually the better place.

### 6. Show internal errors during development

`500` responses hide the error text. To see it while developing, set the `httperr` debug flag:

```go
httperr.DebugShowInternalErrorsInResponse = true
```

The body becomes `Internal Server Error` followed by the error, like `unknown unit "K", use C or F`. Don't enable this in production: error text can contain paths, SQL, and private data.

## Verification

Request each case and check the status with `curl -i`:

```bash
curl -i 'localhost:8080/convert?degrees=1&to=K'     # your status for ErrUnknownUnit
curl -i 'localhost:8080/convert?degrees=hot&to=F'   # 400 invalid value for argument "degrees"
curl -i 'localhost:8080/convert?degrees=100&to=F'   # 200 212
```

In tests, use `httptest.NewRecorder` with the handler and assert `Code` and `Body`.

## Troubleshooting

**A response contains two bodies, or `http: superfluous response.WriteHeader call` is logged.**
An error handler wrote a response but returned `false`, so the next handler or `HandleError` wrote another. Return `true` after writing.

**A handler doesn't recognize invalid-argument errors.**
Error handlers get the parse error wrapped in an internal type that carries the 400 response. Match with `errors.AsType[function.ErrParseArgString](err)`, not with a type switch or `==`.

**Form errors still answer 500.**
`htmlform.Handler` ignores error handlers of `httpfun.Handler`. Use the global `httpfun.HandleError` (step 3), an error that implements `http.Handler` (step 4), or a results writer that handles `resultErr` itself.

**A custom `HandleError` answers invalid arguments with 500.**
It writes every error itself. Delegate unhandled errors to `httpfun.DefaultHandleError`.

**An `httperr` error from the function still answers 500.**
The error was wrapped in a way that hides it from `errors.As`, like `fmt.Errorf("...: %v", err)`. Wrap with `%w`.

## Related

- [HTTP status codes reference](reference-http-status-codes.md): every default status and body
- [Security model](explanation-security-model.md): what error text is safe to send
- [httpfun on pkg.go.dev](https://pkg.go.dev/github.com/domonda/go-function/httpfun#ErrorHandler): `ErrorHandler`, `HandleError`, `DefaultHandleError`
- [httperr on pkg.go.dev](https://pkg.go.dev/github.com/ungerik/go-httpx/httperr)
