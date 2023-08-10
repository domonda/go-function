package cli

import (
	"errors"
	"fmt"
)

type ErrCommandNotFound string

func (e ErrCommandNotFound) Error() string {
	return fmt.Sprintf("command '%s' not found", string(e))
}

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
