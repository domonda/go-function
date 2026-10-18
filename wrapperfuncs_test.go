package function

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiErr is an error with a non-comparable dynamic type,
// comparing two multiErr values with == panics at runtime.
type multiErr []error

func (m multiErr) Error() string {
	strs := make([]string, len(m))
	for i, err := range m {
		strs[i] = err.Error()
	}
	return strings.Join(strs, "; ")
}

// wrappedErr has a comparable type, but == still panics
// if its interface field holds a non-comparable value.
type wrappedErr struct{ err error }

func (w wrappedErr) Error() string { return w.err.Error() }

func Test_sameError(t *testing.T) {
	// Results handlers usually return the passed result error unchanged.
	// Detecting that must never panic, even for error types
	// that can't be compared with ==.
	errA := errors.New("A")
	tests := []struct {
		name      string
		err       error
		resultErr error
		want      bool
	}{
		{name: "both nil", err: nil, resultErr: nil, want: true},
		{name: "nil and non-nil", err: nil, resultErr: errA, want: false},
		{name: "non-nil and nil", err: errA, resultErr: nil, want: false},
		{name: "same comparable error", err: errA, resultErr: errA, want: true},
		{name: "different comparable errors", err: errA, resultErr: errors.New("A"), want: false},
		{name: "different types", err: errA, resultErr: multiErr{errA}, want: false},
		{name: "same non-comparable error", err: multiErr{errA}, resultErr: multiErr{errA}, want: true},
		{name: "different non-comparable errors", err: multiErr{errA}, resultErr: multiErr{errors.New("B")}, want: false},
		{name: "comparable type holding non-comparable value", err: wrappedErr{multiErr{errA}}, resultErr: wrappedErr{multiErr{errA}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			require.NotPanics(t, func() { got = sameError(tt.err, tt.resultErr) })
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestArgsFuncs_NonComparableResultErr(t *testing.T) {
	// A results handler passing through a result error with a non-comparable
	// dynamic type (like a slice of errors) must not crash the args func
	// with "comparing uncomparable type", and the result error must be returned.
	resultErr := multiErr{errors.New("first"), errors.New("second")}
	failingFunc := MustReflectWrapper(func() error { return resultErr })
	passThroughHandler := ResultsHandlerFunc(func(ctx context.Context, results []any, resultErr error) error {
		return resultErr
	})
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "NewStringArgsFunc",
			call: func() error { return NewStringArgsFunc(failingFunc, passThroughHandler)(ctx) },
		},
		{
			name: "NewNamedStringArgsFunc",
			call: func() error { return NewNamedStringArgsFunc(failingFunc, passThroughHandler)(ctx, nil) },
		},
		{
			name: "NewJSONArgsFunc",
			call: func() error { return NewJSONArgsFunc(failingFunc, passThroughHandler)(ctx, []byte(`{}`)) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = tt.call() })
			assert.Equal(t, resultErr, err)
		})
	}
}

func TestArgsFuncs_HandlerErrorStopsHandlers(t *testing.T) {
	// A results handler returning an error different from the result error
	// must stop the handler chain and that error must be returned,
	// also when both errors have the same non-comparable type.
	resultErr := multiErr{errors.New("result")}
	handlerErr := multiErr{errors.New("handler")}
	failingFunc := MustReflectWrapper(func() error { return resultErr })
	failingHandler := ResultsHandlerFunc(func(context.Context, []any, error) error { return handlerErr })
	unreachableHandler := ResultsHandlerFunc(func(context.Context, []any, error) error {
		t.Error("handler after failing handler must not be called")
		return nil
	})
	var err error
	require.NotPanics(t, func() {
		err = NewStringArgsFunc(failingFunc, failingHandler, unreachableHandler)(context.Background())
	})
	assert.Equal(t, handlerErr, err)
}
