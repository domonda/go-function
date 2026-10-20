# Security model

go-function turns text from outside your program into typed Go values and calls your function with them. That is the whole point, and it is also where the risk lies: whoever controls the text controls the arguments. This page explains where the trust boundary is, what go-function defends against on its own, and what stays your job.

## The trust boundary

```
 untrusted input                go-function                          your function
 ───────────────                ───────────                          ─────────────
 HTTP query, body, form ──► RequestArgsFunc ──► ScanString / JSON ──► typed arguments
 CLI arguments          ──► dispatcher      ──► ScanString        ──► typed arguments
                                      │
                                      └── limits, 400 / 413 / 403 responses,
                                          error text without values
```

Every string or JSON value that reaches a wrapper becomes an argument value **as it is**. The wrapper checks that the value parses as the argument type, nothing more. A string that parses as an `int` is a valid `int`, whether it is `1` or `-9223372036854775808`. Range checks, permission checks, and business rules belong in the function or in `htmlform` validators.

Command-line arguments usually come from the user who runs the program and already has its privileges, so the CLI is rarely a boundary. HTTP requests and form submissions are.

## Types that name resources

Some types hold a reference to something on the server instead of data. `fs.File` of [go-fs](https://github.com/ungerik/go-fs) is a path string. A function like this is harmless when your own code calls it:

```go
func Summarize(ctx context.Context, document fs.File) (string, error)
```

Exposed through `httpfun.Handler` with `RequestQueryArgs`, `?document=/etc/passwd` scans into an `fs.File` that names the server's password file, and the function reads it for the client. The same happens through `CallWithJSON` with `{"document": "/etc/passwd"}`.

go-function can't tell a path type from any other string type, so it doesn't block them in `httpfun` or `CallWithJSON`. The defenses:

- **`htmlform` takes file arguments only from uploads.** An argument whose type implements `fs.FileReader`, or a pointer, slice, or array of such a type, is filled only with files uploaded with the form. A non-empty text value for it is rejected with 400 Bad Request before the function runs.
- **Don't expose path-like argument types to untrusted callers** through `httpfun`, `CallWithJSON`, or `CallWithStrings`. Take the content instead: `[]byte`, a `string`, or a type like `fs.MemFile` that carries name and data.
- **Don't register a process-wide scanner** that converts strings into `fs.FileReader` or other resource handles. It would apply to every string-based transport at once.
- **Generator JSON replacements:** `gen-func-wrappers -replaceForJSON=fs.FileReader:fs.File` makes `CallWithJSON` unmarshal JSON strings into server paths. Use a content type like `fs.MemFile` for wrappers that get JSON from clients.

`htmlform` only protects arguments that are files or pointers, slices, or arrays of files. A struct argument with an `fs.File` field is entered as JSON text, and its file field becomes a server path. Don't use such struct arguments in forms for untrusted users.

## Who decides which argument

`httpfun.RequestQueryArgs` and `RequestBodyJSONFieldsAsArgs` pass every parameter the client sends. Any argument of the function can be set that way, also one you meant to fill from somewhere else.

`MergeRequestArgs` lets later sources overwrite earlier ones. Put defaults first and trusted values last, so the client can't override them:

```go
httpfun.MergeRequestArgs(
	httpfun.ConstRequestArg("limit", "20"),           // default, the client may override it
	httpfun.RequestQueryArgs,                         // client input
	httpfun.RequestHeaderAsArg("X-Tenant", "tenant"), // set by your proxy, which drops client values
)
```

This works because the header getters always set their argument, to an empty string if the header is missing. If the proxy didn't set `X-Tenant`, the client's `?tenant=` is still overwritten. The query getters `RequestQueryArg` and `RequestQueryAsArg` leave a missing parameter out instead, so a default placed before them is kept.

Better still, don't make identity an argument at all. `httpfun.Handler` and `htmlform.Handler` call the function with the request's context, so authentication middleware can store the user in the context and the function reads it from `ctx`. An argument that doesn't exist can't be forged.

## Resource limits

Untrusted input can be large on purpose. go-function caps the input it reads:

| Input | Limit | Default | Over the limit |
|---|---|---|---|
| body read by `RequestBodyAsArg`, `RequestBodyJSONFieldsAsArgs` | `httpfun.MaxRequestBodySize` | 32 MiB | 413 |
| body read by `RequestMultipartFormArgs` | `httpfun.MaxMultipartFormSize` | 32 MiB | 413 |
| form submission of `htmlform.Handler`, including uploads | `Handler.SetMaxRequestBodySize` | 100 MiB | 413 |
| upload data kept in memory, the rest goes to temporary files | `httpfun.MaxMultipartFormMemory` | 1 MiB | |
| elements of one slice or array literal in `ScanString` | `function.MaxSliceLiteralElements` | 1,048,576 | error before allocating |

Temporary upload files are removed after the response, also for requests that a middleware cloned, which `net/http` alone doesn't clean up.

go-function doesn't set timeouts. Configure `ReadTimeout`, `ReadHeaderTimeout`, and `WriteTimeout` on your `http.Server`, so slow clients can't hold connections open.

## Cross-site request forgery

A browser sends a user's cookies with requests that other websites trigger. Without protection, a malicious page can submit a form to your server in the user's name.

- **`htmlform.Handler`** checks every POST with `http.CrossOriginProtection` by default. A POST that a browser sends from another origin, detected with the `Sec-Fetch-Site` or `Origin` headers, gets 403 Forbidden. Requests without these headers, like from `curl` or other servers, are accepted. Use `SetCrossOriginProtection` to trust more origins, or pass `nil` for a handler that only non-browser clients call with credentials browsers don't send automatically.
- **`httpfun.Handler` has no CSRF protection.** If browsers with cookies call your endpoints, wrap them:

  ```go
  protection := http.NewCrossOriginProtection()
  mux.Handle("POST /convert", protection.Handler(httpfun.Handler(...)))
  ```

  `RequestBodyJSONFieldsAsArgs` doesn't require a JSON content type, so a cross-site `text/plain` form post can deliver a JSON body. And keep GET handlers free of side effects: `http.CrossOriginProtection` always allows GET.

## What reaches the client

Error text often contains details an attacker can use: file paths, SQL, internal addresses, or the private data that failed to parse. go-function keeps it out of responses:

- 400 responses for invalid arguments name the argument but not its value or the parse error: `invalid value for argument "degrees"`.
- 500 responses say `Internal Server Error` and nothing else, unless `httperr.DebugShowInternalErrorsInResponse` is true. Leave it false in production.
- `ErrParseArgString`, `ErrParseArgJSON`, and `ErrParseArgsJSON` don't add the value or the JSON input to their `Error()` text. The scanner error they wrap usually quotes the value once, like `can't parse "hot" as float`, so their text belongs in logs, not in responses.
- `htmlform` answers a broken multipart body with `invalid multipart form`, because the parser's error can contain the server's network address.

Exceptions you control:

- **`htmlform` validator errors are shown to the client** as `invalid argument "email": <your error text>`. Write messages for users, without internal details.
- **Custom error handlers decide what they write.** A `HandleError` or `ErrorHandler` that writes `err.Error()` shows internal details again. Log the error and delegate to `httpfun.DefaultHandleError`, or write a fixed message.
- **`httpfun.RespondHTML` doesn't escape results.** A function whose result contains user input must return escaped HTML, like from `html/template`. The form title and field labels of `htmlform` are escaped by its template.

## Panics

Without recovery, `net/http` logs a handler panic with its stack trace and aborts the connection, so the client gets no response at all. `httpfun.Handler` recovers panics when `httpfun.CatchHandlerPanics` is true (the default) and answers 500 without the panic message. `htmlform.Handler` always recovers. Both let a panic with `http.ErrAbortHandler` through, which aborts the response like without recovery. Keep `CatchHandlerPanics` on in production.

## Checklist

Before exposing a function over HTTP:

- [ ] No argument type names a file, URL, or other server resource, or the handler only gets trusted input.
- [ ] Arguments the client must not choose come from the context or a trusted source placed last in `MergeRequestArgs`.
- [ ] The function checks ranges and permissions of its arguments, or `htmlform` validators do.
- [ ] Body limits fit the endpoint, and the `http.Server` has timeouts.
- [ ] Endpoints that browsers call with cookies have cross-origin protection, and GET handlers change nothing.
- [ ] `httperr.DebugShowInternalErrorsInResponse` is false, and custom error handlers don't write `err.Error()`.
- [ ] HTML results escape user input.

Open issues found by reviews are listed in [TODOS.md](../TODOS.md).

## Related

- [HTTP status codes reference](reference-http-status-codes.md)
- [How to customize HTTP error responses](howto-http-errors.md)
- [Architecture](explanation-architecture.md)
- [gen-func-wrappers JSON type replacements](../cmd/gen-func-wrappers/README.md#json-type-replacements)
