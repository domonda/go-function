# gen-func-wrappers

[![Go Reference](https://pkg.go.dev/badge/github.com/domonda/go-function/cmd/gen-func-wrappers.svg)](https://pkg.go.dev/github.com/domonda/go-function/cmd/gen-func-wrappers)

A code generator that creates zero-overhead wrapper implementations for Go functions, enabling them to be called with multiple calling conventions (typed arguments, strings, JSON, etc.) without runtime reflection.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Step 1: Write Your Function](#step-1-write-your-function)
  - [Step 2: Declare a Wrapper](#step-2-declare-a-wrapper)
  - [Step 3: Add go:generate Directive](#step-3-add-gogenerate-directive)
  - [Step 4: Run Code Generation](#step-4-run-code-generation)
  - [Step 5: Use the Generated Wrapper](#step-5-use-the-generated-wrapper)
- [Calling Conventions](#calling-conventions)
- [Command Line Options](#command-line-options)
- [JSON Type Replacements](#json-type-replacements)
- [Advanced Usage](#advanced-usage)
- [How It Works](#how-it-works)

## Installation

Add the generator as tool to the `go.mod` file of your module, so that `go tool gen-func-wrappers` used below runs it:

```sh
go get -tool github.com/domonda/go-function/cmd/gen-func-wrappers@latest
```

## Quick Start

Here's a complete example from declaration to usage:

```go
package myapp

import (
    "context"
    "fmt"

    "github.com/domonda/go-function"
)

//go:generate go tool gen-func-wrappers $GOFILE

// GreetUser creates a personalized greeting.
//   name: The user's full name
//   formal: Whether to use formal language
func GreetUser(ctx context.Context, name string, formal bool) (string, error) {
    if name == "" {
        return "", fmt.Errorf("name cannot be empty")
    }
    if formal {
        return "Good day, " + name, nil
    }
    return "Hi, " + name + "!", nil
}

// Declare the wrapper using WrapperTODO
var greetUserWrapper = function.WrapperTODO(GreetUser)
```

Run generation:

```sh
go generate ./...
```

The `WrapperTODO` declaration is replaced with a fully generated wrapper implementation that you can use:

```go
// Call with typed arguments
results, err := greetUserWrapper.Call(ctx, []any{"Alice", false})
// results[0] == "Hi, Alice!"

// Call with string arguments (useful for CLI)
results, err = greetUserWrapper.CallWithStrings(ctx, "Bob", "true")
// results[0] == "Good day, Bob"

// Call with named strings (useful for HTTP query params)
results, err = greetUserWrapper.CallWithNamedStrings(ctx, map[string]string{
    "name":   "Charlie",
    "formal": "false",
})

// Call with JSON (useful for HTTP POST bodies)
results, err = greetUserWrapper.CallWithJSON(ctx, []byte(`{"name":"Diana","formal":true}`))
```

## Usage

### Step 1: Write Your Function

Write any regular Go function with clear documentation. Document arguments with lines of the doc comment that start with `argName:` followed by the description, optionally as list items:

```go
// ProcessOrder processes a customer order.
//   orderID: The unique order identifier
//   quantity: Number of items to order
//   express: Whether to use express shipping
func ProcessOrder(ctx context.Context, orderID string, quantity int, express bool) error {
    // ... implementation
}

// CancelOrder cancels a customer order.
//
// Arguments:
//   - orderID: The unique order identifier
//   - reason: Why the order is cancelled
func CancelOrder(ctx context.Context, orderID, reason string) error {
    // ... implementation
}
```

The description is the rest of the line. Leading spaces and tabs are ignored, so the descriptions are still found after gofmt reformatted the indented lines with a tab. Unnamed arguments get the names `arg0`, `arg1`, ... by their position.

### Step 2: Declare a Wrapper

Use `function.WrapperTODO()` to declare a wrapper variable:

```go
var processOrderWrapper = function.WrapperTODO(ProcessOrder)
```

Exported functions of imported packages, including the standard library, can be wrapped too, like `function.WrapperTODO(strings.ToUpper)`.

**Important:** The variable name should follow the convention `{functionName}Wrapper` (camelCase), but this is not strictly required.

The other TODO functions generate wrappers for single interfaces: `function.CallWrapperTODO`, `function.CallWithStringsWrapperTODO`, `function.CallWithNamedStringsWrapperTODO`, and `function.CallWithJSONWrapperTODO`. TODO calls are also found in `var ( ... )` blocks, where the generated code is inserted after the block, and with the name your file uses for go-function, like `fn.WrapperTODO` for an import with the name `fn` or `WrapperTODO` for a dot import. Declare every wrapper variable separately, `var a, b = function.WrapperTODO(A), function.WrapperTODO(B)` is an error.

A generated wrapper type without var, like `type ProcessOrderWrapper struct{}` with the comment `// ProcessOrderWrapper wraps ProcessOrder as function.Wrapper (generated code)`, is regenerated as type that you instantiate with `ProcessOrderWrapper{}`.

### Step 3: Add go:generate Directive

Add the generation directive to your file (typically at the top after package declaration):

```go
//go:generate go tool gen-func-wrappers $GOFILE
```

Or to process an entire directory:

```go
//go:generate go tool gen-func-wrappers .
```

### Step 4: Run Code Generation

Execute the code generator:

```sh
go generate ./...
```

Or manually:

```sh
go tool gen-func-wrappers myfile.go
```

The generator will replace your `WrapperTODO` declaration with generated code like:

```go
// processOrderWrapper wraps ProcessOrder as function.Wrapper (generated code)
var processOrderWrapper processOrderWrapperT

// processOrderWrapperT wraps ProcessOrder as function.Wrapper (generated code)
type processOrderWrapperT struct{}

func (processOrderWrapperT) Name() string {
    return "myapp.ProcessOrder"
}

func (processOrderWrapperT) NumArgs() int { return 4 }

func (f processOrderWrapperT) Call(ctx context.Context, args []any) (results []any, err error) {
    if len(args) > 3 {
        return nil, fmt.Errorf("%d arguments passed, but function %s takes 3", len(args), f)
    }
    var a struct {
        orderID  string
        quantity int
        express  bool
    }
    if 0 < len(args) && args[0] != nil {
        v, ok := args[0].(string)
        if !ok {
            return nil, fmt.Errorf("can't use %T as type string for argument 'orderID' of function %s", args[0], f)
        }
        a.orderID = v
    }
    // ... the same for quantity and express
    err = ProcessOrder(ctx, a.orderID, a.quantity, a.express) // wrapped call
    return results, err
}

// ... additional methods for CallWithStrings, CallWithNamedStrings, CallWithJSON, etc.
```

### Step 5: Use the Generated Wrapper

The generated wrapper implements the `function.Wrapper` interface and provides multiple calling conventions:

```go
ctx := context.Background()

// Typed call
results, err := processOrderWrapper.Call(ctx, []any{"ORD-123", 5, true})

// String arguments (auto-parsed)
results, err = processOrderWrapper.CallWithStrings(ctx, "ORD-456", "10", "false")

// Named string arguments
results, err = processOrderWrapper.CallWithNamedStrings(ctx, map[string]string{
    "orderID":  "ORD-789",
    "quantity": "3",
    "express":  "true",
})

// JSON arguments
results, err = processOrderWrapper.CallWithJSON(ctx, []byte(`{
    "orderID": "ORD-999",
    "quantity": 7,
    "express": false
}`))
```

## Calling Conventions

Generated wrappers support multiple calling conventions:

### 1. Call - Typed Arguments

```go
func (w Wrapper) Call(ctx context.Context, args []any) ([]any, error)
```

Accepts typed arguments as `[]any`. Fast but requires type assertions. Like `function.ReflectWrapper`, missing arguments and untyped `nil` are passed as zero values, a value assignable to the argument type is accepted, like a `[]int` for an argument of type `MyInts` with the underlying type `[]int`, while too many arguments or an argument of the wrong type return an error. Reflection is only used to check the assignability if the type assertion fails, and not for predeclared types like `int` whose values must have the exact type.

### 2. CallWithStrings - Positional String Arguments

```go
func (w Wrapper) CallWithStrings(ctx context.Context, strs ...string) ([]any, error)
```

Arguments are parsed from strings in order. Perfect for CLI applications. Missing strings are passed as zero values, more strings than arguments are an error. Like in a Go call, every string from the position of a variadic last argument on is parsed as one of its elements, also a single string, so `"[1,2,3]"` is one element and not a slice. Arguments and variadic elements of type `any` get the unparsed string, so `Printf(format string, args ...any)` called with `"%v"` and `"hello"` gets the element `"hello"`.

### 3. CallWithNamedStrings - Named String Arguments

```go
func (w Wrapper) CallWithNamedStrings(ctx context.Context, strs map[string]string) ([]any, error)
```

Arguments are parsed by name. Perfect for HTTP query parameters. The single string of a variadic argument is parsed as slice like `[1,2,3]`. Arguments of type `any` get the unparsed string.

### 4. CallWithJSON - JSON Arguments

```go
func (w Wrapper) CallWithJSON(ctx context.Context, argsJSON []byte) ([]any, error)
```

Arguments are unmarshaled from the values of a JSON object. Perfect for REST APIs. Like `function.ReflectWrapper`, the keys are the exact argument names, like `orderID`. Keys that only differ in case are ignored, so `{"amount":1,"AMOUNT":1000000}` passes `1`, and arguments without key are zero values. An argument of type `error` is unmarshaled from its message string. Invalid JSON, or JSON that is no object like an array, returns a `function.ErrParseArgsJSON`, also for a function without arguments, which is then not called. JSON `null` is handled like an empty object. A value that can't be unmarshaled into its argument returns a `function.ErrParseArgJSON` with the argument name.

### Wrapper Metadata

All wrappers provide metadata methods:

```go
wrapper.Name()              // "myapp.ProcessOrder"
wrapper.NumArgs()           // 4
wrapper.ContextArg()        // true (if first arg is context.Context)
wrapper.Variadic()          // false (true if last arg is variadic like ...int)
wrapper.ErrorResult()       // true (if last result is error)
wrapper.NumResults()        // 1
wrapper.ArgNames()          // ["ctx", "orderID", "quantity", "express"]
wrapper.ArgDescriptions()   // ["", "The unique order identifier", ...]
wrapper.ArgTypes()          // []reflect.Type{...}
wrapper.ResultTypes()       // []reflect.Type{...}
```

## Command Line Options

```sh
go tool gen-func-wrappers [options] [paths...]
```

Every path can be a file, a directory, or a directory followed by `/...` to process its sub-directories recursively. Like the go command, the recursion skips directories starting with `.` or `_`, `testdata`, `vendor`, and nested modules with their own `go.mod` file. Without paths the current directory is processed. All paths are processed and the exit code reports the failure of any path.

### Options

- `-verbose` - Print detailed information about processing
- `-print` - Print generated code to stdout instead of modifying files
- `-validate` - Check for missing or outdated wrappers without modifying files, exits with code 1 if any are found, and with code 2 for any other error, like a file that doesn't parse
- `-replaceForJSON=interface:concrete` - Replace interface types with concrete types for JSON unmarshaling, comma separated and repeatable
- `-localImports=prefixes` - Comma separated import path prefixes that are grouped as "local" imports, detected from the `go.mod` file of every path if not specified
- `-help` - Print help information and exit

### Examples

Process a single file:

```sh
go tool gen-func-wrappers myfile.go
```

Process a directory:

```sh
go tool gen-func-wrappers ./pkg/handlers
```

Process recursively:

```sh
go tool gen-func-wrappers ./pkg/handlers/...
```

Process several paths:

```sh
go tool gen-func-wrappers ./pkg/handlers/... ./cmd/server/main.go
```

Check in CI that all wrappers are up to date:

```sh
go tool gen-func-wrappers -validate ./...
```

Print output without modifying files:

```sh
go tool gen-func-wrappers -print myfile.go
```

With JSON type replacements, see [JSON Type Replacements](#json-type-replacements):

```sh
go tool gen-func-wrappers -replaceForJSON=io.Reader:*bytes.Buffer myfile.go
```

## JSON Type Replacements

When a function uses interface types that need special handling during JSON unmarshaling, use the `-replaceForJSON` flag:

```sh
go tool gen-func-wrappers -replaceForJSON=io.Reader:*bytes.Buffer myfile.go
```

This tells the generator that when unmarshaling JSON for arguments of type `io.Reader`, it should use `*bytes.Buffer` as the concrete type. The unmarshaled value is assigned to the argument, an argument without key or with the value `null` stays `nil`, so the function never gets a nil `*bytes.Buffer` in a non-nil `io.Reader`. A variadic argument `...io.Reader` is unmarshaled from a JSON array whose elements are unmarshaled as `*bytes.Buffer`, `null` elements stay `nil`. The replacement type has to unmarshal the JSON that callers send: `*bytes.Buffer` only shows the syntax, it unmarshals an empty buffer from `{}` and no content, while a type like `fs.MemFile` of [go-fs](https://github.com/ungerik/go-fs) unmarshals the name and content of a file.

The types are written like in the file that gets the generated code. The package of the replacement type is resolved by the imports of that file, else by the imports of the file of the wrapped function, whose import is then added. A package that neither file imports is an error.

**Warning:** The replacement type decides what the JSON of a caller can do. A path-like type makes the JSON name files of the server: with `-replaceForJSON=fs.FileReader:fs.File`, `fs.File` of [go-fs](https://github.com/ungerik/go-fs) being a local file path or URI string, `CallWithJSON` unmarshals a JSON string like `"/etc/passwd"` into a file of the server that the wrapped function can then read. Only use such replacements for wrappers that are called with trusted JSON. For JSON from clients, like HTTP request bodies, use a type that carries the content, like `fs.MemFile` with the file name and base64 encoded data.

Multiple replacements, by repeating the flag or separated by commas:

```sh
go tool gen-func-wrappers \
  -replaceForJSON=io.Reader:*bytes.Buffer \
  -replaceForJSON=fs.FileReader:fs.MemFile \
  myfile.go

go tool gen-func-wrappers -replaceForJSON=io.Reader:*bytes.Buffer,fs.FileReader:fs.MemFile myfile.go
```

## Advanced Usage

### Integration with HTTP Handlers

`httpfun.Handler` of `github.com/domonda/go-function/httpfun` calls the wrapper with the arguments of the request:

```go
// GET /greet?name=Alice&formal=true
http.Handle("/greet", httpfun.Handler(httpfun.RequestQueryArgs, greetUserWrapper, httpfun.RespondPlaintext))
```

It answers an invalid argument value with 400 Bad Request naming the argument, and other errors with `httpfun.HandleError`, without showing the text of internal errors to the client. Prefer it over calling the wrapper in your own handler and answering with `http.Error(w, err.Error(), ...)`, which shows details of the server, like the parse errors and the errors of the wrapped function.

### Integration with CLI

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/domonda/go-function"
    "github.com/domonda/go-function/cli"
)

func main() {
    dispatcher := cli.NewStringArgsDispatcher("myapp")
    // function.Println prints the results of the command
    dispatcher.MustAddCommand("greet", "Greet a user", greetUserWrapper, function.Println)

    command, err := dispatcher.DispatchCombinedCommandAndArgs(context.Background(), os.Args[1:])
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error in %s: %v\n", command, err)
        os.Exit(1)
    }
}

// $ myapp greet Alice false
// Hi, Alice!
```

## How It Works

1. **Parse Source Code:** The generator parses Go source files to find `function.WrapperTODO()` declarations
2. **Find Function Declarations:** It locates the actual function declaration (in same package or imports)
3. **Extract Metadata:** Extracts function signature, argument names/types, result types, and documentation
4. **Generate Wrapper Type:** Creates a struct type that implements `function.Wrapper` interface
5. **Generate Methods:** Creates all required methods (`Call`, `CallWithStrings`, etc.) with direct function calls
6. **Replace Code:** Replaces the `WrapperTODO` with generated implementation
7. **Format Output:** Formats the code, adds necessary imports, and removes the imports that the regenerated code doesn't use anymore (except blank imports, dot imports, and `"C"`)

### Generated vs Reflection

**Generated wrappers** (this tool):
- ✅ Zero runtime overhead (compiled to direct function calls)
- ✅ Type-safe at compile time
- ✅ Fast - no reflection at runtime for arguments of the exact types
- ✅ Best for known functions
- ❌ Requires code generation step

**Reflection wrappers** (`function.ReflectWrapper`):
- ✅ No code generation needed
- ✅ Works with dynamic functions
- ✅ Great for plugins/runtime loading
- ❌ Runtime reflection overhead
- ❌ Slower performance

Use generated wrappers when:
- Performance matters
- Functions are known at compile time
- You control the build process

Use reflection wrappers when:
- Functions are loaded dynamically
- Building plugin systems
- Rapid prototyping

## Testing

The internal `gen` package includes `RewriteAstFileSource` for in-memory testing of the generator without disk I/O:

```go
source := []byte(`package testpkg

import "github.com/domonda/go-function"

func SimpleAdd(a, b int) int { return a + b }
var simpleAddWrapper = function.WrapperTODO(SimpleAdd)
`)

fset := token.NewFileSet()
astFile, _ := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
pkgFiles := map[string]*ast.File{"test.go": astFile}

var output bytes.Buffer
err := gen.RewriteAstFileSource(
    fset, "testpkg", pkgFiles, astFile, "test.go",
    source,
    false,   // verbose
    &output, // print the rewritten file instead of writing it
    false,   // validate
    nil,     // JSON type replacements
    nil,     // local import prefixes
)

// output contains generated wrapper code
```

## Troubleshooting

### "WrapperTODO panics when called"

This is expected! `WrapperTODO` is a placeholder that panics if called before generation. Always run `go generate` after adding `WrapperTODO` declarations.

### "can't find function X in package Y"

Make sure the function is:
1. Exported (starts with capital letter) if in another package
2. Properly imported
3. Spelled correctly in the `WrapperTODO` call

### "generated code doesn't compile"

1. Run `go generate` again to ensure latest code
2. Check for import conflicts (aliases)
3. Verify function signature hasn't changed

### "dot imports in the file of a wrapped function from another package are not supported"

The file of the wrapped function dot imports a package, so the generator can't tell which package the unqualified types of its arguments or results belong to. Import the package with a name in that file, or wrap a function of your own package that calls it.

### Import Alias Conflicts

The generator automatically handles import alias conflicts. If your file imports a package with alias `A` and the wrapped function's package uses alias `B`, the generator remaps package qualifiers correctly.

## License

MIT License. See [LICENSE](../../LICENSE) file.