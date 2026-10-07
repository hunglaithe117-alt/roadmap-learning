// Package apperr provides HTTP status code mapping for application errors.
package apperr

import (
	"errors"
	"net/http"
)

// statusCoder retrieves an HTTP status code from an application error.
type statusCoder interface{ StatusCode() int }

// Status returns the HTTP status code for err, defaulting to 500 if unrecognized.
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var sc statusCoder
	if errors.As(err, &sc) {
		return sc.StatusCode()
	}
	return http.StatusInternalServerError
}

