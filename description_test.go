package function

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReflectDescription_NotAFunction(t *testing.T) {
	// ReflectDescription documents to return an error for non-functions,
	// so a nil value must not crash with a reflect panic about a zero Value.
	var (
		desc Description
		err  error
	)
	require.NotPanics(t, func() { desc, err = ReflectDescription("f", nil) })
	assert.EqualError(t, err, "<nil> passed instead of a function")
	assert.Nil(t, desc)

	desc, err = ReflectDescription("f", 42)
	assert.EqualError(t, err, "int passed instead of a function")
	assert.Nil(t, desc)
}

func TestReflectDescription_Variadic(t *testing.T) {
	variadic, err := ReflectDescription("f", func(int, ...string) {})
	require.NoError(t, err)
	slice, err := ReflectDescription("f", func(int, []string) {})
	require.NoError(t, err)
	assert.True(t, variadic.Variadic())
	assert.False(t, slice.Variadic())
	assert.Equal(t, "f(a0 int, a1 ...string)", variadic.String())
}

func TestReflectDescription_SlicesAreCopies(t *testing.T) {
	// The caller may modify the returned slices
	// without changing the Description
	desc, err := ReflectDescription("f", func(int) string { return "" })
	require.NoError(t, err)
	desc.ArgNames()[0] = "changed"
	desc.ArgDescriptions()[0] = "changed"
	desc.ArgTypes()[0] = nil
	desc.ResultTypes()[0] = nil
	assert.Equal(t, []string{"a0"}, desc.ArgNames())
	assert.Equal(t, []string{""}, desc.ArgDescriptions())
	assert.Equal(t, []reflect.Type{reflect.TypeFor[int]()}, desc.ArgTypes())
	assert.Equal(t, []reflect.Type{reflect.TypeFor[string]()}, desc.ResultTypes())
}

func TestReflectDescription_StringEmptyInterfaceAsAny(t *testing.T) {
	// reflect writes empty interfaces as "interface {}",
	// generated wrappers write them as any like the source code usually does
	desc, err := ReflectDescription("f", func(any, map[string]interface{}, ...any) ([]any, interface{ M() }) { return nil, nil })
	require.NoError(t, err)
	assert.Equal(t, "f(a0 any, a1 map[string]any, a2 ...any) ([]any, interface { M() })", desc.String())
}
