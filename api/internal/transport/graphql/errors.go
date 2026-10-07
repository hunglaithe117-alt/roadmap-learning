package graphql

import (
	"errors"
	"log/slog"
	"net/http"

	"langapp/internal/transport/apperr"
	"langapp/internal/transport/graphql/model"
)

// statusOf returns the business status code of err, or 500.
func statusOf(err error) int {
	if status, ok := statusOfTransport(err); ok {
		return status
	}
	return apperr.Status(err)
}

// statusOfTransport checks for a transport-level status error.
func statusOfTransport(err error) (int, bool) {
	var se *statusError
	if errors.As(err, &se) {
		return se.status, true
	}
	return 0, false
}

const httpStatusBadRequest = 400

// errBadRequest wraps an error with httpStatusBadRequest.
func errBadRequest(err error) error { return &statusError{status: httpStatusBadRequest, err: err} }

// statusError wraps an error with an HTTP status code.
type statusError struct {
	status int
	err    error
}

func (s *statusError) Error() string { return s.err.Error() }
func (s *statusError) Unwrap() error { return s.err }

// internalMessage is the generic message returned for internal system errors.
const internalMessage = "lỗi hệ thống"

// toUserError translates an application error to a UserError model.
func toUserError(err error) *model.UserError {
	if err == nil {
		return nil
	}
	status := statusOf(err)
	if _, isTransport := statusOfTransport(err); !isTransport && !isBusinessStatus(status) {
		logSystemError(err)
		return &model.UserError{Message: internalMessage, Code: model.ErrorCodeInternal}
	}
	return &model.UserError{
		Message: err.Error(),
		Code:    codeForStatus(status),
	}
}

// isBusinessStatus reports whether status represents a business error.
func isBusinessStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

// logSystemError logs an internal system error.
func logSystemError(err error) {
	slog.Error("lỗi hệ thống trong resolver GraphQL, đã che message cho client",
		slog.String("err", err.Error()))
}

// codeForStatus maps an HTTP status code to a GraphQL ErrorCode enum.
func codeForStatus(status int) model.ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return model.ErrorCodeBadRequest
	case http.StatusNotFound:
		return model.ErrorCodeNotFound
	case http.StatusConflict:
		return model.ErrorCodeConflict
	case http.StatusNotImplemented:
		return model.ErrorCodeNotImplemented
	default:
		return model.ErrorCodeInternal
	}
}
