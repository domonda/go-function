package cli

import "github.com/fatih/color"

const (
	// DefaultCommand is the name of the default command,
	// which runs without a command name,
	// see StringArgsDispatcher.AddDefaultCommand.
	DefaultCommand = ""
)

var (
	// UsageColor is the color in which the
	// command usage will be printed on the screen.
	UsageColor = color.New(color.FgHiCyan)

	// DescriptionColor is the color in which the
	// command usage description will be printed on the screen.
	DescriptionColor = color.New(color.FgCyan)
)
