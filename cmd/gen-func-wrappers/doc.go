/*
Package main provides gen-func-wrappers, a code generation tool that creates optimized
function wrapper implementations without reflection overhead.

# Overview

gen-func-wrappers is a command-line tool that automatically generates type-safe wrapper
implementations for Go functions. Unlike ReflectWrapper which uses runtime reflection,
the generated wrappers are compiled code with zero reflection overhead, making them
ideal for performance-critical applications.

The tool scans Go source files for wrapper declarations and generates implementations
of the function.Wrapper interface (or its sub-interfaces) for specified functions.

# Installation

As a go tool:

	go install github.com/domonda/go-function/cmd/gen-func-wrappers@latest

Then use it as:

	go tool gen-func-wrappers [flags] [path]

# Usage

Basic usage:

	go tool gen-func-wrappers                    # Process current directory
	go tool gen-func-wrappers ./pkg/mypackage    # Process specific directory
	go tool gen-func-wrappers ./...              # Process recursively
	go tool gen-func-wrappers ./myfile.go        # Process single file

Command-line flags:

	-replaceForJSON string
	    Comma-separated list of InterfaceType:ImplementationType mappings
	    used for JSON unmarshalling. This is useful when an interface type
	    cannot be directly unmarshalled from JSON.
	    Example: -replaceForJSON=fs.FileReader:fs.File

	-verbose
	    Print detailed information about what's happening during generation.

	-print
	    Print generated code to stdout instead of writing to files.
	    Useful for debugging and previewing changes.

	-help
	    Print help information and exit.

# How It Works

The tool works by scanning for specially formatted declarations in your code
and replacing them with generated implementations. There are two approaches:

## Approach 1: TODO Function Call

Declare a variable using a TODO function call:

	var myWrapper = function.WrapperTODO(MyFunction)

When you run gen-func-wrappers, it will replace this with:

	// myWrapper wraps MyFunction as function.Wrapper (generated code)
	var myWrapper myWrapperT

	// myWrapperT wraps MyFunction as function.Wrapper (generated code)
	type myWrapperT struct{}

	// ... all wrapper methods generated ...

## Approach 2: Type and Variable Declaration

Alternatively, declare a type and variable with implementation comments:

	// myWrapper wraps MyFunction as function.Wrapper (generated code)
	var myWrapper myWrapperT

	// myWrapperT wraps MyFunction as function.Wrapper (generated code)
	type myWrapperT struct{}

The tool will replace everything (variable, type, methods) with fresh generated code.

# Example Walkthrough

Let's say you have a function you want to wrap:

	// GreetUser creates a personalized greeting.
	//   name: The user's name
	//   formal: Whether to use formal language
	func GreetUser(ctx context.Context, name string, formal bool) (string, error) {
	    if formal {
	        return "Good day, " + name, nil
	    }
	    return "Hi, " + name + "!", nil
	}

To create a wrapper, add this declaration to your code:

	var greetUserWrapper = function.WrapperTODO(GreetUser)

Run the code generator:

	go tool gen-func-wrappers -verbose

The tool will replace your declaration with:

	// greetUserWrapper wraps GreetUser as function.Wrapper (generated code)
	var greetUserWrapper greetUserWrapperT

	// greetUserWrapperT wraps GreetUser as function.Wrapper (generated code)
	type greetUserWrapperT struct{}

	func (greetUserWrapperT) String() string {
	    return "GreetUser(ctx context.Context, name string, formal bool) (string, error)"
	}

	func (greetUserWrapperT) Name() string {
	    return "GreetUser"
	}

	func (greetUserWrapperT) NumArgs() int      { return 3 }
	func (greetUserWrapperT) ContextArg() bool  { return true }
	func (greetUserWrapperT) NumResults() int   { return 2 }
	func (greetUserWrapperT) ErrorResult() bool { return true }

	func (greetUserWrapperT) ArgNames() []string {
	    return []string{"ctx", "name", "formal"}
	}

	func (greetUserWrapperT) ArgDescriptions() []string {
	    return []string{"", "The user's name", "Whether to use formal language"}
	}

	// ... and all other wrapper methods (Call, CallWithStrings, etc.) ...

Now you can use greetUserWrapper anywhere that expects a function.Wrapper,
with zero reflection overhead.

# JSON Type Replacement

When a function parameter is an interface type, JSON unmarshalling cannot
automatically determine the concrete type to create. Use -replaceForJSON
to specify the implementation type:

Example function:

	func ProcessFile(ctx context.Context, file fs.FileReader) error {
	    // ...
	}

Generate wrapper with type replacement:

	go tool gen-func-wrappers -replaceForJSON=fs.FileReader:fs.File

This tells the generator to use fs.File as the concrete type when
unmarshalling JSON for CallWithJSON method, while keeping fs.FileReader
in all other methods.

# Generated Methods

The tool generates implementations for all methods of the function.Wrapper interface:

Description methods:
  - Name() string
  - NumArgs() int
  - ContextArg() bool
  - NumResults() int
  - ErrorResult() bool
  - ArgNames() []string
  - ArgDescriptions() []string
  - ArgTypes() []reflect.Type
  - ResultTypes() []reflect.Type

Calling convention methods:
  - Call(context.Context, []any) ([]any, error)
  - CallWithStrings(context.Context, ...string) ([]any, error)
  - CallWithNamedStrings(context.Context, map[string]string) ([]any, error)
  - CallWithJSON(context.Context, []byte) ([]any, error)

# Argument Descriptions

The generator extracts argument descriptions from function comments using
this format:

	// MyFunction does something useful.
	//   argName: Description of this argument
	//   anotherArg: Description of another argument
	func MyFunction(argName, anotherArg string) error {
	    // ...
	}

These descriptions become available via the ArgDescriptions() method and are
used by CLI and HTML form generators to provide helpful labels.

# Integration with Build Process

You can integrate gen-func-wrappers into your build process using go:generate:

	//go:generate go tool gen-func-wrappers -verbose

Or create a script:

	#!/bin/bash
	go tool gen-func-wrappers -verbose -replaceForJSON=fs.FileReader:fs.File

Then run it before building:

	./scripts/gen-func-wrappers.sh
	go build ./...

# Implementation Details

The tool performs these steps:

1. Parse Go source files into Abstract Syntax Trees (AST)
2. Find wrapper declarations (TODO calls or implementation comments)
3. Locate the referenced function's declaration
4. Generate wrapper type and methods based on function signature
5. Detect and add required imports automatically
6. Replace old declarations with generated code
7. Format the result using gofmt

The generated code is type-safe and includes proper error handling for
string-to-type conversions in CallWithStrings and CallWithNamedStrings methods.

# Limitations

- Only exported functions from imported packages can be wrapped (unless in same package)
- Functions with receiver methods cannot be wrapped (use function adapters)
- Variadic parameters are supported but have some JSON unmarshalling limitations
- Local import prefixes are currently hard-coded (see TODO in main.go:74)

# See Also

  - github.com/domonda/go-function - Main package with wrapper interfaces
  - function.ReflectWrapper - Reflection-based wrapper for runtime usage
  - function.MustReflectWrapper - Panic version of ReflectWrapper
*/
package main
