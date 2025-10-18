# go-function

[![Go Reference](https://pkg.go.dev/badge/github.com/domonda/go-function.svg)](https://pkg.go.dev/github.com/domonda/go-function)
[![Go Report Card](https://goreportcard.com/badge/github.com/domonda/go-function)](https://goreportcard.com/report/github.com/domonda/go-function)

**Wrapping Go functions as HTTP handlers, CLI commands, and more**

`go-function` is a Go library that provides utilities for wrapping any Go function to enable multiple calling conventions: direct calls with typed arguments, string-based calls, JSON calls, HTTP handlers, and CLI commands.

## Features

- **Multiple Calling Conventions**: Call functions with `[]any`, strings, named strings, or JSON
- **Automatic Type Conversion**: Convert strings to the correct types automatically
- **HTTP Integration**: Turn functions into HTTP handlers with `httpfun` package
- **CLI Integration**: Build command-line interfaces with `cli` package
- **HTML Forms**: Generate HTML forms from function signatures with `htmlform` package
- **Code Generation**: Generate zero-overhead wrappers with `gen-func-wrappers`
- **Reflection Fallback**: Use reflection-based wrappers when code generation isn't needed
- **Context Support**: Automatic `context.Context` handling
- **Error Handling**: Proper error propagation and panic recovery

## Installation

```bash
go get github.com/domonda/go-function
```

## Quick Start

### Basic Function Wrapping

```go
package main

import (
    "context"
    "fmt"
    "github.com/domonda/go-function"
)

func Add(a, b int) int {
    return a + b
}

func Greet(ctx context.Context, name string) (string, error) {
    return fmt.Sprintf("Hello, %s!", name), nil
}

func main() {
    // Wrap functions using reflection
    addWrapper := function.MustReflectWrapper("Add", Add)
    greetWrapper := function.MustReflectWrapper("Greet", Greet)

    ctx := context.Background()

    // Call with string arguments
    results, _ := addWrapper.CallWithStrings(ctx, "5", "3")
    fmt.Println(results[0]) // Output: 8

    // Call with named arguments
    results, _ := greetWrapper.CallWithNamedStrings(ctx, map[string]string{
        "name": "World",
    })
    fmt.Println(results[0]) // Output: Hello, World!

    // Call with JSON
    results, _ := addWrapper.CallWithJSON(ctx, []byte(`[10, 20]`))
    fmt.Println(results[0]) // Output: 30
}
```

### HTTP Handler

```go
package main

import (
    "context"
    "net/http"
    "github.com/domonda/go-function/httpfun"
)

func Calculate(ctx context.Context, operation string, a, b int) (int, error) {
    switch operation {
    case "add":
        return a + b, nil
    case "multiply":
        return a * b, nil
    default:
        return 0, fmt.Errorf("unknown operation: %s", operation)
    }
}

func main() {
    // Create HTTP handler from function
    handler := httpfun.NewHandler(
        function.MustReflectWrapper("Calculate", Calculate),
        nil, // Use default result writer
    )

    http.Handle("/calculate", handler)
    http.ListenAndServe(":8080", nil)

    // Usage:
    // GET /calculate?operation=add&a=5&b=3 -> 8
    // POST /calculate with JSON: {"operation":"multiply","a":5,"b":3} -> 15
}
```

### CLI Command

```go
package main

import (
    "context"
    "fmt"
    "os"
    "github.com/domonda/go-function"
    "github.com/domonda/go-function/cli"
)

func Deploy(env, service string, version int) error {
    fmt.Printf("Deploying %s v%d to %s\n", service, version, env)
    return nil
}

func main() {
    dispatcher := cli.NewStringArgsDispatcher(context.Background())
    dispatcher.MustAddCommand("", function.MustReflectWrapper("deploy", Deploy))

    err := dispatcher.Dispatch(os.Args[1:]...)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

// Usage:
// $ myapp deploy production api-server 42
// Deploying api-server v42 to production
```

## String to Type Conversion

The library automatically converts strings to the required types:

```go
// Basic types
var i int
function.ScanString("42", &i) // i = 42

var f float64
function.ScanString("3.14", &f) // f = 3.14

var b bool
function.ScanString("true", &b) // b = true

// Time types
var t time.Time
function.ScanString("2024-01-15", &t)

var d time.Duration
function.ScanString("5m30s", &d) // d = 5*time.Minute + 30*time.Second

// Slices
var nums []int
function.ScanString("[1,2,3]", &nums) // nums = []int{1,2,3}

// Structs (as JSON)
type Person struct {
    Name string
    Age  int
}
var p Person
function.ScanString(`{"Name":"Alice","Age":30}`, &p)

// Nil values
var ptr *int
function.ScanString("nil", &ptr) // ptr = nil
```

## Code Generation for Better Performance

For production use, generate optimized wrappers without reflection overhead:

```go
// In your code, use placeholder functions
package myapp

import "github.com/domonda/go-function"

func Calculate(a, b int) int {
    return a + b
}

// Use WrapperTODO during development
var CalculateWrapper = function.WrapperTODO(Calculate)
```

Then run the code generator:

```bash
go run github.com/domonda/go-function/cmd/gen-func-wrappers
```

This replaces `WrapperTODO` calls with generated, type-safe wrapper code that has zero reflection overhead.

## Supported Function Signatures

The library supports various function signatures:

```go
// No arguments or results
func SimpleFunc()

// With context
func WithContext(ctx context.Context, arg string) error

// Multiple arguments and results
func Complex(ctx context.Context, a int, b string) (result int, err error)

// Variadic functions (limited support)
func Variadic(args ...string) string
```

**Requirements:**
- If present, `context.Context` must be the first argument
- If present, `error` must be the last result
- All arguments and results must be supported types (or implement `encoding.TextUnmarshaler` / `json.Unmarshaler`)

## Packages

### Core Package (`function`)

- `Wrapper` - Main interface for wrapped functions
- `Description` - Function metadata (name, arguments, types)
- `StringScanner` - String-to-type conversion
- `ReflectWrapper` - Reflection-based wrapper
- `WrapperTODO` - Placeholder for code generation

### HTTP Package (`httpfun`)

- `NewHandler` - Create HTTP handler from function
- `HandlerWithoutContext` - HTTP handler without context arg
- `RequestArgs` - Parse function arguments from HTTP requests
- `ResultsWriter` - Write function results as HTTP responses

### CLI Package (`cli`)

- `StringArgsDispatcher` - Command dispatcher for CLI apps
- `SuperStringArgsDispatcher` - Multi-level command dispatcher
- `Complete` - Shell completion support

### HTML Forms Package (`htmlform`)

- `NewHandler` - Generate HTML forms for functions
- Form generation with validation
- Customizable templates

### Code Generator (`cmd/gen-func-wrappers`)

- Generates optimized wrapper code
- Zero reflection overhead
- Preserves type safety
- Supports custom import prefixes

## Configuration

### String Scanners

Customize type conversion:

```go
import "github.com/domonda/go-function"

// Add custom scanner for a specific type
function.StringScanners.SetForType(
    reflect.TypeOf(MyCustomType{}),
    function.StringScannerFunc(func(src string, dest any) error {
        // Custom conversion logic
        return nil
    }),
)

// Configure time formats
function.TimeFormats = []string{
    time.RFC3339,
    "2006-01-02",
    "2006-01-02 15:04",
}
```

### HTTP Configuration

```go
import "github.com/domonda/go-function/httpfun"

// Pretty-print JSON responses
httpfun.PrettyPrint = true
httpfun.PrettyPrintIndent = "  "

// Custom error handler
httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) {
    // Custom error handling
}

// Panic recovery
httpfun.CatchHandlerPanics = true
```

## Advanced Examples

### Custom Result Handler

```go
// Custom handler that always returns uppercase strings
customHandler := httpfun.ResultsWriterFunc(func(results []any, w http.ResponseWriter, r *http.Request) error {
    if len(results) > 0 {
        if str, ok := results[0].(string); ok {
            w.Write([]byte(strings.ToUpper(str)))
            return nil
        }
    }
    return httpfun.DefaultResultsWriter.WriteResults(results, w, r)
})

handler := httpfun.NewHandler(wrapper, customHandler)
```

### Multi-level CLI Commands

```go
// Build CLI with subcommands: app user create, app user delete, etc.
dispatcher := cli.NewSuperStringArgsDispatcher(context.Background())

userDispatcher := dispatcher.AddSubDispatcher("user")
userDispatcher.MustAddCommand("create", function.MustReflectWrapper("CreateUser", CreateUser))
userDispatcher.MustAddCommand("delete", function.MustReflectWrapper("DeleteUser", DeleteUser))

dbDispatcher := dispatcher.AddSubDispatcher("db")
dbDispatcher.MustAddCommand("migrate", function.MustReflectWrapper("Migrate", Migrate))
dbDispatcher.MustAddCommand("seed", function.MustReflectWrapper("Seed", Seed))

dispatcher.Dispatch(os.Args[1:]...)
```

## Testing

The library includes comprehensive tests. Run them with:

```bash
go test ./...
```

## Contributing

Contributions are welcome! Please:

1. Add tests for new features
2. Update documentation
3. Follow existing code style
4. Ensure `go test ./...` passes

## License

MIT License - see LICENSE file for details

## Related Projects

- [go-errs](https://github.com/domonda/go-errs) - Error wrapping used by this library
- [go-types](https://github.com/domonda/go-types) - Type definitions with string scanning support
- [go-astvisit](https://github.com/ungerik/go-astvisit) - AST manipulation utilities used by the code generator

