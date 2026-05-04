package websearch

import "fmt"

// Error is a typed error for the websearch package.
type Error struct {
	Msg string
}

func NewError(msg string) *Error { return &Error{Msg: msg} }
func (e *Error) Error() string   { return e.Msg }

// WrapError wraps an error with context.
func WrapError(context string, err error) error {
	return fmt.Errorf("websearch %s: %w", context, err)
}
