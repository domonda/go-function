package cli

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/domonda/go-function"
)

// SuperStringArgsDispatcher dispatches CLI commands with two levels,
// a super command and a command, like "myapp user create",
// to the StringArgsDispatcher of the super command.
// A super command can have a default command that runs
// without a command name, like "myapp deploy".
//
// Example:
//
//	dispatcher := cli.NewSuperStringArgsDispatcher("myapp")
//	dispatcher.MustAddCommand("deploy", "Deploy all services", deployWrapper)
//	userDispatcher := dispatcher.MustAddSuperCommand("user")
//	userDispatcher.MustAddCommand("create", "Create a user", createUserWrapper)
//	superCommand, command, err := dispatcher.DispatchCombinedCommandAndArgs(ctx, os.Args[1:])
type SuperStringArgsDispatcher struct {
	baseCommand string
	sub         map[string]*StringArgsDispatcher
	loggers     []StringArgsCommandLogger
}

// NewSuperStringArgsDispatcher creates a new dispatcher for super commands.
// The baseCommand is used for help text and completion (typically the program name).
// Optional loggers are called whenever a command is executed,
// with the super command and the command joined with a space as command.
func NewSuperStringArgsDispatcher(baseCommand string, loggers ...StringArgsCommandLogger) *SuperStringArgsDispatcher {
	return &SuperStringArgsDispatcher{
		baseCommand: baseCommand,
		sub:         make(map[string]*StringArgsDispatcher),
		loggers:     loggers,
	}
}

// AddSuperCommand registers the super command superCommand
// and returns its dispatcher, to which the commands of the super command are added.
// An empty superCommand (DefaultCommand) holds the default command
// that runs without arguments, see AddDefaultCommand.
// Returns an error if the super command name is invalid or already registered.
func (disp *SuperStringArgsDispatcher) AddSuperCommand(superCommand string) (subDisp *StringArgsDispatcher, err error) {
	if superCommand != "" {
		if err := checkCommandChars(superCommand); err != nil {
			return nil, fmt.Errorf("Command '%s': %w", superCommand, err)
		}
	}
	if _, exists := disp.sub[superCommand]; exists {
		return nil, fmt.Errorf("super command already added: '%s'", superCommand)
	}
	subDisp = NewStringArgsDispatcher(disp.baseCommand, superCommandLoggers(superCommand, disp.loggers)...)
	disp.sub[superCommand] = subDisp
	return subDisp, nil
}

// MustAddSuperCommand is like AddSuperCommand but panics on error.
func (disp *SuperStringArgsDispatcher) MustAddSuperCommand(superCommand string) (subDisp *StringArgsDispatcher) {
	subDisp, err := disp.AddSuperCommand(superCommand)
	if err != nil {
		panic(fmt.Errorf("MustAddSuperCommand(%s): %w", superCommand, err))
	}
	return subDisp
}

// AddDefaultCommand registers a command that runs
// when neither a super command nor a command is specified.
// Returns an error if a default command is already registered.
func (disp *SuperStringArgsDispatcher) AddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	subDisp, err := disp.AddSuperCommand(DefaultCommand)
	if err != nil {
		return err
	}
	return subDisp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
}

// MustAddDefaultCommand is like AddDefaultCommand but panics on error.
func (disp *SuperStringArgsDispatcher) MustAddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(fmt.Errorf("MustAddDefaultCommand(%s): %w", description, err))
	}
}

// AddCommand registers command as super command without sub commands,
// that runs commandFunc as its default command, like "myapp deploy".
// Returns an error if the command name is invalid or already registered.
func (disp *SuperStringArgsDispatcher) AddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	subDisp, err := disp.AddSuperCommand(command)
	if err != nil {
		return err
	}
	return subDisp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
}

// MustAddCommand is like AddCommand but panics on error.
func (disp *SuperStringArgsDispatcher) MustAddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddCommand(command, description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

// HasCommand returns true if superCommand is registered
// and has a default command, like the commands added with AddCommand.
func (disp *SuperStringArgsDispatcher) HasCommand(superCommand string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasDefaultCommand()
}

// Commands returns a sorted list of all registered super command names,
// including DefaultCommand if registered.
func (disp *SuperStringArgsDispatcher) Commands() []string {
	return slices.Sorted(maps.Keys(disp.sub))
}

// HasSubCommand returns true if superCommand has a command with the name command.
func (disp *SuperStringArgsDispatcher) HasSubCommand(superCommand, command string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasCommand(command)
}

// SubCommands returns a sorted list of the command names of superCommand,
// or nil if superCommand is not registered.
func (disp *SuperStringArgsDispatcher) SubCommands(superCommand string) []string {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return nil
	}
	return sub.Commands()
}

// Dispatch executes command of superCommand with the given arguments.
// The context is passed to the wrapped function if it accepts one.
// Loggers are notified before execution.
//
// Returns ErrSuperCommandNotFound if the super command doesn't exist,
// ErrCommandNotFound if the command doesn't exist,
// or any error returned by the wrapped function.
func (disp *SuperStringArgsDispatcher) Dispatch(ctx context.Context, superCommand, command string, args ...string) error {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return ErrSuperCommandNotFound(superCommand)
	}
	return sub.Dispatch(ctx, command, args...)
}

// DispatchDefaultCommand executes the default command without arguments.
// Returns an error for which IsErrCommandNotFound returns true
// if there is no default command, or any error returned by the wrapped function.
func (disp *SuperStringArgsDispatcher) DispatchDefaultCommand(ctx context.Context) error {
	return disp.Dispatch(ctx, DefaultCommand, DefaultCommand)
}

