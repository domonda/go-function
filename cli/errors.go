package cli

import (
	"errors"
	"fmt"
)

// ErrCommandNotFound is returned when dispatching
// a command that is not registered, the string is the command.
type ErrCommandNotFound string

func (e ErrCommandNotFound) Error() string {
	return fmt.Sprintf("command '%s' not found", string(e))
}

// ErrSuperCommandNotFound is returned by SuperStringArgsDispatcher when dispatching
// a super command that is not registered, the string is the super command.
type ErrSuperCommandNotFound string

func (e ErrSuperCommandNotFound) Error() string {
	return fmt.Sprintf("super command '%s' not found", string(e))
}

// IsErrCommandNotFound returns true if the passed error
// can be unwrapped to either ErrCommandNotFound or ErrSuperCommandNotFound.
func IsErrCommandNotFound(err error) bool {
	var (
		errCommandNotFound      ErrCommandNotFound
		errSuperCommandNotFound ErrSuperCommandNotFound
	)
	return errors.As(err, &errCommandNotFound) || errors.As(err, &errSuperCommandNotFound)
}
