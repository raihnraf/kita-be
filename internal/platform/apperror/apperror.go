package apperror

import "fmt"

type Kind string

const (
	KindBadRequest   Kind = "bad_request"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
)

type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func Newf(kind Kind, format string, args ...any) *Error {
	return New(kind, fmt.Sprintf(format, args...))
}

func BadRequest(message string) *Error {
	return New(KindBadRequest, message)
}

func Unauthorized(message string) *Error {
	return New(KindUnauthorized, message)
}

func Forbidden(message string) *Error {
	return New(KindForbidden, message)
}

func NotFound(message string) *Error {
	return New(KindNotFound, message)
}

func Conflict(message string) *Error {
	return New(KindConflict, message)
}

func Conflictf(format string, args ...any) *Error {
	return Newf(KindConflict, format, args...)
}
