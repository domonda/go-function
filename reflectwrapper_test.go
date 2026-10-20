package function

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReflectWrapper(t *testing.T) {
	type description struct {
		NumArgs     int
		ContextArg  bool
		NumResults  int
		ErrorResult bool
		ArgNames    []string
		ArgTypes    []reflect.Type
		ResultTypes []reflect.Type
	}

	f0 := func() {}
	f0desc := description{
		NumArgs:     0,
		ContextArg:  false,
		NumResults:  0,
		ErrorResult: false,
		ArgNames:    nil,
		ArgTypes:    nil,
		ResultTypes: nil,
	}

	f1 := func(i int) {}
	f1desc := description{
		NumArgs:     1,
		ContextArg:  false,
		NumResults:  0,
		ErrorResult: false,
		ArgNames:    []string{"i"},
		ArgTypes:    []reflect.Type{reflect.TypeFor[int]()},
		ResultTypes: nil,
	}

	f1r := func(i int) int { return i * 2 }
	f1rdesc := description{
		NumArgs:     1,
		ContextArg:  false,
		NumResults:  1,
		ErrorResult: false,
		ArgNames:    []string{"i"},
		ArgTypes:    []reflect.Type{reflect.TypeFor[int]()},
		ResultTypes: []reflect.Type{reflect.TypeFor[int]()},
	}

	ferr := func(ctx context.Context, i int, e error) (int, error) { return i * 2, e }
	ferrdesc := description{
		NumArgs:     3,
		ContextArg:  true,
		NumResults:  2, // including the error result like documented by Description
		ErrorResult: true,
		ArgNames:    []string{"ctx", "i", "e"},
		ArgTypes:    []reflect.Type{typeOfContext, reflect.TypeFor[int](), typeOfError},
		ResultTypes: []reflect.Type{reflect.TypeFor[int](), typeOfError},
	}

	type args struct {
		function any
		argNames []string
	}
	type call struct {
		args         []any
		argsStrings  []string
		argsNamedStr map[string]string
		argsJSON     []byte
		results      []any
		wantErr      bool
	}
	tests := []struct {
		name    string
		args    args
		want    *reflectWrapper
		wantErr bool
		call    call
		desc    description
	}{
		{
			name: "func() {}",
			args: args{
				function: f0,
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f0),
				funcType: reflect.TypeOf(f0),
				argNames: nil,
			},
			wantErr: false,
			call: call{
				args:         nil,
				argsStrings:  nil,
				argsNamedStr: nil,
				argsJSON:     []byte(`{}`),
				results:      []any{},
				wantErr:      false,
			},
			desc: f0desc,
		},
		{
			name: "func(i int) {}",
			args: args{
				function: f1,
				argNames: []string{"i"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f1),
				funcType: reflect.TypeOf(f1),
				argNames: []string{"i"},
			},
			wantErr: false,
			call: call{
				args:         []any{666},
				argsStrings:  []string{"666"},
				argsNamedStr: map[string]string{"i": "666"},
				argsJSON:     []byte(`{"i":666}`),
				results:      []any{},
				wantErr:      false,
			},
			desc: f1desc,
		},
		{
			name: "func(i int) int",
			args: args{
				function: f1r,
				argNames: []string{"i"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f1r),
				funcType: reflect.TypeOf(f1r),
				argNames: []string{"i"},
			},
			wantErr: false,
			call: call{
				args:         []any{666},
				argsStrings:  []string{"666"},
				argsNamedStr: map[string]string{"i": "666"},
				argsJSON:     []byte(`{"i":666}`),
				results:      []any{666 * 2},
				wantErr:      false,
			},
			desc: f1rdesc,
		},
		{
			name: "func(ctx context.Context, i int, e error = nil) (int, error)",
			args: args{
				function: ferr,
				argNames: []string{"ctx", "i", "e"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(ferr),
				funcType: reflect.TypeOf(ferr),
				argNames: []string{"ctx", "i", "e"},
			},
			wantErr: false,
			call: call{
				args:         []any{666, nil},
				argsStrings:  []string{"666", ""},
				argsNamedStr: map[string]string{"i": "666", "e": ""},
				argsJSON:     []byte(`{"i":666,"e":null}`),
				results:      []any{666 * 2},
				wantErr:      false,
			},
			desc: ferrdesc,
		},
		{
			name: "func(ctx context.Context, i int, e error = ERROR) (int, error)",
			args: args{
				function: ferr,
				argNames: []string{"ctx", "i", "e"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(ferr),
				funcType: reflect.TypeOf(ferr),
				argNames: []string{"ctx", "i", "e"},
			},
			wantErr: false,
			call: call{
				args:         []any{666, errors.New("ERROR")},
				argsStrings:  []string{"666", "ERROR"},
				argsNamedStr: map[string]string{"i": "666", "e": "ERROR"},
				argsJSON:     []byte(`{"i":666,"e":"ERROR"}`),
				results:      []any{666 * 2},
				wantErr:      true,
			},
			desc: ferrdesc,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newReflectWrapper(tt.args.function, tt.args.argNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("newReflectWrapper() error = %#v, wantErr = %#v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newReflectWrapper() = %#v, want = %#v", got, tt.want)
			}

			if got.NumArgs() != tt.desc.NumArgs {
				t.Errorf("NumArgs(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.NumArgs(), tt.desc.NumArgs)
			}
			if got.ContextArg() != tt.desc.ContextArg {
				t.Errorf("ContextArg(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ContextArg(), tt.desc.ContextArg)
			}
			if got.NumResults() != tt.desc.NumResults {
				t.Errorf("NumResults(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.NumResults(), tt.desc.NumResults)
			}
			if got.ErrorResult() != tt.desc.ErrorResult {
				t.Errorf("ErrorResult(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ErrorResult(), tt.desc.ErrorResult)
			}
			if !reflect.DeepEqual(got.ArgNames(), tt.desc.ArgNames) {
				t.Errorf("ArgNames(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ArgNames(), tt.desc.ArgNames)
			}
			if !reflect.DeepEqual(got.ArgTypes(), tt.desc.ArgTypes) {
				t.Errorf("ArgTypes(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ArgTypes(), tt.desc.ArgTypes)
			}
			if !reflect.DeepEqual(got.ResultTypes(), tt.desc.ResultTypes) {
				t.Errorf("ResultTypes(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ResultTypes(), tt.desc.ResultTypes)
			}

			gotResults, gotErr := got.Call(context.Background(), tt.call.args)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.Call() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.Call() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithStrings(context.Background(), tt.call.argsStrings...)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithStrings() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithStrings() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithNamedStrings(context.Background(), tt.call.argsNamedStr)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithNamedStrings() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithNamedStrings() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithJSON(context.Background(), tt.call.argsJSON)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithJSON() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithJSON() = %#v, want %#v", gotResults, tt.call.results)
			}
		})
	}
}

func TestReflectWrapper_Variadic(t *testing.T) {
	// The variadic parameter is represented as one []T argument in all
	// Call* methods, so the wrapper must call it with CallSlice instead
	// of passing the slice as a single element (which panics in reflect).
	// A missing or nil variadic argument must call the function without
	// variadic elements instead of failing.
	w := MustReflectWrapper(
		func(prefix string, xs ...int) string { return fmt.Sprintf("%s%v", prefix, xs) },
		"prefix", "xs",
	)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() ([]any, error)
		want string
	}{
		{name: "Call", call: func() ([]any, error) { return w.Call(ctx, []any{"p", []int{1, 2}}) }, want: "p[1 2]"},
		{name: "Call missing variadic", call: func() ([]any, error) { return w.Call(ctx, []any{"p"}) }, want: "p[]"},
		{name: "Call nil variadic", call: func() ([]any, error) { return w.Call(ctx, []any{"p", nil}) }, want: "p[]"},
		// Like Go call syntax every string from the variadic position on is one element
		{name: "CallWithStrings single variadic element", call: func() ([]any, error) { return w.CallWithStrings(ctx, "p", "7") }, want: "p[7]"},
		{name: "CallWithStrings missing variadic", call: func() ([]any, error) { return w.CallWithStrings(ctx, "p") }, want: "p[]"},
		{name: "CallWithStrings variadic elements", call: func() ([]any, error) { return w.CallWithStrings(ctx, "p", "1", "2", "3") }, want: "p[1 2 3]"},
		{name: "CallWithNamedStrings", call: func() ([]any, error) {
			return w.CallWithNamedStrings(ctx, map[string]string{"prefix": "p", "xs": "[1,2]"})
		}, want: "p[1 2]"},
		{name: "CallWithNamedStrings missing variadic", call: func() ([]any, error) {
			return w.CallWithNamedStrings(ctx, map[string]string{"prefix": "p"})
		}, want: "p[]"},
		{name: "CallWithJSON", call: func() ([]any, error) {
			return w.CallWithJSON(ctx, []byte(`{"prefix":"p","xs":[1,2]}`))
		}, want: "p[1 2]"},
		{name: "CallWithJSON missing variadic", call: func() ([]any, error) {
			return w.CallWithJSON(ctx, []byte(`{"prefix":"p"}`))
		}, want: "p[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				results []any
				err     error
			)
			require.NotPanics(t, func() { results, err = tt.call() })
			require.NoError(t, err)
			assert.Equal(t, []any{tt.want}, results)
		})
	}
}

func TestReflectWrapper_CallArgChecks(t *testing.T) {
	// Call receives untyped []any arguments from callers like CLI or HTTP glue code,
	// so a wrong number or type of arguments must be reported as an error
	// instead of crashing the process with a reflect or index-out-of-range panic.
	// Missing and untyped nil arguments are still passed as zero values.
	ctx := context.Background()
	withCtx := MustReflectWrapper(
		func(ctx context.Context, a int, s string) string { return fmt.Sprintf("%d:%q", a, s) },
		"ctx", "a", "s",
	)
	variadic := MustReflectWrapper(
		func(xs ...int) string { return fmt.Sprint(xs) },
		"xs",
	)
	withAny := MustReflectWrapper(
		func(v any) string { return fmt.Sprint(v) },
		"v",
	)
	tests := []struct {
		name    string
		w       Wrapper
		args    []any
		want    []any
		wantErr bool
	}{
		{name: "all args", w: withCtx, args: []any{1, "x"}, want: []any{`1:"x"`}},
		{name: "missing args are zero values", w: withCtx, args: []any{1}, want: []any{`1:""`}},
		{name: "untyped nil args are zero values", w: withCtx, args: []any{nil, nil}, want: []any{`0:""`}},
		{name: "any param accepts every type", w: withAny, args: []any{3.5}, want: []any{"3.5"}},
		{name: "variadic as slice", w: variadic, args: []any{[]int{1, 2}}, want: []any{"[1 2]"}},
		// wantErr
		{name: "too many args", w: withCtx, args: []any{1, "x", "extra"}, wantErr: true},
		{name: "wrong arg type", w: withCtx, args: []any{"x"}, wantErr: true},
		{name: "convertible but not assignable arg type", w: withCtx, args: []any{int64(1)}, wantErr: true},
		{name: "variadic element instead of slice", w: variadic, args: []any{1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				results []any
				err     error
			)
			require.NotPanics(t, func() { results, err = tt.w.Call(ctx, tt.args) })
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, results)
		})
	}
}

func TestReflectWrapper_NotAFunction(t *testing.T) {
	// Passing a nil or non-function value is a programming error that must
	// be reported with the documented "expected function" error,
	// not with an obscure reflect panic about a zero Value.
	_, err := ReflectWrapper(nil)
	assert.EqualError(t, err, "expected function but got <nil>")

	_, err = ReflectWrapper(42)
	assert.EqualError(t, err, "expected function but got int")

	assert.PanicsWithError(t, "expected function but got <nil>", func() { MustReflectWrapper(nil) })
}

func TestReflectWrapper_ResultsIncludeError(t *testing.T) {
	// Description documents NumResults and ResultTypes as including
	// a trailing error result. Code using a Description must not have
	// to know if it was created by ReflectWrapper, ReflectDescription
	// or generated code, so all of them must describe results the same way.
	// The Call* methods still return the error separately and
	// not as part of the results slice.
	tests := []struct {
		name            string
		function        any
		wantNumResults  int
		wantResultTypes []reflect.Type
		wantResults     []any
	}{
		{
			name:            "no results",
			function:        func() {},
			wantNumResults:  0,
			wantResultTypes: nil,
			wantResults:     []any{},
		},
		{
			name:            "only error",
			function:        func() error { return nil },
			wantNumResults:  1,
			wantResultTypes: []reflect.Type{typeOfError},
			wantResults:     []any{},
		},
		{
			name:            "int and error",
			function:        func() (int, error) { return 1, nil },
			wantNumResults:  2,
			wantResultTypes: []reflect.Type{reflect.TypeFor[int](), typeOfError},
			wantResults:     []any{1},
		},
		{
			name:            "error not last",
			function:        func() (error, int) { return nil, 1 },
			wantNumResults:  2,
			wantResultTypes: []reflect.Type{typeOfError, reflect.TypeFor[int]()},
			wantResults:     []any{nil, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := ReflectWrapper(tt.function)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNumResults, w.NumResults(), "NumResults")
			assert.Equal(t, tt.wantResultTypes, w.ResultTypes(), "ResultTypes")

			d, err := ReflectDescription("f", tt.function)
			require.NoError(t, err)
			assert.Equal(t, d.NumResults(), w.NumResults(), "NumResults same as ReflectDescription")
			assert.Equal(t, d.ErrorResult(), w.ErrorResult(), "ErrorResult same as ReflectDescription")

			results, err := w.Call(context.Background(), nil)
			require.NoError(t, err)
			assert.Equal(t, tt.wantResults, results, "Call results without error")
		})
	}
}

