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

As a go tool of your module, added to its go.mod file:

	go get -tool github.com/domonda/go-function/cmd/gen-func-wrappers@latest

Then use it as:

	go tool gen-func-wrappers [flags] [paths...]

# Usage

Basic usage:

	go tool gen-func-wrappers                    # Process current directory
	go tool gen-func-wrappers ./pkg/mypackage    # Process specific directory
	go tool gen-func-wrappers ./...              # Process recursively
	go tool gen-func-wrappers ./myfile.go        # Process single file
	go tool gen-func-wrappers ./pkg/... ./cmd    # Process several paths

Every path argument is processed, an error of one path is reported
and the processing continues with the next path.
Like the go command, recursive processing skips directories
starting with . or _, testdata, vendor, and nested modules
with their own go.mod file.

Command-line flags:

	-replaceForJSON string
	    Comma-separated list of InterfaceType:ImplementationType mappings
	    used for JSON unmarshalling. This is useful when an interface type
	    cannot be directly unmarshalled from JSON.
	    The flag can be repeated to add more mappings.
	    Example: -replaceForJSON=io.Reader:*bytes.Buffer
	    See JSON Type Replacement for path-like types that must
	    only be used for trusted JSON.

	-localImports string
	    Comma-separated list of local import prefixes for import grouping.
	    If not specified, automatically detects from the go.mod file
	    of every path argument.
	    Local imports are grouped separately from standard library and third-party imports.
	    Examples:
	      -localImports=github.com/myorg/
	      -localImports=github.com/myorg/,github.com/myteam/

	-verbose
	    Print detailed information about what's happening during generation.

	-print
	    Print generated code to stdout instead of writing to files.
	    Useful for debugging and previewing changes.

	-validate
	    Check for missing or outdated function wrappers without modifying files.
	    Reports issues to stderr and exits with code 1 if any are found.
	    Useful for CI validation to ensure all wrappers are up to date.
	    Exit codes:
	      0 - All wrappers are up to date
	      1 - Missing or outdated wrappers found
	      2 - Any other error, like a path that doesn't exist or a file that doesn't parse

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
A type with implementation comment without variable is regenerated as type
that is instantiated with myWrapperT{}.

