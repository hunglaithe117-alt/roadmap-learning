package graphql

import (
	"errors"
	"log/slog"
	"net/http"

	"langapp/internal/transport/apperr"
	"langapp/internal/transport/graphql/model"
)

// statusOf trả status nghiệp vụ của err, hoặc 500 — xem `apperr.Status`.
func statusOf(err error) int {
	if status, ok := statusOfTransport(err); ok {
		return status
	}
	// 6 kiểu lỗi nghiệp vụ của application được `apperr` nhận diện — xem
	// `internal/transport/apperr` vì sao không dùng interface chung.
	return apperr.Status(err)
}

// statusOfTransport đọc status của lỗi thuần tầng transport, kiểm TRƯỚC 6 kiểu
// application vì `errors.As` không bắt được kiểu khác.
func statusOfTransport(err error) (int, bool) {
	var se *statusError
	if errors.As(err, &se) {
		return se.status, true
	}
	return 0, false
}

// httpStatusBadRequest là hằng cho lỗi tham số của TRANSPORT (không phải của
// application). Dùng hằng số thay vì import `net/http` chỉ để đọc 1 số: mọi
// status khác đến từ `apperr` theo hằng của tầng dưới.
const httpStatusBadRequest = 400

// errBadRequest bọc lỗi tham số của TRANSPORT thành dạng `statusOf` nhận diện
// được, để `toUserError` gắn `ErrorCodeBadRequest` thay vì rơi về `INTERNAL`.
//
// Cần vì `errInvalidSince` là lỗi thuần của transport: nó không đi qua use case
// nào, nên không mang status của application.
func errBadRequest(err error) error { return &statusError{status: httpStatusBadRequest, err: err} }

// statusError là lỗi tầng transport có status.
type statusError struct {
	status int
	err    error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

// internalMessage là message trả về cho lỗi HỆ THỐNG. Không rò chi tiết driver ra
// ngoài: lỗi GORM/Postgres chứa câu SQL, tên bảng, tên cột và SQLSTATE — đủ để
// kẻ khách lập bản đồ DB mà không cần đăng nhập (app v1 không có auth).
const internalMessage = "lỗi hệ thống"

// toUserError dịch lỗi sang `UserError` cho client.
//
// BA NHÓM, xử lý khác nhau:
//
//  1. Lỗi tham số của TRANSPORT (`statusError`): message nguyên văn — nó do
//     chính tầng này soạn ("since phải có dạng YYYY-MM-DD").
//  2. Lỗi NGHIỆP VỤ (`apperr.IsBusiness`): message NGUYÊN VĂN, không dịch lại,
//     không viết hoa. Tầng application đã soạn sẵn tiếng Việt theo đúng loại
//     nút ("không tìm thấy stage", "slug learning path đã tồn tại"); dịch lại ở
//     đây là cách chắc chắn làm lệch.
//  3. Lỗi HỆ THỐNG: chỉ log server-side, trả message chung. Đường leak thật đã
//     xảy ra: `application/insight` bọc `fmt.Errorf("đọc lịch sử ôn: %w", err)`
//     nên `Query.stats` đưa thẳng ra `relation "reviews" does not exist
//     (SQLSTATE 42P01)` cho client. `errorPresenter` KHÔNG cứu được vì resolver
//     trả payload với `error = nil` cho GraphQL.
func toUserError(err error) *model.UserError {
	if err == nil {
		return nil
	}
	// `statusOf` đọc `statusError` trước 6 kiểu application — lỗi tham số của
	// transport không mang status của application.
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

// isBusinessStatus báo status có phải nghiệp vụ (400/404/409/501) hay 500.
//
// 501 (Not Implemented) là nghiệp vụ ở đây dù nghe như lỗi hệ thống: message
// `"chưa cấu hình nguồn snapshot peer"` do chính `application/sync` soạn, và
// nó **không** chứa chi tiết driver nào để che. Nếu loại nó ra khỏi danh sách
// này thì `toUserError` biến nó thành `"lỗi hệ thống"` + `INTERNAL` — tức sửa
// xong panic 500 ở M7c thì user lại **không đọc được** vì sao, quay về đúng
// bài toán cũ. Xem `application/sync/service.go` `Sync` để đọc vì sao chọn
// 501 thay vì 409/503.
func isBusinessStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

// logSystemError ghi lỗi hệ thống ra log server-side. Dùng `slog.Default()` vì
// `toUserError` là hàm đứng riêng, không mang logger theo (mọi resolver gọi nó
// không có biến logger nào để truyền).
func logSystemError(err error) {
	slog.Error("lỗi hệ thống trong resolver GraphQL, đã che message cho client",
		slog.String("err", err.Error()))
}

// codeForStatus map status của application sang enum GraphQL. 4 hằng status mà
// application dùng (400/404/409/500) là hợp đồng đóng băng — thêm hằng thứ 5 ở
// tầng dưới mà không sửa hàm này sẽ rơi về INTERNAL, nên default là INTERNAL
// (an toàn: không rò chi tiết lỗi hệ thống ra client).
//
// 501 → NOT_IMPLEMENTED: cần hằng riêng vì client dùng `code` để QUYẾT ĐỊNH
// hiển thị, không chỉ để hiện chữ. "Tính năng chưa bật" (hiện "đang xây") và
// "server hỏng" (hiện lỗi + nút thử lại) là 2 hành vi khác nhau; gộp vào
// CONFLICT thì user bị bảo rằng dữ liệu của họ xung đột, gộp vào INTERNAL thì
// user bị bảo thử lại một việc sẽ không bao giờ thành công.
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
