package cli

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/domonda/go-function"
)

// stringArgsCommand stores metadata and execution details for a single CLI command.
// It wraps a function along with its description and result handlers.
type stringArgsCommand struct {
	command         string
	description     string
	commandFunc     function.Wrapper
	stringArgsFunc  function.StringArgsFunc
	resultsHandlers []function.ResultsHandler
}

// checkCommandChars validates that a command string contains only valid characters.
// Commands cannot contain whitespace, shell metacharacters, or non-graphic characters.
// This prevents command injection and ensures commands work across different shells.
func checkCommandChars(command string) error {
	if strings.IndexFunc(command, unicode.IsSpace) >= 0 {
		return fmt.Errorf("command contains space characters: '%s'", command)
	}
	if strings.IndexFunc(command, unicode.IsGraphic) == -1 {
		return fmt.Errorf("command contains non graphic characters: '%s'", command)
	}
	if strings.ContainsAny(command, "|&;()<>") {
		return fmt.Errorf("command contains invalid characters: '%s'", command)
	}
	return nil
}

// StringArgsCommandLogger provides a way to log or track command executions.
// Implementations can write to log files, send metrics, or perform auditing.
type StringArgsCommandLogger interface {
	LogStringArgsCommand(command string, args []string)
}

// StringArgsCommandLoggerFunc is a function type that implements StringArgsCommandLogger.
// It allows standalone functions to be used as loggers.
type StringArgsCommandLoggerFunc func(command string, args []string)

func (f StringArgsCommandLoggerFunc) LogStringArgsCommand(command string, args []string) {
	f(command, args)
}

// StringArgsDispatcher dispatches CLI commands to wrapped functions.
// It maintains a registry of commands and their associated functions,
// handles command-line argument parsing, and executes the appropriate function.
//
// Example:
//
//	dispatcher := cli.NewStringArgsDispatcher("myapp")
//	dispatcher.MustAddCommand("deploy", "Deploy a service", deployWrapper)
//	dispatcher.DispatchCombinedCommandAndArgs(ctx, os.Args[1:])
type StringArgsDispatcher struct {
	baseCommand string
	comm        map[string]*stringArgsCommand
	loggers     []StringArgsCommandLogger
}

// NewStringArgsDispatcher creates a new command dispatcher.
// The baseCommand is used for help text and completion (typically the program name).
// Optional loggers are called whenever a command is executed.
//
// Example:
//
//	logger := cli.StringArgsCommandLoggerFunc(func(cmd string, args []string) {
//	    log.Printf("Executing: %s %v", cmd, args)
//	})
//	dispatcher := cli.NewStringArgsDispatcher("myapp", logger)
func NewStringArgsDispatcher(baseCommand string, loggers ...StringArgsCommandLogger) *StringArgsDispatcher {
	return &StringArgsDispatcher{
		baseCommand: baseCommand,
		comm:        make(map[string]*stringArgsCommand),
		loggers:     loggers,
	}
}

// AddCommand registers a new command with the dispatcher.
// Returns an error if the command name is invalid or already registered.
//
// Parameters:
//   - command: The command name (e.g., "deploy", "create", "delete")
//   - description: Human-readable description for help text
//   - commandFunc: The wrapped function to execute
//   - resultsHandlers: Optional handlers to process function results
//
// Example:
//
//	err := dispatcher.AddCommand("deploy", "Deploy a service",
//	    function.MustReflectWrapper("deploy", Deploy))
func (disp *StringArgsDispatcher) AddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	if _, exists := disp.comm[command]; exists {
		return fmt.Errorf("Command '%s' already added", command)
	}
	if err := checkCommandChars(command); err != nil {
		return fmt.Errorf("Command '%s' returned: %w", command, err)
	}
	disp.comm[command] = &stringArgsCommand{
		command:         command,
		description:     description,
		commandFunc:     commandFunc,
		stringArgsFunc:  function.NewStringArgsFunc(commandFunc, resultsHandlers...),
		resultsHandlers: resultsHandlers,
	}
	return nil
}

// MustAddCommand is like AddCommand but panics on error.
// Use this in initialization code where command registration failures should be fatal.
func (disp *StringArgsDispatcher) MustAddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddCommand(command, description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

// AddDefaultCommand registers a command that runs when no command is specified.
// This is useful for single-purpose CLIs or when you want a default action.
//
// Example:
//
//	// When user runs just "myapp" with no arguments
//	dispatcher.AddDefaultCommand("Run the server",
//	    function.MustReflectWrapper("serve", Serve))
func (disp *StringArgsDispatcher) AddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	disp.comm[DefaultCommand] = &stringArgsCommand{
		command:         DefaultCommand,
		description:     description,
		commandFunc:     commandFunc,
		stringArgsFunc:  function.NewStringArgsFunc(commandFunc, resultsHandlers...),
		resultsHandlers: resultsHandlers,
	}
	return nil
}

// MustAddDefaultCommand is like AddDefaultCommand but panics on error.
func (disp *StringArgsDispatcher) MustAddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

// HasCommand returns true if a command with the given name is registered.
func (disp *StringArgsDispatcher) HasCommand(command string) bool {
	_, found := disp.comm[command]
	return found
}

// HasDefaultCommand returns true if a default command is registered.
func (disp *StringArgsDispatcher) HasDefaultCommand() bool {
	_, found := disp.comm[DefaultCommand]
	return found
}

// Commands returns a sorted list of all registered command names.
func (disp *StringArgsDispatcher) Commands() []string {
	return slices.Sorted(maps.Keys(disp.comm))
}

