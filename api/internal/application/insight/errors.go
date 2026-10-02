package insight

import (
	"errors"
	"fmt"
)

// Error là lỗi nghiệp vụ mang HTTP status sẵn. Transport (M4) chỉ cần
// `errors.As(err, &appErr)` rồi `appErr.Status`.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Status OK của HTTP, khai báo tại chỗ để application không import net/http.
const (
	StatusOK                  = 200
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusInternalServerError = 500
)

// ErrNotFound là sentinel cho "không tìm thấy".
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}
