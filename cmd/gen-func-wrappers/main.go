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

	-localImports string
	    Comma-separated list of local import prefixes for import grouping.
	    If not specified, automatically detects from go.mod file.
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

# Validation in CI

Use the -validate flag in CI pipelines to ensure all wrappers are up to date:

	# In your CI workflow (e.g., GitHub Actions, GitLab CI)
	go tool gen-func-wrappers -validate -verbose ./...

This will:
- Exit with code 0 if all wrappers are current
- Exit with code 1 and print errors if any wrappers need updating
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

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/domonda/go-function/cmd/gen-func-wrappers/gen"
)

var (
	// replaceForJSON specifies type replacements for JSON unmarshalling.
	// Format: "InterfaceType:ImplementationType,..."
	// Example: "fs.FileReader:fs.File"
	replaceForJSON string

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
// It processes command-line flags, validates the target path, and invokes
// the code generator on the specified files or directories.
//
// Usage:
//
//	gen-func-wrappers [flags] [path]
//
// The path argument can be:
//   - Empty: Processes current working directory
//   - A directory: Processes that directory
//   - A directory with "...": Processes recursively (e.g., "./...")
//   - A file: Processes single file
//
// Exit codes:
//   - 0: Success
//   - 2: Error (invalid arguments or generation failure)
func main() {
	// flag.BoolVar(&exportedFuncs, "exported", false, "generate function.Wrapper implementation types exported package functions")
	// flag.StringVar(&genFilename, "genfile", "generated.go", "name of the file to be generated")
	// flag.StringVar(&namePrefix, "prefix", "Func", "prefix for function type names in the same package")
	flag.StringVar(&replaceForJSON, "replaceForJSON", "", "comma separated list of InterfaceType:ImplementationType used for JSON unmarshalling")
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

	var (
		args     = flag.Args()
		cwd, _   = os.Getwd()
		filePath string
	)
	if len(args) == 0 {
		filePath = cwd
	} else {
		recursive := strings.HasSuffix(args[0], "...")
		if args[0] == "." || args[0] == "./..." {
			filePath = cwd
		} else {
			filePath = filepath.Clean(strings.TrimSuffix(args[0], "..."))
		}
		if recursive {
			filePath = filepath.Join(filePath, "...")
		}
	}
	info, err := os.Stat(strings.TrimSuffix(filePath, "..."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-func-wrappers error:", err)
		os.Exit(2)
	}

	jsonTypeReplacements := make(map[string]string)
	if replaceForJSON != "" {
		for repl := range strings.SplitSeq(replaceForJSON, ",") {
			types := strings.Split(repl, ":")
			if len(types) != 2 {
				fmt.Fprintln(os.Stderr, "gen-func-wrappers error: invalid -replaceForJSON syntax")
				os.Exit(2)
			}
			jsonTypeReplacements[types[0]] = types[1]
		}
	}

	// Determine local import prefixes: use flag if provided, otherwise auto-detect
	var localImportPrefixes []string
	if localImports != "" {
		// Parse comma-separated list from command-line flag
		for _, prefix := range strings.Split(localImports, ",") {
			prefix = strings.TrimSpace(prefix)
			if prefix != "" {
				localImportPrefixes = append(localImportPrefixes, prefix)
			}
		}
	} else {
		// Auto-detect from go.mod file
		localImportPrefixes = gen.DetectLocalImportPrefixes(filePath)
		if verbose && len(localImportPrefixes) > 0 {
			fmt.Println("Auto-detected local import prefix:", localImportPrefixes[0])
		}
	}

	var printOnlyWriter io.Writer
	if printOnly {
		printOnlyWriter = os.Stdout
	}
	if info.IsDir() {
		err = gen.RewriteDir(filePath, verbose, printOnlyWriter, validate, jsonTypeReplacements, localImportPrefixes)
	} else {
		err = gen.RewriteFile(filePath, verbose, printOnlyWriter, validate, jsonTypeReplacements, localImportPrefixes)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-func-wrappers error:", err)
		if validate {
			os.Exit(1)
		} else {
			os.Exit(2)
		}
	}
}
