package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/domonda/go-function"
)

// captureStdout returns what f prints to os.Stdout,
// including the colored output of UsageColor and DescriptionColor
// which is written without color codes.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	stdout, colorOutput, noColor := os.Stdout, color.Output, color.NoColor
	os.Stdout, color.Output, color.NoColor = writer, writer, true
	defer func() {
		os.Stdout, color.Output, color.NoColor = stdout, colorOutput, noColor
	}()

	output := make(chan string)
	go func() {
		b, _ := io.ReadAll(reader)
		output <- string(b)
	}()
	f()
	require.NoError(t, writer.Close())
	return <-output
}

type ctxKey struct{}

func TestStringArgsDispatcher_AddCommand(t *testing.T) {
	disp := NewStringArgsDispatcher("myapp")
	noop := function.MustReflectWrapper(func() {})

	require.NoError(t, disp.AddCommand("b", "", noop))
	require.NoError(t, disp.AddCommand("a", "", noop))
	assert.True(t, disp.HasCommand("a"))
	assert.False(t, disp.HasCommand("c"))
	assert.False(t, disp.HasDefaultCommand())
	assert.Equal(t, []string{"a", "b"}, disp.Commands(), "sorted for stable help output")

	// A command must not silently replace another one
	assert.Error(t, disp.AddCommand("a", "", noop))
	assert.Panics(t, func() { disp.MustAddCommand("a", "", noop) })
	require.NoError(t, disp.AddDefaultCommand("", noop))
	assert.Error(t, disp.AddDefaultCommand("", noop), "default command must not silently replace another one")

	// Commands must be a single shell word without shell metacharacters
	for _, command := range []string{"", "two words", "pipe|", "and&", "semi;", "(paren)", "<redirect>"} {
		assert.Error(t, disp.AddCommand(command, "", noop), "command %q", command)
	}
}

func TestStringArgsDispatcher_Dispatch(t *testing.T) {
	var (
		gotCtx           context.Context
		gotEnv           string
		gotVersion       int
		gotForce         bool
		loggedCommand    string
		loggedArgs       []string
		handledResults   []any
		handledResultErr error
	)
	deploy := func(ctx context.Context, env string, version int, force bool) (string, error) {
		gotCtx, gotEnv, gotVersion, gotForce = ctx, env, version, force
		if env == "fail" {
			return "", errors.New("deploy failed")
		}
		return "deployed", nil
	}
	logger := StringArgsCommandLoggerFunc(func(command string, args []string) {
		loggedCommand, loggedArgs = command, args
	})
	resultsHandler := function.ResultsHandlerFunc(func(ctx context.Context, results []any, resultErr error) error {
		handledResults, handledResultErr = results, resultErr
		return resultErr
	})
	disp := NewStringArgsDispatcher("myapp", logger)
	disp.MustAddCommand("deploy", "Deploy a service", function.MustReflectWrapper(deploy, "ctx", "env", "version", "force"), resultsHandler)

	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	err := disp.Dispatch(ctx, "deploy", "production", "42", "true")
	require.NoError(t, err)

	// String arguments are parsed into the types of the function arguments
	assert.Equal(t, "production", gotEnv)
	assert.Equal(t, 42, gotVersion)
	assert.Equal(t, true, gotForce)
	assert.Equal(t, "value", gotCtx.Value(ctxKey{}), "the context of the caller is passed to the function")
	assert.Equal(t, "deploy", loggedCommand)
	assert.Equal(t, []string{"production", "42", "true"}, loggedArgs)
	assert.Equal(t, []any{"deployed"}, handledResults)
	assert.NoError(t, handledResultErr)

	t.Run("function error", func(t *testing.T) {
		err := disp.Dispatch(ctx, "deploy", "fail", "1", "false")
		assert.EqualError(t, err, "deploy failed")
		assert.EqualError(t, handledResultErr, "deploy failed", "results handlers see the error of the function")
	})

	t.Run("unparseable argument", func(t *testing.T) {
		gotEnv = ""
		err := disp.Dispatch(ctx, "deploy", "staging", "not-a-number", "false")
		var errParse function.ErrParseArgString
		require.ErrorAs(t, err, &errParse)
		assert.Equal(t, "version", errParse.ArgName)
		assert.Empty(t, gotEnv, "function must not be called with unparsed arguments")
	})

	t.Run("unknown command", func(t *testing.T) {
		loggedCommand = ""
		err := disp.Dispatch(ctx, "unknown")
		assert.Equal(t, ErrCommandNotFound("unknown"), err)
		assert.True(t, IsErrCommandNotFound(err))
		assert.Empty(t, loggedCommand, "unknown commands are not logged as executed")
	})
}

