package cli

import "fmt"

type ErrCommandNotFound string

func (e ErrCommandNotFound) Error() string {
	return fmt.Sprintf("command '%s' not found", string(e))
}

type ErrSuperCommandNotFound string

func (e ErrSuperCommandNotFound) Error() string {
	return fmt.Sprintf("super command '%s' not found", string(e))
}
