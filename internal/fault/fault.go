// Package fault defines safe, actionable diagnostics. Never wrap upstream
// response bodies or URL-bearing transport errors in a user-visible message.
package fault

import (
	"context"
	"errors"
)

type Kind string

const (
	Configuration  Kind = "configuration"
	Authentication Kind = "authentication"
	Permission     Kind = "permission"
	Lookup         Kind = "lookup"
	Task           Kind = "task"
	TLS            Kind = "TLS"
	Protocol       Kind = "compatibility"
	Connection     Kind = "console connection"
)

type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string            { return string(e.Kind) + ": " + e.Message }
func New(kind Kind, message string) error { return &Error{kind, message} }

func Diagnostic(err error) (string, int) {
	if errors.Is(err, context.Canceled) {
		return "Interrupted; session closed.", 130
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "task: setup timed out; check PC connectivity and rerun the command.", 7
	}
	var e *Error
	if !errors.As(err, &e) {
		return "console connection: operation failed; check connectivity and rerun the command.", 1
	}
	codes := map[Kind]int{Configuration: 2, Authentication: 3, Permission: 4, Lookup: 5, TLS: 6, Task: 7, Protocol: 8, Connection: 9}
	code := codes[e.Kind]
	if code == 0 {
		code = 1
	}
	return e.Error(), code
}
