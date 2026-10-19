# Tutorial: one function, four interfaces

In this tutorial you build `tempconv`, a small program around one Go function that converts temperatures. By the end, that single function answers on the command line (`tempconv convert 100 F`), as a JSON API (`GET /convert?degrees=100&to=F`), and as an HTML form in the browser. go-function parses every argument, reports invalid input, and turns errors into the right HTTP status. You don't write any parsing code.

It takes about 15 minutes.

## What you'll need

- Go 1.26 or later (`go version`)
- `curl` and a web browser
- Basic Go: functions, packages, `go run`

## Step 1: Create the module

Create a directory and a module, and add go-function:

```bash
mkdir tempconv && cd tempconv
go mod init example.com/tempconv
go get github.com/domonda/go-function@latest
```

`go get` adds `github.com/domonda/go-function` to `go.mod`. This tutorial needs v1.0.0 or later.

## Step 2: Wrap the function and call it with strings

Create `main.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/domonda/go-function"
)

// ErrUnknownUnit is returned by Convert for a unit other than C or F.
var ErrUnknownUnit = errors.New("unknown unit")

// Convert converts a temperature to the unit "C" (Celsius) or "F" (Fahrenheit).
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

func main() {
	ctx := context.Background()
	fmt.Println(convert)

	results, err := convert.CallWithStrings(ctx, "100", "F")
	fmt.Println(results, err)

	results, err = convert.CallWithJSON(ctx, []byte(`{"degrees": 212, "to": "C"}`))
	fmt.Println(results, err)

	results, err = convert.CallWithStrings(ctx, "hot", "F")
	fmt.Println(results, err)
}
```

`Convert` is a plain Go function. Nothing in it knows about go-function. `ErrUnknownUnit` will come in handy in step 6.

`function.MustReflectWrapper` wraps it as a `function.Wrapper`. You pass the argument names, because Go's reflection can see argument types but not their names. The `ctx` argument counts too.

Run it:

```bash
go run .
```

```
main.Convert(ctx context.Context, degrees float64, to string) (float64, error)
[212] <nil>
[100] <nil>
[] can't parse string value of argument 'degrees' of function main.Convert(ctx context.Context, degrees float64, to string) (float64, error): can't parse "hot" as float because of: invalid syntax
```

What happened:

- `CallWithStrings` parsed `"100"` into a `float64`, passed the context as the first argument, and returned the results without the error result.
- `CallWithJSON` took the arguments by name from a JSON object.
- `"hot"` isn't a number, so the wrapper returned an error naming the argument, and `Convert` was never called.

## Step 3: Add a command line interface

Add the `cli` module:

```bash
go get github.com/domonda/go-function/cli@latest
```

Replace `main` and add the imports `"os"` and `"github.com/domonda/go-function/cli"`:

```go
func main() {
	dispatcher := cli.NewStringArgsDispatcher("tempconv")
	dispatcher.MustAddCommand("convert", "Convert a temperature to C or F", convert, function.Println)
	dispatcher.MustAddDefaultCommand("Print the commands", function.VoidFuncWrapper(dispatcher.PrintCommandsUsageIntro))

	command, err := dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "tempconv %s: %v\n", command, err)
		os.Exit(1)
	}
}
```

The `convert` command calls the wrapper with the command line arguments. `function.Println` prints its results. The default command runs when no command is given and prints the help. `function.VoidFuncWrapper` turns the `func()` method `PrintCommandsUsageIntro` into a wrapper.

Build and try it:

```bash
go build -o tempconv .
./tempconv convert 100 F
./tempconv convert 212 c
```

```
212
100
```

Invalid input is reported, and the program exits with status 1:

```bash
./tempconv convert hot F
./tempconv convert 100 K
```

```
tempconv convert: can't parse string value of argument 'degrees' of function main.Convert(ctx context.Context, degrees float64, to string) (float64, error): can't parse "hot" as float because of: invalid syntax
tempconv convert: unknown unit "K", use C or F
```

Run it without arguments to see the help, built from the argument names and types:

```bash
./tempconv
```