func TestReflectWrapper_CallWithStringsArgChecks(t *testing.T) {
	// Command line dispatchers pass all remaining arguments to CallWithStrings,
	// so extra arguments must be an error instead of being silently dropped,
	// and a variadic element that can't be parsed must name the argument.
	ctx := context.Background()

	greet := MustReflectWrapper(func(name string) string { return "Hello " + name }, "name")
	_, err := greet.CallWithStrings(ctx, "Alice", "Bob")
	require.Error(t, err, "too many arguments")

	sum := MustReflectWrapper(
		func(xs ...int) int {
			s := 0
			for _, x := range xs {
				s += x
			}
			return s
		},
		"xs",
	)
	_, err = sum.CallWithStrings(ctx, "1", "two", "3")
	require.Error(t, err)
	var errParse ErrParseArgString
	require.ErrorAs(t, err, &errParse)
	assert.Equal(t, "xs", errParse.ArgName)

	identity := MustReflectWrapper(func(xs ...any) []any { return xs }, "xs")
	results, err := identity.CallWithStrings(ctx, "a", "1")
	require.NoError(t, err)
	assert.Equal(t, []any{[]any{"a", "1"}}, results, "elements of type any get the unparsed strings")
}

func TestReflectWrapper_CallWithStringsVariadicSingleString(t *testing.T) {
	// Like the Go call f(x) means f([]T{x}...), a single string at the variadic
	// position is one element and not a slice literal. Parsing it as slice literal
	// broke command line arguments like a Printf value or a log message
	// that happen to look like JSON or start with '['.
	ctx := context.Background()
	strs := MustReflectWrapper(func(xs ...string) []string { return xs }, "xs")

	t.Run("any element", func(t *testing.T) {
		printf := MustReflectWrapper(func(format string, args ...any) string { return fmt.Sprintf(format, args...) }, "format", "args")
		results, err := printf.CallWithStrings(ctx, "%v", "hello")
		require.NoError(t, err)
		assert.Equal(t, []any{"hello"}, results)
	})

	t.Run("string element", func(t *testing.T) {
		for _, str := range []string{"[WARN]", "[WARN] disk", `["a","b"]`, ""} {
			results, err := strs.CallWithStrings(ctx, str)
			if assert.NoError(t, err, "string %q", str) {
				assert.Equal(t, []any{[]string{str}}, results, "string %q", str)
			}
		}
	})

	t.Run("slice literal is no int element", func(t *testing.T) {
		ints := MustReflectWrapper(func(xs ...int) []int { return xs }, "xs")
		_, err := ints.CallWithStrings(ctx, "[1,2,3]")
		var errParse ErrParseArgString
		require.ErrorAs(t, err, &errParse)
		assert.Equal(t, "xs", errParse.ArgName)
	})

	t.Run("CallWithNamedStrings parses slice literal", func(t *testing.T) {
		// CallWithNamedStrings has only one string per argument,
		// so the variadic argument is still parsed as slice literal
		results, err := strs.CallWithNamedStrings(ctx, map[string]string{"xs": "[WARN]"})
		require.NoError(t, err)
		assert.Equal(t, []any{[]string{"WARN"}}, results)
	})
}

