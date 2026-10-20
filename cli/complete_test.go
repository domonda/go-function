package cli

import (
	"testing"

	"github.com/posener/complete/v2"

	"github.com/domonda/go-function"
)

func TestCompleteStringArgsDispatcher(t *testing.T) {
	disp := NewStringArgsDispatcher("myapp")
	noop := function.MustReflectWrapper(func() {})
	disp.MustAddDefaultCommand("", noop)
	disp.MustAddCommand("start", "", noop)
	disp.MustAddCommand("stop", "", noop)
	disp.MustAddCommand("deploy", "", noop)

	// Shell completion suggests the same commands as PrintCompletion
	// and posener/complete adds the help flag.
	// The default command has no name that could be typed,
	// so it must not be suggested as empty word.
	complete.Test(t, completer{disp}, "", []string{"deploy", "start", "stop", "-h"})
	complete.Test(t, completer{disp}, "st", []string{"start", "stop"})
	complete.Test(t, completer{disp}, "sta", []string{"start"})
}

func TestCompleteSuperStringArgsDispatcher(t *testing.T) {
	var calls []string
	disp := newDeploySuperDispatcher(&calls)

	// Shell completion suggests the same commands as PrintCompletion
	// and posener/complete adds the help flag.
	// The default command has no name that could be typed,
	// so it must not be suggested as empty word.
	complete.Test(t, superCompleter{disp}, "", []string{"deploy", "status", "-h"})
	complete.Test(t, superCompleter{disp}, "st", []string{"status"})
	complete.Test(t, superCompleter{disp}, "deploy ", []string{"start", "status", "stop", "-h"})
	complete.Test(t, superCompleter{disp}, "deploy sta", []string{"start", "status"})
	complete.Test(t, superCompleter{disp}, "deploy sto", []string{"stop"})
}
