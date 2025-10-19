# gen-func-wrappers

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

```sh
go install github.com/domonda/go-function/cmd/gen-func-wrappers@latest
```

## Quick Start

Here's a complete example from declaration to usage:

```go
package myapp

import "context"
import "github.com/domonda/go-function"

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
results, err = greetUserWrapper.CallWithJSON(ctx, []byte(`{"Name":"Diana","Formal":true}`))
```

## Usage

### Step 1: Write Your Function

Write any regular Go function with clear documentation. Use comment format `argName: description` to document arguments:

```go
// ProcessOrder processes a customer order.
//   orderID: The unique order identifier
//   quantity: Number of items to order
//   express: Whether to use express shipping
func ProcessOrder(ctx context.Context, orderID string, quantity int, express bool) error {
    // ... implementation
}
```

### Step 2: Declare a Wrapper

Use `function.WrapperTODO()` to declare a wrapper variable:

```go
var processOrderWrapper = function.WrapperTODO(ProcessOrder)
```

**Important:** The variable name should follow the convention `{functionName}Wrapper` (camelCase), but this is not strictly required.

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
    return "ProcessOrder"
}

func (processOrderWrapperT) NumArgs() int { return 4 }

func (processOrderWrapperT) Call(ctx context.Context, args []any) (results []any, err error) {
    err = ProcessOrder(ctx, args[0].(string), args[1].(int), args[2].(bool))
    return nil, err
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
    "OrderID": "ORD-999",
    "Quantity": 7,
    "Express": false
}`))
```

## Calling Conventions

Generated wrappers support multiple calling conventions:

### 1. Call - Typed Arguments

```go
func (w Wrapper) Call(ctx context.Context, args []any) ([]any, error)
```

Accepts typed arguments as `[]any`. Fast but requires type assertions.

### 2. CallWithStrings - Positional String Arguments

```go
func (w Wrapper) CallWithStrings(ctx context.Context, strs ...string) ([]any, error)
```

Arguments are parsed from strings in order. Perfect for CLI applications.

### 3. CallWithNamedStrings - Named String Arguments

```go
func (w Wrapper) CallWithNamedStrings(ctx context.Context, strs map[string]string) ([]any, error)
```

Arguments are parsed by name. Perfect for HTTP query parameters.

### 4. CallWithJSON - JSON Arguments

```go
func (w Wrapper) CallWithJSON(ctx context.Context, argsJSON []byte) ([]any, error)
```

Arguments are unmarshaled from JSON. Perfect for REST APIs. Field names are capitalized (e.g., `orderID` → `OrderID`).

### Wrapper Metadata

All wrappers provide metadata methods:

```go
wrapper.Name()              // "ProcessOrder"
wrapper.NumArgs()           // 4
wrapper.ContextArg()        // true (if first arg is context.Context)
wrapper.ErrorResult()       // true (if last result is error)
wrapper.NumResults()        // 1
wrapper.ArgNames()          // ["ctx", "orderID", "quantity", "express"]
wrapper.ArgDescriptions()   // ["", "The unique order identifier", ...]
wrapper.ArgTypes()          // []reflect.Type{...}
wrapper.ResultTypes()       // []reflect.Type{...}
```

## Command Line Options

```sh
go tool gen-func-wrappers [options] <file|directory|package>
```

### Options

- `-verbose` - Print detailed information about processing
- `-print` - Print generated code to stdout instead of modifying files
- `-replaceForJSON=interface:concrete` - Replace interface types with concrete types for JSON unmarshaling
- `-localImportPrefix=prefix` - Treat import paths starting with prefix as "local" for import grouping

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

Print output without modifying files:

```sh
go tool gen-func-wrappers -print myfile.go
```

With JSON type replacements:

```sh
go tool gen-func-wrappers -replaceForJSON=fs.FileReader:fs.File myfile.go
```

## JSON Type Replacements

When a function uses interface types that need special handling during JSON unmarshaling, use the `-replaceForJSON` flag:

```sh
go tool gen-func-wrappers -replaceForJSON=io.Reader:*bytes.Buffer myfile.go
```

This tells the generator that when unmarshaling JSON for arguments of type `io.Reader`, it should use `*bytes.Buffer` as the concrete type.

Multiple replacements:

```sh
go tool gen-func-wrappers \
  -replaceForJSON=io.Reader:*bytes.Buffer \
  -replaceForJSON=fs.FileReader:fs.File \
  myfile.go
```

## Advanced Usage

### Integration with HTTP Handlers

```go
http.HandleFunc("/greet", func(w http.ResponseWriter, r *http.Request) {
    results, err := greetUserWrapper.CallWithNamedStrings(
        r.Context(),
        map[string]string{
            "name":   r.URL.Query().Get("name"),
            "formal": r.URL.Query().Get("formal"),
        },
    )
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    fmt.Fprint(w, results[0])
})
```

### Integration with CLI

```go
import "github.com/domonda/go-function/cli"

dispatcher := cli.NewStringArgsDispatcher()
dispatcher.MustAddCommand(greetUserWrapper, "greet")

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
7. **Format Output:** Formats the code and adds necessary imports

### Generated vs Reflection

**Generated wrappers** (this tool):
- ✅ Zero runtime overhead (compiled to direct function calls)
- ✅ Type-safe at compile time
- ✅ Fast - no reflection at runtime
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

The package includes `RewriteAstFileSource` for in-memory testing without disk I/O:

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
    source, false, &output, nil, nil,
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

### Import Alias Conflicts

The generator automatically handles import alias conflicts. If your file imports a package with alias `A` and the wrapped function's package uses alias `B`, the generator remaps package qualifiers correctly.

## License

MIT License. See [LICENSE](../../LICENSE) file.