package practice

import (
	"errors"
	"fmt"
)

// Error represents an application-level business error with an HTTP status code.
type Error struct {
	Status  int
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// StatusCode returns the HTTP status code.
func (e *Error) StatusCode() int { return e.Status }

// HTTP status code constants.
const (
	StatusOK                  = 200
	StatusCreated             = 201
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusConflict            = 409
	StatusInternalServerError = 500
)

// ErrNotFound indicates that the requested entity was not found.
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// wrapNotFound wraps ErrNotFound into an application 404 Error.
func wrapNotFound(err error, msg string) error {
	if errors.Is(err, ErrNotFound) {
		return newError(StatusNotFound, "%s", msg)
	}
	return err
}