TODO calls are also found in var blocks, the generated code is then
inserted after the block. They are recognized with the name that the file
uses for the go-function package, like fn.WrapperTODO for an import
with the name fn, or WrapperTODO for a dot import.
A TODO call that can't be rewritten, like in the declaration
of several variables, is an error.

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
	    return "myapp.GreetUser(ctx context.Context, name string, formal bool) (string, error)"
	}

	func (greetUserWrapperT) Name() string {
	    return "myapp.GreetUser"
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

	func ProcessData(ctx context.Context, data io.Reader) error {
	    // ...
	}

Generate wrapper with type replacement:

	go tool gen-func-wrappers -replaceForJSON=io.Reader:*bytes.Buffer

This tells the generator to use *bytes.Buffer as the concrete type when
unmarshalling JSON for CallWithJSON method, while keeping io.Reader
in all other methods. The unmarshalled value is assigned to the argument,
an argument without key or with the value null stays nil.
A variadic argument ...io.Reader is unmarshalled from a JSON array
whose elements are unmarshalled as *bytes.Buffer, null elements stay nil.
The replacement type has to unmarshal the JSON that callers send:
*bytes.Buffer only shows the syntax, it unmarshals an empty buffer
from {} and no content, while a type like fs.MemFile of
github.com/ungerik/go-fs unmarshals the name and content of a file.

The types are written like in the rewritten file. The package of the
replacement type is resolved by the imports of the rewritten file,
else by the imports of the file of the wrapped function, whose import
is then added. A package that neither file imports is an error.

Warning: the replacement type decides what the JSON of a caller can do.
A path-like type makes the JSON name files of the server:
with -replaceForJSON=fs.FileReader:fs.File, fs.File of
github.com/ungerik/go-fs being a local file path or URI string,
CallWithJSON unmarshals a JSON string like "/etc/passwd" into a file
of the server that the wrapped function can then read.
Only use such replacements for wrappers that are called with trusted JSON.
For JSON from clients, like HTTP request bodies, use a type that carries
the content, like fs.MemFile with the file name and base64 encoded data.

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

Like function.ReflectWrapper, Call passes missing arguments and untyped nil
as zero values, accepts values that are assignable to the argument type,
like a []int for an argument of type MyInts with the underlying type []int,
and returns an error for too many arguments or an argument of the wrong type.
The argument is checked with a type assertion, only if it fails
reflection checks if it's assignable, except for predeclared types like int
and interface type literals, for which the type assertion is enough.

CallWithStrings returns an error for more strings than arguments,
except for a variadic last argument: like in a Go call, every string
from its position on is one of its elements, also a single string,
and without strings it has no elements.
CallWithNamedStrings parses the single string of a variadic argument as slice
like "[1,2,3]". CallWithStrings and CallWithNamedStrings pass the unparsed
string to arguments and variadic elements of type any.

Like function.ReflectWrapper, CallWithJSON unmarshals a JSON object
whose keys are the exact argument names, other keys are ignored,
also the ones that only differ in case, and the arguments without key
are zero values. An argument of type error is unmarshalled from its
message string. Invalid JSON, or JSON that is no object like an array,
returns a function.ErrParseArgsJSON, also for a function without arguments,
which is then not called. JSON null is handled like an empty object.
An argument value that can't be unmarshalled returns a function.ErrParseArgJSON.

# Argument Descriptions

The generator extracts argument descriptions from function comments.
A line of the comment that starts with the argument name followed by a colon
describes the argument with the rest of the line, also as list item:

	// MyFunction does something useful.
	//   argName: Description of this argument
	//   - anotherArg: Description of another argument
	func MyFunction(argName, anotherArg string) error {
	    // ...
	}

Leading spaces and tabs are ignored, so the descriptions are still found
after gofmt reformatted the indented lines with a tab.
Unnamed arguments get the names arg0, arg1, ... by their position.

These descriptions become available via the ArgDescriptions() method and are
used by CLI and HTML form generators to provide helpful labels.

# Integration with Build Process

You can integrate gen-func-wrappers into your build process using go:generate:

	//go:generate go tool gen-func-wrappers -verbose

Or create a script:

	#!/bin/bash
	go tool gen-func-wrappers -verbose -replaceForJSON=io.Reader:*bytes.Buffer

Then run it before building:

	./scripts/gen-func-wrappers.sh
	go build ./...

# Validation in CI

Use the -validate flag in CI pipelines to ensure all wrappers are up to date:

	# In your CI workflow (e.g., GitHub Actions, GitLab CI)
	go tool gen-func-wrappers -validate -verbose ./...

This will:
- Exit with code 0 if all wrappers are current
- Exit with code 1 and print errors if any wrappers need updating
- Exit with code 2 for any other error, like a file that doesn't parse
- Prevent merging code with outdated or missing wrapper implementations

Example GitHub Actions workflow:

  - name: Validate function wrappers
    run: go tool gen-func-wrappers -validate -verbose ./...

# Implementation Details

The tool performs these steps:

 1. Parse Go source files into Abstract Syntax Trees (AST)
 2. Find wrapper declarations (TODO calls or implementation comments)
 3. Locate the referenced function's declaration
 4. Generate wrapper type and methods based on function signature
 5. Detect and add required imports automatically
 6. Replace old declarations with generated code
 7. Format the result using gofmt and remove the imports that are not used
    anymore, except blank imports, dot imports, and "C"

The generated code is type-safe and includes proper error handling for
string-to-type conversions in CallWithStrings and CallWithNamedStrings methods.

# Limitations

  - Only exported functions from imported packages can be wrapped (unless in same package),
    functions of the standard library included
  - Functions with receiver methods cannot be wrapped (use function adapters)
  - Functions of other packages whose file has dot imports can't be wrapped
    if their arguments or results use unqualified types

# See Also

  - github.com/domonda/go-function - Main package with wrapper interfaces
  - function.ReflectWrapper - Reflection-based wrapper for runtime usage
  - function.MustReflectWrapper - Panic version of ReflectWrapper
*/
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/domonda/go-function/cmd/gen-func-wrappers/internal/gen"
)

var (
	// jsonTypeReplacements maps types to the types used for JSON unmarshalling,
	// set by the -replaceForJSON flag that can be repeated.
	// Format: "InterfaceType:ImplementationType,..."
	// Example: "io.Reader:*bytes.Buffer"
	jsonTypeReplacements = make(map[string]string)

	// localImports specifies local import prefixes separated by commas.
	// If empty, auto-detects from go.mod file.
	// Example: "github.com/myorg/,github.com/myteam/"
	localImports string

	// verbose controls whether to print detailed generation information.
	verbose bool

	// printOnly prints generated code to stdout instead of writing files.
	printOnly bool

	// validate performs dry-run validation without modifying files.
	validate bool

	// printHelp prints usage information and exits.
	printHelp bool
)

