package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoidAndErrorFuncWrapper_Args(t *testing.T) {
	// Like ReflectWrapper, the wrappers of functions without arguments
	// reject arguments, strings, and JSON that is no object,
	// instead of calling the function with whatever they get
	var calls int
	tests := []struct {
		name    string
		w       Wrapper
		callErr error
	}{
		{name: "VoidFuncWrapper", w: VoidFuncWrapper(func() { calls++ })},
		{name: "ErrorFuncWrapper", w: ErrorFuncWrapper(func() error { calls++; return errFuncWrapper }), callErr: errFuncWrapper},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			w := tt.w
			calls = 0
			assert.False(t, w.Variadic())

			_, err := w.Call(ctx, []any{1})
			assert.EqualError(t, err, "1 arguments passed, but function "+w.String()+" takes 0")
			_, err = w.CallWithStrings(ctx, "a", "b")
			assert.EqualError(t, err, "2 arguments passed, but function "+w.String()+" takes 0")
			for _, argsJSON := range []string{`[]`, `1`, `not JSON`, ``} {
				_, err = w.CallWithJSON(ctx, []byte(argsJSON))
				var errArgs ErrParseArgsJSON
				assert.ErrorAs(t, err, &errArgs, "JSON %q", argsJSON)
			}
			require.Zero(t, calls, "function must not be called for invalid arguments")

			// Named strings and the values of a JSON object
			// are ignored like by ReflectWrapper
			valid := []func() ([]any, error){
				func() ([]any, error) { return w.Call(ctx, nil) },
				func() ([]any, error) { return w.CallWithStrings(ctx) },
				func() ([]any, error) { return w.CallWithNamedStrings(ctx, map[string]string{"x": "1"}) },
				func() ([]any, error) { return w.CallWithJSON(ctx, []byte(`{"x":1}`)) },
				func() ([]any, error) { return w.CallWithJSON(ctx, []byte(`null`)) },
			}
			for _, call := range valid {
				results, err := call()
				assert.Equal(t, tt.callErr, err)
				assert.Empty(t, results)
			}
			assert.Equal(t, len(valid), calls)
		})
	}
}

var errFuncWrapper = errors.New("func error")