func TestReflectWrapper_NameAndString(t *testing.T) {
	// Name and String are shown in error messages and help texts,
	// so they must name the function and its signature,
	// not the formatting of a reflect.Value
	w := MustReflectWrapper(strings.ToUpper, "s")
	assert.Equal(t, "strings.ToUpper", w.Name())
	assert.Equal(t, "strings.ToUpper(s string) string", w.String())

	// Generated wrappers and ReflectDescription use the same format
	w = MustReflectWrapper(fmt.Fprintf, "w", "format", "a")
	assert.Equal(t, "fmt.Fprintf(w io.Writer, format string, a ...any) (int, error)", w.String())
	d, err := ReflectDescription("fmt.Fprintf", fmt.Fprintf)
	require.NoError(t, err)
	assert.Equal(t, "fmt.Fprintf(a0 io.Writer, a1 string, a2 ...any) (int, error)", d.String())
}

func TestReflectWrapper_CallWithStringsContextArg(t *testing.T) {
	// CLI commands usually take a context.Context that is not one of the strings,
	// so its offset must not drop strings or let too many strings pass
	ctx := context.Background()
	variadic := MustReflectWrapper(func(ctx context.Context, n int, nums ...int) string { return fmt.Sprint(n, nums) }, "ctx", "n", "nums")
	results, err := variadic.CallWithStrings(ctx, "1", "2", "3")
	require.NoError(t, err)
	assert.Equal(t, []any{"1 [2 3]"}, results, "no variadic element may be dropped")

	greet := MustReflectWrapper(func(ctx context.Context, name string) string { return name }, "ctx", "name")
	_, err = greet.CallWithStrings(ctx, "Alice", "extra")
	assert.Error(t, err, "too many strings for a function with context")
}