// main is the entry point for the gen-func-wrappers code generation tool.
// It processes command-line flags, validates the target paths, and invokes
// the code generator on the specified files or directories.
//
// Usage:
//
//	gen-func-wrappers [flags] [paths...]
//
// Every path argument can be:
//   - A directory: Processes that directory
//   - A directory with "...": Processes recursively (e.g., "./...")
//   - A file: Processes single file
//
// Without path arguments the current working directory is processed.
//
// Exit codes:
//   - 0: Success
//   - 1: Validation found missing or outdated wrappers (with -validate) and no other error
//   - 2: Error (invalid arguments or generation failure)
func main() {
	flag.Func("replaceForJSON", "comma separated list of InterfaceType:ImplementationType used for JSON unmarshalling, can be repeated", jsonTypeReplacementsFlag(jsonTypeReplacements))
	flag.StringVar(&localImports, "localImports", "", "comma separated list of local import prefixes (auto-detects from go.mod if not specified)")
	flag.BoolVar(&verbose, "verbose", false, "prints information of what's happening")
	flag.BoolVar(&printOnly, "print", false, "prints to stdout instead of writing files")
	flag.BoolVar(&validate, "validate", false, "check for missing or outdated wrappers without modifying files")
	flag.BoolVar(&printHelp, "help", false, "prints this help output")
	flag.Parse()
	if printHelp {
		flag.PrintDefaults()
		os.Exit(2)
	}

	if exitCode := rewritePaths(flag.Args(), jsonTypeReplacements, os.Stdout, os.Stderr); exitCode != 0 {
		os.Exit(exitCode)
	}
}

// jsonTypeReplacementsFlag returns the function of the -replaceForJSON flag
// that adds the comma separated InterfaceType:ImplementationType pairs
// of every flag value to replacements.
func jsonTypeReplacementsFlag(replacements map[string]string) func(string) error {
	return func(value string) error {
		for repl := range strings.SplitSeq(value, ",") {
			types := strings.Split(repl, ":")
			if len(types) != 2 || types[0] == "" || types[1] == "" {
				return fmt.Errorf("invalid InterfaceType:ImplementationType %q", repl)
			}
			replacements[types[0]] = types[1]
		}
		return nil
	}
}

// rewritePaths rewrites the function wrappers of every path argument,
// or of the current working directory without arguments.
// The error of a path is printed to stderr and the next path is processed.
// Generated code is printed to stdout with the -print flag.
// Returns the exit code of the process: 0 if all paths succeeded,
// 1 if the only failures are missing or outdated wrappers found with -validate,
// else 2.
func rewritePaths(args []string, jsonTypeReplacements map[string]string, stdout, stderr io.Writer) (exitCode int) {
	if len(args) == 0 {
		args = []string{"."}
	}
	for _, arg := range args {
		err := rewritePath(arg, jsonTypeReplacements, stdout)
		if err != nil {
			fmt.Fprintln(stderr, "gen-func-wrappers error:", err)
			if errors.Is(err, gen.ErrWrappersOutdated) {
				exitCode = max(exitCode, 1)
			} else {
				exitCode = 2
			}
		}
	}
	return exitCode
}

// rewritePath rewrites the function wrappers of the path argument,
// a file, a directory, or a directory with "..." to process it recursively.
func rewritePath(arg string, jsonTypeReplacements map[string]string, stdout io.Writer) error {
	recursive := strings.HasSuffix(arg, "...")
	path := filepath.Clean(strings.TrimSuffix(arg, "..."))
	if path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = cwd
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Determine local import prefixes: use flag if provided, otherwise auto-detect
	var localImportPrefixes []string
	if localImports != "" {
		// Parse comma-separated list from command-line flag
		for prefix := range strings.SplitSeq(localImports, ",") {
			prefix = strings.TrimSpace(prefix)
			if prefix != "" {
				localImportPrefixes = append(localImportPrefixes, prefix)
			}
		}
	} else {
		// Auto-detect from the go.mod file of every path argument,
		// because the paths can be in different modules
		localImportPrefixes = gen.DetectLocalImportPrefixes(path)
		if verbose && len(localImportPrefixes) > 0 {
			fmt.Println("Auto-detected local import prefix for", arg+":", localImportPrefixes[0])
		}
	}

	var printOnlyWriter io.Writer
	if printOnly {
		printOnlyWriter = stdout
	}
	if !info.IsDir() {
		return gen.RewriteFile(path, verbose, printOnlyWriter, validate, jsonTypeReplacements, localImportPrefixes)
	}
	if recursive {
		path = filepath.Join(path, "...")
	}
	return gen.RewriteDir(path, verbose, printOnlyWriter, validate, jsonTypeReplacements, localImportPrefixes)
}
