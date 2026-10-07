package roadmap

import (
	"errors"
	"fmt"
)

// Error represents a business error with an associated HTTP status code.
type Error struct {
	Status  int
	Message string
}

// Error returns the business error message.
func (e *Error) Error() string { return e.Message }

// StatusCode returns the HTTP status code.
func (e *Error) StatusCode() int { return e.Status }

// HTTP status codes defined locally to avoid importing net/http in the application layer.
const (
	StatusOK                  = 200
	StatusCreated             = 201
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusConflict            = 409
	StatusInternalServerError = 500
)

// ErrNotFound signals that a requested entity was not found.
var ErrNotFound = errors.New("không tìm thấy")

// newError creates an Error instance.
func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// IsNotFound reports whether err represents a 404 not found error.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == StatusNotFound
}
