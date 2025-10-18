// Package function provides utilities for wrapping Go functions to enable
// multiple calling conventions including strings, JSON, HTTP handlers, and CLI commands.
//
// # Overview
//
// This package allows you to wrap any Go function and call it through different interfaces:
//   - Direct calls with []any arguments
//   - Calls with string arguments that are automatically converted to the correct types
//   - Calls with named string arguments (map[string]string)
//   - Calls with JSON-encoded arguments
//   - HTTP handlers that invoke the function
//   - CLI commands that parse command-line arguments
//
// # Basic Usage
//
// There are two main approaches to wrapping functions:
//
// 1. Using reflection (runtime overhead):
//
//	func Add(a, b int) int {
//	    return a + b
//	}
//
//	wrapper := function.MustReflectWrapper("Add", Add)
//	results, err := wrapper.CallWithStrings(ctx, "5", "3")
//	// results[0] == 8
//
// 2. Using code generation (no runtime overhead):
//
//	// In your code, use placeholder functions:
//	var AddWrapper = function.WrapperTODO(Add)
//
//	// Then run gen-func-wrappers to replace with generated code:
//	// go run github.com/domonda/go-function/cmd/gen-func-wrappers
//
// # Function Signatures
//
// The package supports various function signatures:
//   - With or without context.Context as first argument
//   - With or without error as last result
//   - Zero or more arguments of any type
//   - Zero or more results of any type
//
// Examples:
//
//	func SimpleFunc(a int) string
//	func WithContext(ctx context.Context, a int) (string, error)
//	func NoArgs() error
//	func NoResults(a string)
//	func Complex(ctx context.Context, a int, b string, c bool) (int, string, error)
//
// # String Conversion
//
// The package automatically converts strings to function argument types:
//   - Basic types: int, float, bool, string
//   - Time types: time.Time, time.Duration
//   - Pointers: converts "nil"/"null" to nil
//   - Slices and arrays: parses JSON-like syntax [1,2,3]
//   - Structs: parses JSON
//   - Custom types: supports encoding.TextUnmarshaler and json.Unmarshaler
//
// String conversion can be customized via StringScanners configuration.
//
// # Sub-packages
//
// The package includes several sub-packages for specific use cases:
//
//   - httpfun: Create HTTP handlers from functions
//   - htmlform: Generate HTML forms for function arguments
//   - cli: Build command-line interfaces from functions
//   - cmd/gen-func-wrappers: Code generator for zero-overhead wrappers
//
// # Performance Considerations
//
// Reflection-based wrappers (ReflectWrapper) have runtime overhead due to
// reflection and type conversion. For performance-critical code, use the
// code generator (gen-func-wrappers) to create specialized wrappers with
// no reflection overhead.
//
// # Error Handling
//
// Functions can return an error as the last result. The wrapper will return
// this error through the error return value of the Call* methods. If the
// function doesn't return an error, any panics during execution will be
// recovered and returned as errors (when CatchHandlerPanics is enabled).
//
// # Context Support
//
// If the first argument is context.Context, the wrapper will automatically
// pass the context provided to Call* methods. This enables proper
// cancellation, timeouts, and context value propagation.
package function
