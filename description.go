package function

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Description interface {
	Name() string
	String() string

	NumArgs() int
	ContextArg() bool
	NumResults() int
	ErrorResult() bool

	ArgNames() []string
	ArgDescriptions() []string
	ArgTypes() []reflect.Type
	ResultTypes() []reflect.Type
}

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
	return len(f.argTypes) > 0 && f.argTypes[0].String() == "context.Context"
}
func (f *description) NumResults() int { return len(f.resultTypes) }
func (f *description) ErrorResult() bool {
	return len(f.resultTypes) > 0 && f.resultTypes[0].String() == "error"
}
func (f *description) ArgNames() []string          { return f.argNames }
func (f *description) ArgDescriptions() []string   { return f.argDescriptions }
func (f *description) ArgTypes() []reflect.Type    { return f.argTypes }
func (f *description) ResultTypes() []reflect.Type { return f.resultTypes }
