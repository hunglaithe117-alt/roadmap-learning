package roadmap

import (
	"errors"
	"fmt"
)

// Error là lỗi nghiệp vụ mang HTTP status sẵn. Tầng transport (M4) chỉ cần
// `errors.As(err, &appErr)` rồi `appErr.Status` → không phải dịch lại chuỗi
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
// không còn (đã xoá mềm). Use case bọc lại thành *Error 404 với message
// tiếng Việt của đúng loại nút.
var ErrNotFound = errors.New("không tìm thấy")

// newError dựng *Error. Nhãn là tiếng Việt vì đây là thứ user đọc thấy.
func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// IsNotFound báo err có phải *Error 404 không (transport dùng để log, test
// dùng để assert).
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == StatusNotFound
}
