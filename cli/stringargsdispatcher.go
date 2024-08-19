package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/domonda/go-function"
)

type stringArgsCommand struct {
	command         string
	description     string
	commandFunc     function.Wrapper
	stringArgsFunc  function.StringArgsFunc
	resultsHandlers []function.ResultsHandler
}

func checkCommandChars(command string) error {
	if strings.IndexFunc(command, unicode.IsSpace) >= 0 {
		return fmt.Errorf("command contains space characters: '%s'", command)
	}
	if strings.IndexFunc(command, unicode.IsGraphic) == -1 {
		return fmt.Errorf("command contains non graphc characters: '%s'", command)
	}
	if strings.ContainsAny(command, "|&;()<>") {
		return fmt.Errorf("command contains invalid characters: '%s'", command)
	}
	return nil
}

type StringArgsCommandLogger interface {
	LogStringArgsCommand(command string, args []string)
}

type StringArgsCommandLoggerFunc func(command string, args []string)

func (f StringArgsCommandLoggerFunc) LogStringArgsCommand(command string, args []string) {
	f(command, args)
}

type StringArgsDispatcher struct {
	comm    map[string]*stringArgsCommand
	loggers []StringArgsCommandLogger
}

func NewStringArgsDispatcher(loggers ...StringArgsCommandLogger) *StringArgsDispatcher {
	return &StringArgsDispatcher{
		comm:    make(map[string]*stringArgsCommand),
		loggers: loggers,
	}
}

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

func (disp *StringArgsDispatcher) MustAddCommand(command, description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddCommand(command, description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

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

func (disp *StringArgsDispatcher) MustAddDefaultCommand(description string, commandFunc function.Wrapper, resultsHandlers ...function.ResultsHandler) {
	err := disp.AddDefaultCommand(description, commandFunc, resultsHandlers...)
	if err != nil {
		panic(err)
	}
}

func (disp *StringArgsDispatcher) HasCommnd(command string) bool {
	_, found := disp.comm[command]
	return found
}

func (disp *StringArgsDispatcher) HasDefaultCommnd() bool {
	_, found := disp.comm[DefaultCommand]
	return found
}

func (disp *StringArgsDispatcher) Commands() []string {
	return slices.Sorted(maps.Keys(disp.comm))
}

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

func (disp *StringArgsDispatcher) MustDispatch(ctx context.Context, command string, args ...string) {
	err := disp.Dispatch(ctx, command, args...)
	if err != nil {
		panic(fmt.Errorf("command '%s' returned: %w", command, err))
	}
}

func (disp *StringArgsDispatcher) DispatchDefaultCommand() error {
	return disp.Dispatch(context.Background(), DefaultCommand)
}

func (disp *StringArgsDispatcher) MustDispatchDefaultCommand() {
	err := disp.DispatchDefaultCommand()
	if err != nil {
		panic(fmt.Errorf("default command: %w", err))
	}
}

func (disp *StringArgsDispatcher) DispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (command string, err error) {
	if len(commandAndArgs) == 0 {
		return DefaultCommand, disp.DispatchDefaultCommand()
	}
	command = commandAndArgs[0]
	args := commandAndArgs[1:]
	return command, disp.Dispatch(ctx, command, args...)
}

func (disp *StringArgsDispatcher) MustDispatchCombinedCommandAndArgs(ctx context.Context, commandAndArgs []string) (command string) {
	command, err := disp.DispatchCombinedCommandAndArgs(ctx, commandAndArgs)
	if err != nil {
		panic(fmt.Errorf("MustDispatchCombinedCommandAndArgs(%v): %w", commandAndArgs, err))
	}
	return command
}

func (disp *StringArgsDispatcher) PrintCommands(appName string) {
	list := make([]*stringArgsCommand, 0, len(disp.comm))
	for _, cmd := range disp.comm {
		list = append(list, cmd)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].command < list[j].command
	})

	for _, cmd := range list {
		UsageColor.Printf("  %s %s %s\n", appName, cmd.command, functionArgsString(cmd.commandFunc))
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

func (disp *StringArgsDispatcher) PrintCommandsUsageIntro(appName string, output io.Writer) {
	if len(disp.comm) > 0 {
		fmt.Fprint(output, "Commands:\n")
		disp.PrintCommands(appName)
		fmt.Fprint(output, "Flags:\n")
	}
}

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

func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
