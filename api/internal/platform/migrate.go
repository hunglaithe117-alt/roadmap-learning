package platform

import (
	"context"

	"gorm.io/gorm"

	"langapp/internal/migrate"
)

// File này là lớp gọi lại MỏNG của `internal/migrate`.
//
// Lý do tồn tại: `platform.Migrate` / `platform.MigrationStatus` là API mà M1
// đã dựng và test M1 đang assert. Toàn bộ logic đã chuyển sang `internal/migrate`
// vì `platform` từ M4 import `internal/infrastructure/*` (DI wiring), mà
// `platform/testdb` import ngược lại `platform` — hai chiều đó tạo vòng import
// cho in-package test của infrastructure.
//
// Giữ alias thay vì xoá hẳn: 1 dòng gọi lại, không đổi chữ ký, không đổi call
// site nào, và comment ở đây nói rõ lý do để không ai "dọn cho gọn" rồi tái
// tạo vòng import.

// MigrationStatus là kết quả Up — alias của `migrate.Status`.
type MigrationStatus = migrate.Status

// Migrate chạy migration — xem `internal/migrate.Up`.
func Migrate(ctx context.Context, db *gorm.DB) (MigrationStatus, error) {
	return migrate.Up(ctx, db)
}
