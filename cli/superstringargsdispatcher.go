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

type SuperStringArgsDispatcher struct {
	baseCommand string
	sub         map[string]*StringArgsDispatcher
	loggers     []StringArgsCommandLogger
}

func NewSuperStringArgsDispatcher(baseCommand string, loggers ...StringArgsCommandLogger) *SuperStringArgsDispatcher {
	return &SuperStringArgsDispatcher{
		baseCommand: baseCommand,
		sub:         make(map[string]*StringArgsDispatcher),
		loggers:     loggers,
	}
}

func (disp *SuperStringArgsDispatcher) AddSuperCommand(superCommand string) (subDisp *StringArgsDispatcher, err error) {
	if superCommand != "" {
		if err := checkCommandChars(superCommand); err != nil {
			return nil, fmt.Errorf("Command '%s': %w", superCommand, err)
		}
	}
	if _, exists := disp.sub[superCommand]; exists {
		return nil, fmt.Errorf("super command already added: '%s'", superCommand)
	}
	subDisp = NewStringArgsDispatcher(disp.baseCommand, disp.loggers...)
	disp.sub[superCommand] = subDisp
	return subDisp, nil
}

func (disp *SuperStringArgsDispatcher) MustAddSuperCommand(superCommand string) (subDisp *StringArgsDispatcher) {
	subDisp, err := disp.AddSuperCommand(superCommand)
	if err != nil {
		panic(fmt.Errorf("MustAddSuperCommand(%s): %w", superCommand, err))
	}
	return subDisp
}

func (disp *SuperStringArgsDispatcher) AddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	subDisp, err := disp.AddSuperCommand(DefaultCommand)
	if err != nil {
		return err
	}
	return subDisp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
}

func (disp *SuperStringArgsDispatcher) MustAddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(fmt.Errorf("MustAddDefaultCommand(%s): %w", description, err))
	}
}

func (disp *SuperStringArgsDispatcher) AddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) error {
	subDisp, err := disp.AddSuperCommand(command)
	if err != nil {
		return err
	}
	return subDisp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
}

func (disp *SuperStringArgsDispatcher) MustAddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddCommand(command, description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

func (disp *SuperStringArgsDispatcher) HasCommand(superCommand string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasDefaultCommand()
}

func (disp *SuperStringArgsDispatcher) Commands() []string {
	return slices.Sorted(maps.Keys(disp.sub))
}

func (disp *SuperStringArgsDispatcher) HasSubCommand(superCommand, command string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasCommand(command)
}

func (disp *SuperStringArgsDispatcher) SubCommands(superCommand string) []string {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return nil
	}
	return sub.Commands()
}

func (disp *SuperStringArgsDispatcher) Dispatch(ctx context.Context, superCommand, command string, args ...string) error {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return ErrSuperCommandNotFound(superCommand)
	}
	return sub.Dispatch(ctx, command, args...)
}

func (disp *SuperStringArgsDispatcher) MustDispatch(ctx context.Context, superCommand, command string, args ...string) {
	err := disp.Dispatch(ctx, superCommand, command, args...)
	if err != nil {
		panic(fmt.Errorf("Command '%s': %w", command, err))
	}
}

func (disp *SuperStringArgsDispatcher) DispatchDefaultCommand() error {
	return disp.Dispatch(context.Background(), DefaultCommand, DefaultCommand)
}

func (disp *SuperStringArgsDispatcher) MustDispatchDefaultCommand() {
	err := disp.DispatchDefaultCommand()
	if err != nil {
		panic(fmt.Errorf("Default command: %w", err))
	}
}

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

func (disp *SuperStringArgsDispatcher) MustDispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (superCommand, command string) {
	superCommand, command, err := disp.DispatchCombinedCommandAndArgs(ctx, commandAndArgs)
	if err != nil {
		panic(fmt.Errorf("MustDispatchCombinedCommandAndArgs(%v): %w", commandAndArgs, err))
	}
	return superCommand, command
}

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
			for i, desc := range cmd.commandFunc.ArgDescriptions() {
				DescriptionColor.Printf("          <%s:%s> %s\n", cmd.commandFunc.ArgNames()[i], derefType(cmd.commandFunc.ArgTypes()[i]), desc) //#nosec G104
			}
		}
		DescriptionColor.Println() //#nosec G104
	}
}

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
