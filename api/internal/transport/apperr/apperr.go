// Package apperr là nơi DUY NHẤT ở tầng transport biết các kiểu lỗi nghiệp vụ
// của tầng application.
//
// Cả 6 package application khai một struct `Error{Status int; Message string}`
// giống hệt nhau nhưng là 6 kiểu pháp lý KHÁC NHAU. Cần 1 chỗ để biết "lỗi này
// là 404 hay 500", và GraphQL (`internal/transport/graphql`) lẫn REST
// (`internal/transport/http`) đều cần.
//
// Vì sao không gom về interface `StatusCode() int` ở tầng application: phải sửa
// 6 file dưới (application nằm ngoài phạm vi ghi của M4), và đổi contract của 6
// package chỉ để tiện 1 helper ở trên là đánh đổi sai. Ở đây 6 nhánh
// `errors.As` là giá để đổi lấy "sửa tầng dưới 0 dòng".
//
// QUY TẮC: `Message` đi ra client NGUYÊN VĂN, không dịch, không viết hoa lại.
package apperr

import (
	"errors"
	"net/http"

	contentapp "langapp/internal/application/content"
	insightapp "langapp/internal/application/insight"
	practiceapp "langapp/internal/application/practice"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	syncapp "langapp/internal/application/sync"
)

// Status trả HTTP status của lỗi nghiệp vụ, hoặc 500 nếu không nhận diện được.
//
// 500 chứ không phải 0: 0 sẽ thành HTTP 200 mang body lỗi — lỗi mà client nào
// cũng bỏ qua lúc render, tức lỗi biến mất âm thầm.
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var srsErr *srsapp.Error
	if errors.As(err, &srsErr) {
		return srsErr.Status
	}
	var contentErr *contentapp.Error
	if errors.As(err, &contentErr) {
		return contentErr.Status
	}
	var roadmapErr *roadmapapp.Error
	if errors.As(err, &roadmapErr) {
		return roadmapErr.Status
	}
	var practiceErr *practiceapp.Error
	if errors.As(err, &practiceErr) {
		return practiceErr.Status
	}
	var insightErr *insightapp.Error
	if errors.As(err, &insightErr) {
		return insightErr.Status
	}
	var syncErr *syncapp.Error
	if errors.As(err, &syncErr) {
		return syncErr.Status
	}
	return http.StatusInternalServerError
}

// IsBusiness báo lỗi có phải nghiệp vụ (đã biết status) hay lỗi hệ thống.
//
// Dùng để QUYẾT ĐỊNH có nên đưa message ra client: lỗi hệ thống (GORM, Postgres,
// dial gRPC) chứa câu SQL, tên bảng, địa chỉ — log server-side rồi trả message
// chung; lỗi nghiệp vụ đã do application soạn sẵn cho người dùng.
func IsBusiness(err error) bool {
	return Status(err) != http.StatusInternalServerError
}
