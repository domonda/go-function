package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/domonda/go-function"
)

// newDeploySuperDispatcher returns a dispatcher with
// a default command, a super command with sub commands,
// and a command without sub commands.
// calls records the dispatched commands with their arguments.
func newDeploySuperDispatcher(calls *[]string) *SuperStringArgsDispatcher {
	record := func(command string) function.Wrapper {
		return function.MustReflectWrapper(func(a, b string) {
			*calls = append(*calls, command+" "+a+","+b)
		}, "a", "b")
	}
	disp := NewSuperStringArgsDispatcher("myapp")
	disp.MustAddDefaultCommand("Default", record("default"))
	deploy := disp.MustAddSuperCommand("deploy")
	deploy.MustAddCommand("start", "Start deployment", record("deploy start"))
	deploy.MustAddCommand("stop", "Stop deployment", record("deploy stop"))
	deploy.MustAddCommand("status", "Deployment status", record("deploy status"))
	disp.MustAddCommand("status", "Status", record("status"))
	return disp
}

func TestSuperStringArgsDispatcher_AddSuperCommand(t *testing.T) {
	var calls []string
	disp := newDeploySuperDispatcher(&calls)

	assert.Equal(t, []string{DefaultCommand, "deploy", "status"}, disp.Commands())
	assert.Equal(t, []string{"start", "status", "stop"}, disp.SubCommands("deploy"))
	assert.Nil(t, disp.SubCommands("unknown"))
	assert.True(t, disp.HasSubCommand("deploy", "start"))
	assert.False(t, disp.HasSubCommand("deploy", "unknown"))
	assert.False(t, disp.HasSubCommand("unknown", "start"))
	// HasCommand is true for commands that can be called without sub command
	assert.True(t, disp.HasCommand("status"))
	assert.False(t, disp.HasCommand("deploy"))

	noop := function.MustReflectWrapper(func() {})
	// Super commands and commands share one namespace
	// and must not silently replace each other
	_, err := disp.AddSuperCommand("deploy")
	assert.Error(t, err)
	assert.Error(t, disp.AddCommand("status", "", noop))
	assert.Error(t, disp.AddDefaultCommand("", noop))
	assert.Panics(t, func() { disp.MustAddSuperCommand("deploy") })
	// Super commands must be a single shell word without shell metacharacters
	_, err = disp.AddSuperCommand("two words")
	assert.Error(t, err)
}

func TestSuperStringArgsDispatcher_Dispatch(t *testing.T) {
	var calls []string
	disp := newDeploySuperDispatcher(&calls)
	ctx := context.Background()

	require.NoError(t, disp.Dispatch(ctx, "deploy", "start", "a", "b"))
	assert.Equal(t, []string{"deploy start a,b"}, calls)

	err := disp.Dispatch(ctx, "unknown", "start")
	assert.Equal(t, ErrSuperCommandNotFound("unknown"), err)
	assert.True(t, IsErrCommandNotFound(err))

	err = disp.Dispatch(ctx, "deploy", "unknown")
	assert.Equal(t, ErrCommandNotFound("unknown"), err)
	assert.True(t, IsErrCommandNotFound(err))
}

func TestSuperStringArgsDispatcher_DispatchCombinedCommandAndArgs(t *testing.T) {
	tests := []struct {
		commandAndArgs   []string
		wantSuperCommand string
		wantCommand      string
		wantCall         string
	}{
		{commandAndArgs: nil, wantSuperCommand: DefaultCommand, wantCommand: DefaultCommand, wantCall: "default ,"},
		{commandAndArgs: []string{"status"}, wantSuperCommand: "status", wantCommand: DefaultCommand, wantCall: "status ,"},
		// A command without sub commands gets all following arguments
		{commandAndArgs: []string{"status", "start", "x"}, wantSuperCommand: "status", wantCommand: DefaultCommand, wantCall: "status start,x"},
		{commandAndArgs: []string{"deploy", "stop", "x", "y"}, wantSuperCommand: "deploy", wantCommand: "stop", wantCall: "deploy stop x,y"},
	}
	for _, tt := range tests {
		var calls []string
		disp := newDeploySuperDispatcher(&calls)

		superCommand, command, err := disp.DispatchCombinedCommandAndArgs(context.Background(), tt.commandAndArgs)

		require.NoError(t, err, "%#v", tt.commandAndArgs)
		assert.Equal(t, tt.wantSuperCommand, superCommand)
		assert.Equal(t, tt.wantCommand, command)
		assert.Equal(t, []string{tt.wantCall}, calls)
	}

	var calls []string
	disp := newDeploySuperDispatcher(&calls)
	_, _, err := disp.DispatchCombinedCommandAndArgs(context.Background(), []string{"unknown", "x"})
	assert.True(t, IsErrCommandNotFound(err))
	assert.Empty(t, calls)
}

func TestSuperStringArgsDispatcher_DispatchDefaultCommand(t *testing.T) {
	var gotCtx context.Context
	disp := NewSuperStringArgsDispatcher("myapp")
	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	assert.True(t, IsErrCommandNotFound(disp.DispatchDefaultCommand(ctx)))

	disp.MustAddDefaultCommand("", function.MustReflectWrapper(func(ctx context.Context) { gotCtx = ctx }))
	require.NoError(t, disp.DispatchDefaultCommand(ctx))
	require.NotNil(t, gotCtx)
	assert.Equal(t, "value", gotCtx.Value(ctxKey{}), "the context of the caller is passed to the function")
}

