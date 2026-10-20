package cli

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/posener/complete/v2"
)

// CompleteStringArgsDispatcher provides shell completion of the commands of disp
// using github.com/posener/complete/v2 for the program named like
// the base name of os.Args[0]. Call it at the start of main:
// it completes the commands and exits if the shell requests a completion
// with the COMP_LINE and COMP_POINT environment variables,
// installs or uninstalls the completion in the shell configuration and exits
// for COMP_INSTALL=1 or COMP_UNINSTALL=1,
// and else returns so the program can dispatch the command.
func CompleteStringArgsDispatcher(disp *StringArgsDispatcher) {
	commandName := filepath.Base(os.Args[0])
	complete.Complete(commandName, completer{disp})
}

type completer struct {
	*StringArgsDispatcher
}

func (c completer) SubCmdList() []string                    { return nil }
func (c completer) SubCmdGet(cmd string) complete.Completer { return nil }
func (c completer) FlagList() []string                      { return nil }
func (c completer) FlagGet(flag string) complete.Predictor  { return nil }
func (c completer) ArgsGet() complete.Predictor             { return c }

func (c completer) Predict(prefix string) (commands []string) {
	for command := range c.comm {
		// The default command has no name that could be completed
		if command != DefaultCommand && strings.HasPrefix(command, prefix) {
			commands = append(commands, command)
		}
	}
	sort.Strings(commands)
	return commands
}

// CompleteSuperStringArgsDispatcher provides shell completion of the super commands
// and their sub commands of disp like CompleteStringArgsDispatcher.
func CompleteSuperStringArgsDispatcher(disp *SuperStringArgsDispatcher) {
	commandName := filepath.Base(os.Args[0])
	complete.Complete(commandName, superCompleter{disp})
}

type superCompleter struct {
	*SuperStringArgsDispatcher
}

func (c superCompleter) SubCmdList() []string {
	// The default command has no name that could be completed
	return slices.DeleteFunc(c.Commands(), func(command string) bool {
		return command == DefaultCommand
	})
}

func (c superCompleter) SubCmdGet(cmd string) complete.Completer {
	disp := c.sub[cmd]
	if disp == nil {
		return nil
	}
	return completer{disp}
}

func (c superCompleter) FlagList() []string                     { return nil }
func (c superCompleter) FlagGet(flag string) complete.Predictor { return nil }
func (c superCompleter) ArgsGet() complete.Predictor            { return c }

func (c superCompleter) Predict(prefix string) (commands []string) {
	for command := range c.sub {
		if command != DefaultCommand && strings.HasPrefix(command, prefix) {
			commands = append(commands, command)
		}
	}
	sort.Strings(commands)
	return commands
}