```
Commands:
  tempconv  
      Print the commands

  tempconv convert <ctx:context.Context> <degrees:float64> <to:string>
      Convert a temperature to C or F
```

The help lists the `ctx` argument too, but you don't type it: the dispatcher passes the context.

## Step 4: Serve it over HTTP

The `httpfun` package is part of the module you already have. Add a `Serve` function and register it as a second command, so `tempconv serve :8080` starts the server. Add the imports `"net/http"` and `"github.com/domonda/go-function/httpfun"`:

```go
// Serve serves Convert over HTTP at addr.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("GET /convert", httpfun.Handler(httpfun.RequestQueryArgs, convert, httpfun.RespondJSON))
	mux.Handle("POST /convert", httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, convert, httpfun.RespondJSON))

	fmt.Println("Listening on", addr)
	return http.ListenAndServe(addr, mux)
}
```

In `main`, register it before the default command:

```go
	dispatcher.MustAddCommand("serve", "Serve the converter over HTTP", function.MustReflectWrapper(Serve, "addr"))
```

`httpfun.Handler` combines three parts: where the arguments come from (`RequestQueryArgs` reads the URL query, `RequestBodyJSONFieldsAsArgs` the fields of a JSON body), the wrapper, and how to write the results (`RespondJSON`). `Serve` itself is a wrapped function, so the CLI parses its `addr` argument the same way.

Build and start the server:

```bash
go build -o tempconv . && ./tempconv serve :8080
```

```
Listening on :8080
```

In a second terminal:

```bash
curl 'localhost:8080/convert?degrees=100&to=F'
curl -X POST localhost:8080/convert -d '{"degrees": 98.6, "to": "C"}'
```

```
212
37
```

An invalid argument gets `400 Bad Request` naming the argument, without echoing the value:

```bash
curl -i 'localhost:8080/convert?degrees=hot&to=F'
```

```
HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
Content-Length: 37

invalid value for argument "degrees"
```

Stop the server with Ctrl+C.

## Step 5: Add an HTML form

Add the `htmlform` module:

```bash
go get github.com/domonda/go-function/htmlform@latest
```

At the start of `Serve`, create a form handler, and register it on the mux. Add the import `"github.com/domonda/go-function/htmlform"`:

```go
func Serve(addr string) error {
	form := htmlform.MustNewHandler(convert, "Temperature converter", httpfun.RespondJSON)
	form.SetArgDefaultValue("degrees", 20)
	form.SetArgOptions("to", []htmlform.Option{
		{Label: "Fahrenheit", Value: "F"},
		{Label: "Celsius", Value: "C"},
	})

	mux := http.NewServeMux()
	mux.Handle("GET /convert", httpfun.Handler(httpfun.RequestQueryArgs, convert, httpfun.RespondJSON))
	mux.Handle("POST /convert", httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, convert, httpfun.RespondJSON))
	mux.Handle("/form", form)

	fmt.Println("Listening on", addr)
	return http.ListenAndServe(addr, mux)
}
```

`htmlform.MustNewHandler` reads the argument names and types from the wrapper: `degrees` gets a number input, and `to` becomes a dropdown because of `SetArgOptions`. The context argument gets no field.

Rebuild, restart the server, and open <http://localhost:8080/form>. Enter `100`, pick Fahrenheit, and submit. The page shows `212`.

The same works from the command line:

```bash
curl -F degrees=100 -F to=F localhost:8080/form
```

```
212
```

The form labels its fields with the argument names. With [generated wrappers](howto-generated-wrappers.md) they use the argument descriptions from the doc comment instead.

## Step 6: Answer an unknown unit with 400

Try a unit that `Convert` rejects:

```bash
curl -i 'localhost:8080/convert?degrees=1&to=K'
```

```
HTTP/1.1 500 Internal Server Error
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
Content-Length: 22

Internal Server Error
```

go-function treats an error returned by the function as a server error, and doesn't show its text, which could contain internal details. But an unknown unit is the client's mistake, so it deserves `400 Bad Request` with the message.

`Convert` wraps `ErrUnknownUnit`, so it can be recognized with `errors.Is`. Add this at the start of `Serve`:

```go
	// Answer ErrUnknownUnit with 400 Bad Request instead of 500
	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) bool {
		if errors.Is(err, ErrUnknownUnit) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		return httpfun.DefaultHandleError(err, w, r)
	}
```

`httpfun.HandleError` answers the errors of every `httpfun.Handler` and `htmlform.Handler`. The new function handles `ErrUnknownUnit` and leaves everything else to `httpfun.DefaultHandleError`, which keeps the 400 for invalid arguments and the 500 for real server errors.

Rebuild, restart, and try again:

```bash
curl -i 'localhost:8080/convert?degrees=1&to=K'
```

```
HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
Content-Length: 29

unknown unit "K", use C or F
```

The JSON endpoint and the form answer the same way:

```bash
curl -X POST localhost:8080/convert -d '{"degrees": 1, "to": "K"}'
curl -F degrees=1 -F to=K localhost:8080/form
```

```
unknown unit "K", use C or F
unknown unit "K", use C or F
```

## What you built

One function, `Convert`, now runs as:

| Interface | How it's called | Built with |
|---|---|---|
| Go | `convert.CallWithStrings(ctx, "100", "F")` | `function.MustReflectWrapper` |
| CLI | `tempconv convert 100 F` | `cli.StringArgsDispatcher` |
| JSON API | `GET /convert?degrees=100&to=F`, `POST /convert` with a JSON object | `httpfun.Handler` |
| Web form | <http://localhost:8080/form> | `htmlform.Handler` |

Argument parsing, the help text, the form fields, and the 400 responses all come from the function's signature. Adding a second function takes one wrapper and one line per interface.

The complete `main.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/cli"
	"github.com/domonda/go-function/htmlform"
	"github.com/domonda/go-function/httpfun"
)

// ErrUnknownUnit is returned by Convert for a unit other than C or F.
var ErrUnknownUnit = errors.New("unknown unit")

// Convert converts a temperature to the unit "C" (Celsius) or "F" (Fahrenheit).
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

// Serve serves Convert over HTTP at addr.
func Serve(addr string) error {
	// Answer ErrUnknownUnit with 400 Bad Request instead of 500
	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) bool {
		if errors.Is(err, ErrUnknownUnit) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		return httpfun.DefaultHandleError(err, w, r)
	}

	form := htmlform.MustNewHandler(convert, "Temperature converter", httpfun.RespondJSON)
	form.SetArgDefaultValue("degrees", 20)
	form.SetArgOptions("to", []htmlform.Option{
		{Label: "Fahrenheit", Value: "F"},
		{Label: "Celsius", Value: "C"},
	})

	mux := http.NewServeMux()
	mux.Handle("GET /convert", httpfun.Handler(httpfun.RequestQueryArgs, convert, httpfun.RespondJSON))
	mux.Handle("POST /convert", httpfun.Handler(httpfun.RequestBodyJSONFieldsAsArgs, convert, httpfun.RespondJSON))
	mux.Handle("/form", form)

	fmt.Println("Listening on", addr)
	return http.ListenAndServe(addr, mux)
}

func main() {
	dispatcher := cli.NewStringArgsDispatcher("tempconv")
	dispatcher.MustAddCommand("convert", "Convert a temperature to C or F", convert, function.Println)
	dispatcher.MustAddCommand("serve", "Serve the converter over HTTP", function.MustReflectWrapper(Serve, "addr"))
	dispatcher.MustAddDefaultCommand("Print the commands", function.VoidFuncWrapper(dispatcher.PrintCommandsUsageIntro))

	command, err := dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "tempconv %s: %v\n", command, err)
		os.Exit(1)
	}
}
```

## Next steps

- [How to switch to generated wrappers](howto-generated-wrappers.md): argument names and descriptions from your source, no reflection, and a CI check
- [How to make your own types scannable from strings](howto-custom-string-types.md): accept your own types as arguments everywhere
- [How to customize HTTP error responses](howto-http-errors.md): per-endpoint error handlers, statuses chosen by the function, logging
- [Security model](explanation-security-model.md): read this before exposing functions to the internet
- [Architecture](explanation-architecture.md): how wrappers and adapters fit together