func TestSuperStringArgsDispatcher_loggers(t *testing.T) {
	// Loggers get the full command, the super command and the command
	// joined with a space, not only the command of the sub dispatcher
	var logged []string
	logger := StringArgsCommandLoggerFunc(func(command string, args []string) {
		logged = append(logged, fmt.Sprintf("%q %v", command, args))
	})
	disp := NewSuperStringArgsDispatcher("app", logger)
	noop := function.MustReflectWrapper(func(string) {}, "s")
	disp.MustAddDefaultCommand("", function.MustReflectWrapper(func() {}))
	disp.MustAddCommand("deploy", "", noop)
	disp.MustAddSuperCommand("user").MustAddCommand("create", "", noop)
	ctx := context.Background()

	for _, commandAndArgs := range [][]string{nil, {"deploy", "x"}, {"user", "create", "bob"}, {"user", "unknown"}} {
		_, _, _ = disp.DispatchCombinedCommandAndArgs(ctx, commandAndArgs)
	}

	assert.Equal(t, []string{`"" []`, `"deploy" [x]`, `"user create" [bob]`}, logged, "unknown commands are not logged")
}

func TestSuperStringArgsDispatcher_PrintCompletion(t *testing.T) {
	var calls []string
	disp := newDeploySuperDispatcher(&calls)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "everything",
			args: nil,
			want: "myapp\nmyapp deploy start\nmyapp deploy status\nmyapp deploy stop\nmyapp status\n",
		},
		{
			name: "partial super command",
			args: []string{"st"},
			want: "myapp status\n",
		},
		{
			name: "super command",
			args: []string{"deploy"},
			want: "myapp deploy start\nmyapp deploy status\nmyapp deploy stop\n",
		},
		{
			// Only the sub commands of the typed super command
			// that start with the typed partial sub command
			name: "partial sub command",
			args: []string{"deploy", "sta"},
			want: "myapp deploy start\nmyapp deploy status\n",
		},
		{
			name: "longer partial sub command",
			args: []string{"deploy", "sto"},
			want: "myapp deploy stop\n",
		},
		{
			name: "empty partial sub command",
			args: []string{"deploy", ""},
			want: "myapp deploy start\nmyapp deploy status\nmyapp deploy stop\n",
		},
		{
			// The second argument of a command without
			// sub commands is an argument, not a sub command
			name: "command without sub commands",
			args: []string{"status", "st"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureStdout(t, func() { disp.PrintCompletion(tt.args) })
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSuperStringArgsDispatcher_PrintCommands(t *testing.T) {
	disp := NewSuperStringArgsDispatcher("myapp")
	assert.Empty(t, captureStdout(t, disp.PrintCommandsUsageIntro), "no intro without commands")

	user := disp.MustAddSuperCommand("user")
	user.MustAddCommand("create", "Create a user", function.MustReflectWrapper(func(name string, age int) {}, "name", "age"))
	disp.MustAddCommand("version", "Print the version", function.MustReflectWrapper(func() {}))

	got := captureStdout(t, disp.PrintCommandsUsageIntro)

	// The usage shows how to call each command with the argument names and types,
	// sub commands are shown after their super command, sorted by super command
	assert.True(t, strings.HasPrefix(got, "Commands:\n"), got)
	userCreate := strings.Index(got, "  myapp user create <name:string> <age:int>\n      Create a user\n")
	version := strings.Index(got, "  myapp version")
	assert.True(t, userCreate > 0 && version > userCreate, got)
	assert.Contains(t, got, "      Print the version\n")
}

func TestSuperStringArgsDispatcher_PrintCommands_contextArg(t *testing.T) {
	// The dispatcher passes the context argument,
	// so the usage doesn't show it as command line argument
	disp := NewSuperStringArgsDispatcher("myapp")
	db := disp.MustAddSuperCommand("db")
	db.MustAddCommand("migrate", "Migrate the database", function.MustReflectWrapper(func(ctx context.Context, steps int) {}, "ctx", "steps"))
	disp.MustAddCommand("version", "Print the version", function.MustReflectWrapper(func(ctx context.Context) {}))

	got := captureStdout(t, disp.PrintCommands)

	want := "  myapp db migrate <steps:int>\n" +
		"      Migrate the database\n" +
		"\n" +
		"  myapp version\n" +
		"      Print the version\n" +
		"\n"
	assert.Equal(t, want, got)
}

func TestSuperStringArgsDispatcher_DispatchCombinedCommandAndArgs_defaultAndSubCommands(t *testing.T) {
	// A super command with a default command must still dispatch its named sub commands,
	// which PrintCompletion advertises, only other arguments go to the default command
	var calls []string
	record := func(command string) function.Wrapper {
		return function.MustReflectWrapper(func(a, b string) {
			calls = append(calls, command+" "+a+","+b)
		}, "a", "b")
	}
	disp := NewSuperStringArgsDispatcher("myapp")
	user := disp.MustAddSuperCommand("user")
	user.MustAddDefaultCommand("List users", record("user default"))
	user.MustAddCommand("create", "Create a user", record("user create"))
	ctx := context.Background()

	_, command, err := disp.DispatchCombinedCommandAndArgs(ctx, []string{"user", "create", "alice"})
	require.NoError(t, err)
	assert.Equal(t, "create", command)
	assert.Equal(t, []string{"user create alice,"}, calls)

	calls = nil
	_, command, err = disp.DispatchCombinedCommandAndArgs(ctx, []string{"user", "alice"})
	require.NoError(t, err)
	assert.Equal(t, DefaultCommand, command)
	assert.Equal(t, []string{"user default alice,"}, calls)

	// An empty argument is an argument of the default command,
	// not the name of the default command, so no argument is dropped
	calls = nil
	_, command, err = disp.DispatchCombinedCommandAndArgs(ctx, []string{"user", "", "file.csv"})
	require.NoError(t, err)
	assert.Equal(t, DefaultCommand, command)
	assert.Equal(t, []string{"user default ,file.csv"}, calls)
}
