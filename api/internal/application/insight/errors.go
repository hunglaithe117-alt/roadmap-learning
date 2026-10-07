package insight

import (
	"errors"
	"fmt"
)

// Error represents an application error with an HTTP status code.
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
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusInternalServerError = 500
)

// ErrNotFound indicates the requested resource was not found.
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}
