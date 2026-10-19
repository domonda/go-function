package function

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Description provides metadata about a wrapped function including its name,
// arguments, and results. This interface is used to introspect function signatures
// and generate documentation, help text, or user interfaces.
type Description interface {
	// Name returns the function name qualified with its package name like "pkg.Func".
	// ReflectWrapper only knows the import path of the package, so it qualifies
	// the name with the package name that goimports assumes for the import path:
	// its last element that is not a major version like v2, without a "go-" prefix,
	// up to the first character that can't be in an identifier,
	// like "function" for "github.com/domonda/go-function" and "yaml" for "gopkg.in/yaml.v3".
	// This differs from the package name of packages not named that way.
	// ReflectDescription returns the passed name unchanged,
	// VoidFuncWrapper and ErrorFuncWrapper return their signature.
	Name() string

	// String returns the signature of the function with its qualified name,
	// argument names and result types like "pkg.Func(a int, b string) (int, error)".
	// ReflectDescription uses the passed name, VoidFuncWrapper and ErrorFuncWrapper
	// return their signature without name.
	String() string

	// NumArgs returns the total number of arguments (including context if present).
	NumArgs() int

	// ContextArg returns true if the first argument is context.Context.
	ContextArg() bool

	// Variadic returns true if the last argument is variadic like xs in func(xs ...int).
	// ArgTypes returns the slice type of a variadic argument,
	// so Variadic distinguishes func(xs ...int) from func(xs []int),
	// which CallWithStringsWrapper.CallWithStrings calls differently.
	Variadic() bool

	// NumResults returns the total number of results (including error if present).
	NumResults() int

	// ErrorResult returns true if the last result is an error.
	ErrorResult() bool

	// ArgNames returns the names of all arguments,
	// including a context.Context argument if present.
	// The returned slices of ArgNames, ArgDescriptions, ArgTypes, and ResultTypes
	// can be modified by the caller without changing the Description.
	// For generated wrappers, these are the actual parameter names.
	// For ReflectWrapper, these are the passed argNames.
	// For ReflectDescription, these are generated as "a0", "a1", etc.
	ArgNames() []string

	// ArgDescriptions returns documentation for each argument,
	// including a context.Context argument if present.
	// This is typically extracted from function comments by code generators.
	ArgDescriptions() []string

	// ArgTypes returns the reflect.Type for each argument,
	// including a context.Context argument if present.
	// The type of a variadic last argument is its slice type.
	ArgTypes() []reflect.Type

	// ResultTypes returns the reflect.Type for each result.
	ResultTypes() []reflect.Type
}

// ReflectDescription creates a Description for any function using reflection.
// The name parameter is returned unchanged as the function name.
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
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return nil, fmt.Errorf("%T passed instead of a function", f)
	}
	t := v.Type()
	info := &description{
		name:            name,
		argNames:        make([]string, t.NumIn()),
		argDescriptions: make([]string, t.NumIn()),
		argTypes:        make([]reflect.Type, t.NumIn()),
		resultTypes:     make([]reflect.Type, t.NumOut()),
		variadic:        t.IsVariadic(),
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
	variadic        bool
}

func (f *description) Name() string { return f.name }
func (f *description) String() string {
	return signatureString(f.name, f.argNames, f.argTypes, f.resultTypes, f.variadic)
}

// signatureString returns the signature of a function
// like "pkg.Func(ctx context.Context, a int, xs ...string) (int, error)",
// the format of Description.String for all Description implementations.
// The last argument type of a variadic function is its slice type.
// Empty interfaces are written as any like in the source code
// of generated wrappers, also in types like map[string]any.
func signatureString(name string, argNames []string, argTypes, resultTypes []reflect.Type, variadic bool) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('(')
	for i, argName := range argNames {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(argName)
		b.WriteByte(' ')
		if variadic && i == len(argNames)-1 {
			b.WriteString("...")
			b.WriteString(typeString(argTypes[i].Elem()))
		} else {
			b.WriteString(typeString(argTypes[i]))
		}
	}
	b.WriteByte(')')
	switch len(resultTypes) {
	case 0:
	case 1:
		b.WriteByte(' ')
		b.WriteString(typeString(resultTypes[0]))
	default:
		b.WriteString(" (")
		for i, resultType := range resultTypes {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(typeString(resultType))
		}
		b.WriteByte(')')
	}
	return b.String()
}

// typeString returns t.String() with the empty interfaces,
// which reflect writes as "interface {}", written as "any".
func typeString(t reflect.Type) string {
	return strings.ReplaceAll(t.String(), "interface {}", "any")
}

func (f *description) NumArgs() int { return len(f.argNames) }
func (f *description) ContextArg() bool {
	return len(f.argTypes) > 0 && f.argTypes[0] == typeOfContext
}
func (f *description) Variadic() bool  { return f.variadic }
func (f *description) NumResults() int { return len(f.resultTypes) }
func (f *description) ErrorResult() bool {
	// Check if the LAST result (not first) implements error interface
	// This follows Go convention where error is always the last return value
	return len(f.resultTypes) > 0 && f.resultTypes[len(f.resultTypes)-1] == typeOfError
}
func (f *description) ArgNames() []string          { return slices.Clone(f.argNames) }
func (f *description) ArgDescriptions() []string   { return slices.Clone(f.argDescriptions) }
func (f *description) ArgTypes() []reflect.Type    { return slices.Clone(f.argTypes) }
func (f *description) ResultTypes() []reflect.Type { return slices.Clone(f.resultTypes) }