func TestReflectWrapper_CallWithJSONArgError(t *testing.T) {
	// An invalid value of one argument must name that argument
	// like generated wrappers do,
	// while malformed JSON is an error of the whole arguments object
	w := MustReflectWrapper(func(amount int, e error) {}, "amount", "e")
	ctx := context.Background()

	_, err := w.CallWithJSON(ctx, []byte(`{"amount":"x"}`))
	var errArg ErrParseArgJSON
	require.ErrorAs(t, err, &errArg)
	assert.Equal(t, "amount", errArg.ArgName)

	_, err = w.CallWithJSON(ctx, []byte(`{"e":1}`))
	require.ErrorAs(t, err, &errArg)
	assert.Equal(t, "e", errArg.ArgName)

	_, err = w.CallWithJSON(ctx, []byte(`{`))
	assert.ErrorAs(t, err, new(ErrParseArgsJSON))
}

func TestReflectWrapper_NameOfPackageWithPath(t *testing.T) {
	// The runtime reports the full package path,
	// Name keeps only its last element like a package qualifier
	w := MustReflectWrapper(assert.True, "t", "value", "msgAndArgs")
	assert.Equal(t, "assert.True", w.Name())
}

func TestReflectWrapper_VariadicDescription(t *testing.T) {
	// ArgTypes returns the slice type for both functions,
	// so only Variadic tells callers like the cli help
	// that CallWithStrings takes the elements as separate strings
	variadic := MustReflectWrapper(func(xs ...int) {}, "xs")
	slice := MustReflectWrapper(func(xs []int) {}, "xs")
	noArgs := MustReflectWrapper(func() {})
	assert.Equal(t, variadic.ArgTypes(), slice.ArgTypes())
	assert.True(t, variadic.Variadic())
	assert.False(t, slice.Variadic())
	assert.False(t, noArgs.Variadic())
}

