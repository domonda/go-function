package function

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrParseArgString_ErrorWithoutValue(t *testing.T) {
	// The value is in the ArgValue field but not repeated in the message,
	// because it can be large or private and the scanner error describes it
	w := MustReflectWrapper(func(n int) {}, "n")
	value := strings.Repeat("x", 50)
	_, err := w.CallWithStrings(context.Background(), value)
	var errArg ErrParseArgString
	require.ErrorAs(t, err, &errArg)
	assert.Equal(t, value, errArg.ArgValue)
	assert.Equal(t, "n", errArg.ArgName)
	assert.Equal(t, 1, strings.Count(err.Error(), value), "value only in the scanner error: %s", err)
}

func TestErrParseArgsJSON_WithoutJSON(t *testing.T) {
	err := NewErrParseArgsJSON(errors.New("invalid"), VoidFuncWrapper(nil))
	assert.EqualError(t, err, "error unmarshalling JSON object for arguments of function func(): invalid")
}
