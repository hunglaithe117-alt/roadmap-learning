package content

import (
	"errors"
	"fmt"
	"strings"

	domain "langapp/internal/domain/content"
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

// ErrLangMismatch indicates a language mismatch on an existing entity.
var ErrLangMismatch = errors.New("ngôn ngữ không khớp")

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

// wrapTone dịch lỗi thuần của bộ thanh (domain/content.ToneError) thành 400 với
// giữ nguyên wording tiếng Việt mà app v1 trả về.
func wrapTone(err error) error {
	var te domain.ToneError
	if errors.As(err, &te) {
		return newError(StatusBadRequest, "%s", te.Error())
	}
	return err
}

// normalizeLang map các biến thể hợp lệ của cùng 1 ngôn ngữ về zh|en.
func normalizeLang(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zh", "zh-cn", "zh_cn", "cn":
		return "zh", true
	case "en", "en-us", "en_us":
		return "en", true
	default:
		return "", false
	}
}