func TestReflectWrapper_ArgNamesAreCopies(t *testing.T) {
	// Modifying the argNames passed to ReflectWrapper
	// or returned by ArgNames must not rename the arguments of the wrapper
	argNames := []string{"a"}
	w := MustReflectWrapper(func(a int) int { return a }, argNames...)
	argNames[0] = "changed"
	w.ArgNames()[0] = "changed"
	assert.Equal(t, []string{"a"}, w.ArgNames())
	assert.Contains(t, w.String(), "(a int)")
}

func Test_qualifiedFuncName(t *testing.T) {
	// The runtime reports the import path, which is qualified
	// with the package name that goimports assumes for it,
	// like generated wrappers use the package name
	tests := []struct {
		runtimeName string
		want        string
	}{
		{runtimeName: "strings.ToUpper", want: "strings.ToUpper"},
		{runtimeName: "main.main", want: "main.main"},
		{runtimeName: "github.com/domonda/go-function.ScanString", want: "function.ScanString"},
		{runtimeName: "github.com/domonda/go-function_test.TestX", want: "function_test.TestX"},
		{runtimeName: "example.com/demo/dashed-dir.F", want: "dashed.F"},
		{runtimeName: "example.com/demo/v2.F", want: "demo.F"},
		// The runtime escapes dots in the last element of the import path
		{runtimeName: "gopkg.in/yaml%2ev3.Marshal", want: "yaml.Marshal"},
		{runtimeName: "github.com/a/go-b.(*T).M", want: "b.(*T).M"},
		{runtimeName: "github.com/a/go-b.T.M-fm", want: "b.T.M-fm"},
		{runtimeName: "github.com/a/go-b.F.func1", want: "b.F.func1"},
		{runtimeName: "github.com/a/go-b.F[...]", want: "b.F[...]"},
		{runtimeName: "github.com/a/b.F[github.com/c/d.T]", want: "b.F[github.com/c/d.T]"},
		// Not an identifier after removing the "go-" prefix
		{runtimeName: "example.com/go-.F", want: "go-.F"},
		{runtimeName: "noDot", want: "noDot"},
	}
	for _, tt := range tests {
		t.Run(tt.runtimeName, func(t *testing.T) {
			assert.Equal(t, tt.want, qualifiedFuncName(tt.runtimeName))
		})
	}

	// The package of this test is named function, its import path ends with go-function
	w := MustReflectWrapper(isNilSource, "trimmedSrc")
	assert.Equal(t, "function.isNilSource", w.Name())
	assert.Equal(t, "function.isNilSource(trimmedSrc string) bool", w.String())
}

func TestReflectWrapper_InvalidFunctionOrNames(t *testing.T) {
	// Invalid input is an error of ReflectWrapper
	// instead of a panic or wrong arguments when the wrapper is called
	var nilFunc func(int)
	_, err := ReflectWrapper(nilFunc, "a")
	assert.EqualError(t, err, "nil function of type func(int) passed")

	_, err = ReflectWrapper(func(a, b int) {}, "a", "a")
	assert.EqualError(t, err, `argument name "a" passed more than once for func(int, int)`)

	_, err = ReflectWrapper(func(a, b int) {}, "a", "")
	assert.EqualError(t, err, "empty name passed for argument 1 of func(int, int)")

	// Like in Go, "_" can be used for multiple arguments
	w, err := ReflectWrapper(func(a, b, c int) int { return c }, "_", "_", "c")
	require.NoError(t, err)
	results, err := w.CallWithStrings(context.Background(), "1", "2", "3")
	require.NoError(t, err)
	assert.Equal(t, []any{3}, results)
}
