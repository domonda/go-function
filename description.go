package function

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Description provides metadata about a wrapped function including its name,
// arguments, and results. This interface is used to introspect function signatures
// and generate documentation, help text, or user interfaces.
type Description interface {
	// Name returns the function name.
	Name() string

	// String returns a string representation of the function signature.
	String() string

	// NumArgs returns the total number of arguments (including context if present).
	NumArgs() int

	// ContextArg returns true if the first argument is context.Context.
	ContextArg() bool

	// NumResults returns the total number of results (including error if present).
	NumResults() int

	// ErrorResult returns true if the last result is an error.
	ErrorResult() bool

	// ArgNames returns the names of all arguments.
	// For generated wrappers, these are the actual parameter names.
	// For reflection-based wrappers, these are generated as "a0", "a1", etc.
	ArgNames() []string

	// ArgDescriptions returns documentation for each argument.
	// This is typically extracted from function comments by code generators.
	ArgDescriptions() []string

	// ArgTypes returns the reflect.Type for each argument.
	ArgTypes() []reflect.Type

	// ResultTypes returns the reflect.Type for each result.
	ResultTypes() []reflect.Type
}

// ReflectDescription creates a Description for any function using reflection.
// The name parameter is used as the function name since reflection cannot determine it.
// Argument names are generated as "a0", "a1", etc.
// Returns an error if f is not a function.
//
// Example:
//
//	func Add(a, b int) int { return a + b }
//	desc, err := function.ReflectDescription("Add", Add)
//	fmt.Println(desc.NumArgs()) // 2
//	fmt.Println(desc.ArgTypes()[0]) // int
func ReflectDescription(name string, f any) (Description, error) {
	t := reflect.ValueOf(f).Type()
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("%s passed instead of a function", t)
	}
	info := &description{
		name:            name,
		argNames:        make([]string, t.NumIn()),
		argDescriptions: make([]string, t.NumIn()),
		argTypes:        make([]reflect.Type, t.NumIn()),
		resultTypes:     make([]reflect.Type, t.NumOut()),
	}
	for i := range info.argTypes {
		info.argNames[i] = "a" + strconv.Itoa(i)
		info.argTypes[i] = t.In(i)
	}
	for i := range info.resultTypes {
		info.resultTypes[i] = t.Out(i)
	}
	return info, nil
}

// description is the internal implementation of the Description interface.
// It stores function metadata extracted either through reflection or code generation.
type description struct {
	name            string
	argNames        []string
	argDescriptions []string
	argTypes        []reflect.Type
	resultTypes     []reflect.Type
}

func (f *description) Name() string { return f.name }
func (f *description) String() string {
	var b strings.Builder
	b.WriteString(f.name)
	b.WriteByte('(')
	for i, argName := range f.argNames {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(argName)
		b.WriteByte(' ')
		b.WriteString(f.argTypes[i].String())
	}
	b.WriteByte(')')
	return b.String()
}
func (f *description) NumArgs() int { return len(f.argNames) }
func (f *description) ContextArg() bool {
	return len(f.argTypes) > 0 && f.argTypes[0] == typeOfContext
}
func (f *description) NumResults() int { return len(f.resultTypes) }
func (f *description) ErrorResult() bool {
	// Check if the LAST result (not first) implements error interface
	// This follows Go convention where error is always the last return value
	return len(f.resultTypes) > 0 && f.resultTypes[len(f.resultTypes)-1] == typeOfError
}
func (f *description) ArgNames() []string          { return f.argNames }
func (f *description) ArgDescriptions() []string   { return f.argDescriptions }
func (f *description) ArgTypes() []reflect.Type    { return f.argTypes }
func (f *description) ResultTypes() []reflect.Type { return f.resultTypes }
