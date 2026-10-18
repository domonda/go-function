package function

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_unmarshalJSONFunctionArgs(t *testing.T) {
	description := func(f any) Description {
		info, err := ReflectDescription("f", f)
		if err != nil {
			panic(err)
		}
		return info
	}
	type args struct {
		f          Description
		jsonObject []byte
	}
	tests := []struct {
		name     string
		args     args
		wantArgs []any
		wantErr  bool
	}{
		{
			name: "empty",
			args: args{
				f:          description(func() {}),
				jsonObject: []byte(`{"a0": "ignored"}`),
			},
			wantArgs: []any{},
		},
		{
			name: "default 0",
			args: args{
				f:          description(func(string, int) {}),
				jsonObject: []byte(`{"a0": "default"}`),
			},
			wantArgs: []any{"default", 0},
		},
		{
			name: "hello 666",
			args: args{
				f:          description(func(string, int) {}),
				jsonObject: []byte(`{"a0": "hello", "a1": 666, "a2": "ignored"}`),
			},
			wantArgs: []any{"hello", 666},
		},
		{
			name: "ptr",
			args: args{
				f:          description(func(*string, *string, any) {}),
				jsonObject: []byte(`{"a0": "", "a2": null}`),
			},
			wantArgs: []any{new(string), (*string)(nil), nil},
		},
		{
			// Call implementations expect args without the context.Context
			// because the context is passed separately, so the ctx argument
			// must not be part of the unmarshalled args.
			name: "context arg is not part of args",
			args: args{
				f:          description(func(context.Context, int) {}),
				jsonObject: []byte(`{"a1": 21}`),
			},
			wantArgs: []any{21},
		},
		{
			// A JSON value named like the context argument can't
			// be unmarshalled as context.Context and must be ignored.
			name: "JSON value for context arg is ignored",
			args: args{
				f:          description(func(context.Context, int) {}),
				jsonObject: []byte(`{"a0": "not a context", "a1": 21}`),
			},
			wantArgs: []any{21},
		},
		{
			// json.Unmarshal can't unmarshal into the error interface,
			// so like in ReflectWrapper.CallWithJSON an error argument
			// is a JSON string and an empty string or null is no error
			name: "error args",
			args: args{
				f:          description(func(error, error, error, error) {}),
				jsonObject: []byte(`{"a0": "boom", "a1": "", "a2": null}`),
			},
			wantArgs: []any{errors.New("boom"), nil, nil, nil},
		},
		// wantErr
		{
			name: "error arg not a JSON string",
			args: args{
				f:          description(func(error) {}),
				jsonObject: []byte(`{"a0": 1}`),
			},
			wantErr: true,
		},
		{
			name: "JSON array",
			args: args{
				f:          description(func() {}),
				jsonObject: []byte(`[]`),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, err := unmarshalJSONFunctionArgs(tt.args.f, tt.args.jsonObject)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalJSONFunctionArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("unmarshalJSONFunctionArgs() = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestCallFunctionWithJSONArgs(t *testing.T) {
	// CallFunctionWithJSONArgs (and NewJSONArgsFunc which uses it) must work
	// for functions with a context.Context argument: the caller's ctx is
	// passed to the function and only the other arguments come from JSON.
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "from caller ctx")
	tests := []struct {
		name       string
		f          Wrapper
		jsonObject string
		want       []any
	}{
		{
			name:       "without context arg",
			f:          MustReflectWrapper(func(a int) int { return a * 2 }, "a"),
			jsonObject: `{"a":21}`,
			want:       []any{42},
		},
		{
			name: "with context arg",
			f: MustReflectWrapper(
				func(ctx context.Context, a int) (int, any) { return a * 2, ctx.Value(ctxKey{}) },
				"ctx", "a",
			),
			jsonObject: `{"a":21}`,
			want:       []any{42, "from caller ctx"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				results []any
				err     error
			)
			require.NotPanics(t, func() { results, err = CallFunctionWithJSONArgs(ctx, tt.f, []byte(tt.jsonObject)) })
			require.NoError(t, err)
			assert.Equal(t, tt.want, results)
		})
	}
}

func TestCallFunctionWithJSONArgs_ErrorArg(t *testing.T) {
	// CallFunctionWithJSONArgs (behind NewJSONArgsFunc) and Wrapper.CallWithJSON
	// both call a function with JSON arguments, so they must accept
	// the same JSON for an error argument and pass the same error.
	f := MustReflectWrapper(
		func(err error) string {
			if err == nil {
				return "<nil>"
			}
			return err.Error()
		},
		"err",
	)
	tests := []struct {
		jsonObject string
		want       []any
	}{
		{jsonObject: `{"err": "boom"}`, want: []any{"boom"}},
		{jsonObject: `{"err": ""}`, want: []any{"<nil>"}},
		{jsonObject: `{"err": null}`, want: []any{"<nil>"}},
		{jsonObject: `{}`, want: []any{"<nil>"}},
	}
	for _, tt := range tests {
		t.Run(tt.jsonObject, func(t *testing.T) {
			wantResults, err := f.CallWithJSON(t.Context(), []byte(tt.jsonObject))
			require.NoError(t, err)
			require.Equal(t, tt.want, wantResults)

			results, err := CallFunctionWithJSONArgs(t.Context(), f, []byte(tt.jsonObject))
			require.NoError(t, err)
			assert.Equal(t, wantResults, results)
		})
	}
}
