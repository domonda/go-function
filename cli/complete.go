package cli

import (
	"os"
	"sort"
	"strings"

	"github.com/posener/complete/v2"
)

func CompleteStringArgsDispatcher(disp *StringArgsDispatcher) {
	complete.Complete(os.Args[0], completer{disp})
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
		if strings.HasPrefix(command, prefix) {
			commands = append(commands, command)
		}
	}
	sort.Strings(commands)
	return commands
}

func CompleteSuperStringArgsDispatcher(disp *SuperStringArgsDispatcher) {
	complete.Complete(os.Args[0], superCompleter{disp})
}

type superCompleter struct {
	*SuperStringArgsDispatcher
}

func (c superCompleter) SubCmdList() []string {
	return c.Commands()
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
		if strings.HasPrefix(command, prefix) {
			commands = append(commands, command)
		}
	}
	sort.Strings(commands)
	return commands
}
