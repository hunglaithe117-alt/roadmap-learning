package practice

import (
	"errors"
	"fmt"
)

// Error là lỗi nghiệp vụ mang HTTP status sẵn. Transport (M4) chỉ cần
// `errors.As(err, &appErr)` rồi `appErr.Status` — không phải dịch lại chuỗi
// message (làm vỡ khi ai đó đổi wording).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Status OK của HTTP, khai báo tại chỗ để application không import net/http
// (tầng này không được phụ thuộc framework).
const (
	StatusOK                  = 200
	StatusCreated             = 201
	StatusBadRequest          = 400
	StatusNotFound            = 404
	StatusConflict            = 409
	StatusInternalServerError = 500
)

// ErrNotFound là sentinel cho "không tìm thấy" — repository trả về khi row
// không còn. Use case bọc lại thành *Error 404 với message tiếng Việt.
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// wrapNotFound dịch sentinel ErrNotFound thành *Error 404; lỗi kỹ thuật khác
// đi nguyên vẹn (transport sẽ 500).
func wrapNotFound(err error, msg string) error {
	if errors.Is(err, ErrNotFound) {
		return newError(StatusNotFound, "%s", msg)
	}
	return err
}
