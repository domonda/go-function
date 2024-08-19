package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"

	"github.com/domonda/go-function"
)

type SuperStringArgsDispatcher struct {
	sub     map[string]*StringArgsDispatcher
	loggers []StringArgsCommandLogger
}

func NewSuperStringArgsDispatcher(loggers ...StringArgsCommandLogger) *SuperStringArgsDispatcher {
	return &SuperStringArgsDispatcher{
		sub:     make(map[string]*StringArgsDispatcher),
		loggers: loggers,
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
	subDisp = NewStringArgsDispatcher(disp.loggers...)
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
func (disp *SuperStringArgsDispatcher) HasCommnd(superCommand string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasDefaultCommnd()
}

func (disp *SuperStringArgsDispatcher) Commands() []string {
	return slices.Sorted(maps.Keys(disp.sub))
}

func (disp *SuperStringArgsDispatcher) HasSubCommnd(superCommand, command string) bool {
	sub, ok := disp.sub[superCommand]
	if !ok {
		return false
	}
	return sub.HasCommnd(command)
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
		if ok && sub.HasDefaultCommnd() {
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

func (disp *SuperStringArgsDispatcher) PrintCommands(appName string) {
	type superCmd struct {
		super string
		cmd   *stringArgsCommand
	}

	var list []superCmd
	for super, sub := range disp.sub {
		for _, cmd := range sub.comm {
			list = append(list, superCmd{super: super, cmd: cmd})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].super == list[j].super {
			return list[i].cmd.command < list[j].cmd.command
		}
		return list[i].super < list[j].super
	})

	for i := range list {
		cmd := list[i].cmd
		command := list[i].super
		if cmd.command != DefaultCommand {
			command += " " + cmd.command
		}

		UsageColor.Printf("  %s %s %s\n", appName, command, functionArgsString(cmd.commandFunc))
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

func (disp *SuperStringArgsDispatcher) PrintCommandsUsageIntro(appName string, output io.Writer) {
	if len(disp.sub) > 0 {
		fmt.Fprint(output, "Commands:\n")
		disp.PrintCommands(appName)
		// fmt.Fprint(output, "Flags:\n")
	}
}