// DispatchCombinedCommandAndArgs parses and dispatches from os.Args style input
// and returns the dispatched super command and command for error messages.
// The first element is the super command. The second element is its command,
// except if the super command has a default command and the second element
// is empty or not the name of one of its commands, then the default command
// gets all elements after the super command as arguments.
// If commandAndArgs is empty, the default command is executed.
func (disp *SuperStringArgsDispatcher) DispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (superCommand, command string, err error) {
	var args []string
	switch len(commandAndArgs) {
	case 0:
		superCommand = DefaultCommand
		command = DefaultCommand
	case 1:
		superCommand = commandAndArgs[0]
		command = DefaultCommand
	default:
		superCommand = commandAndArgs[0]
		sub, ok := disp.sub[superCommand]
		// A named sub command takes precedence over the default command,
		// an empty argument is an argument and not the name of the default command
		if ok && sub.HasDefaultCommand() && (commandAndArgs[1] == DefaultCommand || !sub.HasCommand(commandAndArgs[1])) {
			command = DefaultCommand
			args = commandAndArgs[1:]
		} else {
			command = commandAndArgs[1]
			args = commandAndArgs[2:]
		}
	}
	return superCommand, command, disp.Dispatch(ctx, superCommand, command, args...)
}

// PrintCommands prints all registered commands of all super commands
// with their descriptions and argument types like StringArgsDispatcher.PrintCommands.
func (disp *SuperStringArgsDispatcher) PrintCommands() {
	type superCmd struct {
		super string
		cmd   *stringArgsCommand
	}
	var commands []superCmd
	for super, sub := range disp.sub {
		for _, cmd := range sub.comm {
			commands = append(commands, superCmd{super: super, cmd: cmd})
		}
	}
	sort.Slice(commands, func(i, j int) bool {
		if commands[i].super == commands[j].super {
			return commands[i].cmd.command < commands[j].cmd.command
		}
		return commands[i].super < commands[j].super
	})

	for i := range commands {
		cmd := commands[i].cmd
		command := commands[i].super
		if cmd.command != DefaultCommand {
			command += " " + cmd.command
		}

		UsageColor.Printf("  %s %s %s\n", disp.baseCommand, command, functionArgsString(cmd.commandFunc)) //#nosec G104
		if cmd.description != "" {
			DescriptionColor.Printf("      %s\n", cmd.description) //#nosec G104
		}
		hasAnyArgDesc := false
		for _, desc := range cmd.commandFunc.ArgDescriptions() {
			if desc != "" {
				hasAnyArgDesc = true
				break
			}
		}
		if hasAnyArgDesc {
			argNames := cmd.commandFunc.ArgNames()
			for i, desc := range cmd.commandFunc.ArgDescriptions() {
				DescriptionColor.Printf("          <%s:%s> %s\n", argNames[i], argTypeString(cmd.commandFunc, i), desc) //#nosec G104
			}
		}
		DescriptionColor.Println() //#nosec G104
	}
}

// PrintCommandsUsageIntro prints a "Commands:" header followed by all commands.
// Does nothing if no commands are registered.
func (disp *SuperStringArgsDispatcher) PrintCommandsUsageIntro() {
	if len(disp.sub) == 0 {
		return
	}
	fmt.Println("Commands:")
	disp.PrintCommands()
}

// PrintCompletion prints the commands matching args for shell completion,
// one per line prefixed with the base command.
// A single argument is the prefix of a super command or command,
// two arguments are a super command and the prefix of one of its sub commands.
func (disp *SuperStringArgsDispatcher) PrintCompletion(args []string) {
	// Super command and sub command are joined with a space
	// like the commands that are compared with the prefix
	prefix := strings.Join(args[:min(len(args), 2)], " ")
	type superCmd struct {
		super string
		cmd   *stringArgsCommand
	}
	var commands []superCmd
	for super, sub := range disp.sub {
		for _, cmd := range sub.comm {
			commands = append(commands, superCmd{super: super, cmd: cmd})
		}
	}
	sort.Slice(commands, func(i, j int) bool {
		if commands[i].super == commands[j].super {
			return commands[i].cmd.command < commands[j].cmd.command
		}
		return commands[i].super < commands[j].super
	})

	for i := range commands {
		cmd := commands[i].cmd
		command := commands[i].super
		if cmd.command != DefaultCommand {
			command += " " + cmd.command
		}
		if strings.HasPrefix(command, prefix) {
			// No trailing space for the default command
			fmt.Println(strings.TrimSpace(disp.baseCommand + " " + command))
		}
	}
}

// superCommandLoggers returns loggers for the sub dispatcher of superCommand
// that log the super command and the command joined with a space,
// because the sub dispatcher only knows the command.
func superCommandLoggers(superCommand string, loggers []StringArgsCommandLogger) []StringArgsCommandLogger {
	if len(loggers) == 0 {
		return nil
	}
	return []StringArgsCommandLogger{
		StringArgsCommandLoggerFunc(func(command string, args []string) {
			fullCommand := superCommand
			switch {
			case superCommand == DefaultCommand:
				fullCommand = command
			case command != DefaultCommand:
				fullCommand += " " + command
			}
			for _, logger := range loggers {
				logger.LogStringArgsCommand(fullCommand, args)
			}
		}),
	}
}