func TestStringArgsDispatcher_DispatchCombinedCommandAndArgs(t *testing.T) {
	var (
		calledDefault bool
		defaultCtx    context.Context
		greeted       string
	)
	disp := NewStringArgsDispatcher("myapp")
	disp.MustAddDefaultCommand("Run the default", function.MustReflectWrapper(func(ctx context.Context) {
		calledDefault, defaultCtx = true, ctx
	}))
	disp.MustAddCommand("greet", "Greet someone", function.MustReflectWrapper(func(name string) {
		greeted = name
	}, "name"))
	assert.True(t, disp.HasDefaultCommand())

	ctx := context.WithValue(context.Background(), ctxKey{}, "value")

	// Running the program without arguments runs the default command
	command, err := disp.DispatchCombinedCommandAndArgs(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultCommand, command)
	assert.True(t, calledDefault)
	// The default command must get the context of the caller
	// for cancellation and values like any other command
	require.NotNil(t, defaultCtx)
	assert.Equal(t, "value", defaultCtx.Value(ctxKey{}))

	command, err = disp.DispatchCombinedCommandAndArgs(ctx, []string{"greet", "Alice"})
	require.NoError(t, err)
	assert.Equal(t, "greet", command)
	assert.Equal(t, "Alice", greeted)

	command, err = disp.DispatchCombinedCommandAndArgs(ctx, []string{"unknown"})
	assert.Equal(t, "unknown", command, "the command is returned for error messages")
	assert.True(t, IsErrCommandNotFound(err))

	t.Run("without default command", func(t *testing.T) {
		disp := NewStringArgsDispatcher("myapp")
		_, err := disp.DispatchCombinedCommandAndArgs(ctx, nil)
		assert.True(t, IsErrCommandNotFound(err))
		assert.True(t, IsErrCommandNotFound(disp.DispatchDefaultCommand(ctx)))
	})

	t.Run("DispatchDefaultCommand", func(t *testing.T) {
		calledDefault, defaultCtx = false, nil
		require.NoError(t, disp.DispatchDefaultCommand(ctx))
		assert.True(t, calledDefault)
		// Like Dispatch, the default command gets the context of the caller
		require.NotNil(t, defaultCtx)
		assert.Equal(t, "value", defaultCtx.Value(ctxKey{}))
	})
}

func TestStringArgsDispatcher_PrintCompletion(t *testing.T) {
	disp := NewStringArgsDispatcher("myapp")
	noop := function.MustReflectWrapper(func() {})
	disp.MustAddCommand("start", "", noop)
	disp.MustAddCommand("stop", "", noop)
	disp.MustAddCommand("status", "", noop)
	disp.MustAddCommand("deploy", "", noop)

	tests := []struct {
		args []string
		want string
	}{
		{args: nil, want: "myapp deploy\nmyapp start\nmyapp status\nmyapp stop\n"},
		{args: []string{"st"}, want: "myapp start\nmyapp status\nmyapp stop\n"},
		{args: []string{"sta"}, want: "myapp start\nmyapp status\n"},
		{args: []string{"x"}, want: ""},
	}
	for _, tt := range tests {
		got := captureStdout(t, func() { disp.PrintCompletion(tt.args) })
		assert.Equal(t, tt.want, got, "args %#v", tt.args)
	}

	// The default command completes to the base command without a trailing space
	disp.MustAddDefaultCommand("", noop)
	got := captureStdout(t, func() { disp.PrintCompletion(nil) })
	assert.Equal(t, "myapp\nmyapp deploy\nmyapp start\nmyapp status\nmyapp stop\n", got)
}

func TestStringArgsDispatcher_PrintCommands(t *testing.T) {
	disp := NewStringArgsDispatcher("myapp")
	assert.Empty(t, captureStdout(t, disp.PrintCommandsUsageIntro), "no intro without commands")

	disp.MustAddCommand("deploy", "Deploy a service", function.MustReflectWrapper(func(env string, version *int) {}, "env", "version"))

	got := captureStdout(t, disp.PrintCommandsUsageIntro)

	// The usage shows how to call each command with the argument names and types
	want := "Commands:\n" +
		"  myapp deploy <env:string> <version:int>\n" +
		"      Deploy a service\n" +
		"\n"
	assert.Equal(t, want, got)
}

// argDescriptionsWrapper is a Wrapper with argument descriptions
// like the ones generated wrappers take from the doc comment.
type argDescriptionsWrapper struct {
	function.Wrapper
	descriptions []string
}

func (w argDescriptionsWrapper) ArgDescriptions() []string { return w.descriptions }

func TestStringArgsDispatcher_PrintCommands_contextArg(t *testing.T) {
	// The dispatcher passes the context argument,
	// so the usage doesn't show it as command line argument
	disp := NewStringArgsDispatcher("myapp")
	disp.MustAddCommand("convert", "Convert a temperature", argDescriptionsWrapper{
		Wrapper:      function.MustReflectWrapper(func(ctx context.Context, degrees float64, to string) {}, "ctx", "degrees", "to"),
		descriptions: []string{"", "The temperature", "The unit"},
	})
	disp.MustAddCommand("migrate", "Migrate the database", function.MustReflectWrapper(func(ctx context.Context) {}))
	disp.MustAddDefaultCommand("Print the commands", function.VoidFuncWrapper(func() {}))

	got := captureStdout(t, disp.PrintCommands)

	// No trailing space after the default command and commands without arguments
	want := "  myapp\n" +
		"      Print the commands\n" +
		"\n" +
		"  myapp convert <degrees:float64> <to:string>\n" +
		"      Convert a temperature\n" +
		"          <degrees:float64> The temperature\n" +
		"          <to:string> The unit\n" +
		"\n" +
		"  myapp migrate\n" +
		"      Migrate the database\n" +
		"\n"
	assert.Equal(t, want, got)
}

func TestStringArgsDispatcher_PrintCommands_variadic(t *testing.T) {
	// A variadic argument takes every string from its position on as element,
	// so its usage shows the element type with "..." instead of the slice type,
	// which a slice argument shows because it takes one slice literal like [1,2]
	disp := NewStringArgsDispatcher("myapp")
	disp.MustAddCommand("sum", "", function.MustReflectWrapper(func(label string, nums ...*int) {}, "label", "nums"))
	disp.MustAddCommand("sumslice", "", function.MustReflectWrapper(func(nums []int) {}, "nums"))

	got := captureStdout(t, disp.PrintCommands)

	assert.Equal(t, "  myapp sum <label:string> <nums:...int>\n\n  myapp sumslice <nums:[]int>\n\n", got)
}
