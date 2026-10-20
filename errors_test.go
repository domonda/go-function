package function

import (
	"context"
	"encoding/json"
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

func TestErrParseArgJSON_ErrorAndUnwrap(t *testing.T) {
	// The JSON errors name the argument and unwrap to the error of the
	// JSON decoder, so callers can find out why the JSON was rejected
	w := MustReflectWrapper(func(amount int) {}, "amount")
	_, err := w.CallWithJSON(context.Background(), []byte(`{"amount":"x"}`))
	var errArg ErrParseArgJSON
	require.ErrorAs(t, err, &errArg)
	assert.Equal(t, "amount", errArg.ArgName)
	assert.ErrorContains(t, err, `error unmarshalling JSON for argument "amount" of function`)
	var typeErr *json.UnmarshalTypeError
	assert.ErrorAs(t, err, &typeErr)

	_, err = w.CallWithJSON(context.Background(), []byte(`{`))
	assert.ErrorAs(t, err, new(ErrParseArgsJSON))
	var syntaxErr *json.SyntaxError
	assert.ErrorAs(t, err, &syntaxErr)
}