// Dispatch executes the specified command with the given arguments.
// The context is passed to the wrapped function if it accepts one.
// Loggers are notified before execution.
//
// Returns ErrCommandNotFound if the command doesn't exist,
// or any error returned by the wrapped function.
//
// Example:
//
//	err := dispatcher.Dispatch(ctx, "deploy", "production", "api-server", "42")
func (disp *StringArgsDispatcher) Dispatch(ctx context.Context, command string, args ...string) error {
	cmd, found := disp.comm[command]
	if !found {
		return ErrCommandNotFound(command)
	}
	for _, logger := range disp.loggers {
		logger.LogStringArgsCommand(command, args)
	}
	return cmd.stringArgsFunc(ctx, args...)
}

// MustDispatch is like Dispatch but panics on error.
func (disp *StringArgsDispatcher) MustDispatch(ctx context.Context, command string, args ...string) {
	err := disp.Dispatch(ctx, command, args...)
	if err != nil {
		panic(fmt.Errorf("command '%s' returned: %w", command, err))
	}
}

// DispatchDefaultCommand executes the default command with a background context.
func (disp *StringArgsDispatcher) DispatchDefaultCommand() error {
	return disp.Dispatch(context.Background(), DefaultCommand)
}

// MustDispatchDefaultCommand is like DispatchDefaultCommand but panics on error.
func (disp *StringArgsDispatcher) MustDispatchDefaultCommand() {
	err := disp.DispatchDefaultCommand()
	if err != nil {
		panic(fmt.Errorf("default command: %w", err))
	}
}

// DispatchCombinedCommandAndArgs parses and dispatches from os.Args style input.
// The first element is treated as the command name, and the rest as arguments.
// If commandAndArgs is empty, the default command is executed.
//
// This is the typical entry point for CLI applications:
//
//	command, err := dispatcher.DispatchCombinedCommandAndArgs(ctx, os.Args[1:])
//	if err != nil {
//	    fmt.Fprintf(os.Stderr, "Error in %s: %v\n", command, err)
//	    os.Exit(1)
//	}
func (disp *StringArgsDispatcher) DispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (command string, err error) {
	if len(commandAndArgs) == 0 {
		return DefaultCommand, disp.DispatchDefaultCommand()
	}
	command = commandAndArgs[0]
	args := commandAndArgs[1:]
	return command, disp.Dispatch(ctx, command, args...)
}

// MustDispatchCombinedCommandAndArgs is like DispatchCombinedCommandAndArgs but panics on error.
func (disp *StringArgsDispatcher) MustDispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (command string) {
	command, err := disp.DispatchCombinedCommandAndArgs(ctx, commandAndArgs)
	if err != nil {
		panic(fmt.Errorf("MustDispatchCombinedCommandAndArgs(%v): %w", commandAndArgs, err))
	}
	return command
}

// PrintCommands prints all registered commands with their descriptions and argument types.
// Output is colorized using UsageColor and DescriptionColor.
// This is useful for generating help text.
func (disp *StringArgsDispatcher) PrintCommands() {
	commands := slices.SortedFunc(maps.Values(disp.comm), func(a, b *stringArgsCommand) int {
		return strings.Compare(a.command, b.command)
	})

	for _, cmd := range commands {
		UsageColor.Printf("  %s %s %s\n", disp.baseCommand, cmd.command, functionArgsString(cmd.commandFunc))
		if cmd.description != "" {
			DescriptionColor.Printf("      %s\n", cmd.description)
		}
		hasAnyArgDesc := false
		for _, desc := range cmd.commandFunc.ArgDescriptions() {
			if desc != "" {
				hasAnyArgDesc = true
				break
			}
		}
		if hasAnyArgDesc {
			for i, desc := range cmd.commandFunc.ArgDescriptions() {
				DescriptionColor.Printf("          <%s:%s> %s\n", cmd.commandFunc.ArgNames()[i], derefType(cmd.commandFunc.ArgTypes()[i]), desc)
			}
		}
		DescriptionColor.Println()
	}
}

// PrintCommandsUsageIntro prints a "Commands:" header followed by all commands.
// Does nothing if no commands are registered.
func (disp *StringArgsDispatcher) PrintCommandsUsageIntro() {
	if len(disp.comm) == 0 {
		return
	}
	fmt.Println("Commands:")
	disp.PrintCommands()
}

// PrintCompletion prints commands matching the given prefix for shell completion.
// This is used internally by the completion system.
func (disp *StringArgsDispatcher) PrintCompletion(args []string) {
	prefix := ""
	if len(args) > 0 {
		prefix = args[0]
	}
	commands := slices.SortedFunc(maps.Values(disp.comm), func(a, b *stringArgsCommand) int {
		return strings.Compare(a.command, b.command)
	})
	for _, cmd := range commands {
		if strings.HasPrefix(cmd.command, prefix) {
			fmt.Println(disp.baseCommand, cmd.command)
		}
	}
}

// functionArgsString formats function arguments as a string for help text.
// Returns a space-separated list like: "<name:string> <age:int> <active:bool>"
func functionArgsString(f function.Wrapper) string {
	b := strings.Builder{}
	argNames := f.ArgNames()
	argTypes := f.ArgTypes()
	for i := range argNames {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "<%s:%s>", argNames[i], derefType(argTypes[i]))
	}
	return b.String()
}

// derefType returns the element type if t is a pointer type, otherwise returns t.
// This is used to display cleaner type names in help text (e.g., "int" instead of "*int").
func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
