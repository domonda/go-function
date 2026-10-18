package function

import (
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
