package testdb

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestDB mở pool GORM cho schema test tạm.
//
// VÌ SAO KHÔNG GỌI `platform.OpenPostgres`: từ M4, `platform/di.go` import
// `internal/infrastructure/*` để dựng repository + service. Nếu helper này import
// `platform` thì in-package test của `infrastructure/{roadmap,srs}` (cần nắm
// `txHandle` không export — xem `internal_test.go` của 2 package đó) tạo vòng:
//
//	roadmapinfra (test) → platform/testdb → platform → roadmapinfra
//
// 4 dòng cấu hình GORM bên dưới là bản sao CỐ Ý của `platform.OpenPostgres`,
// chỉ phần test cần: tắt log SQL, `NowFunc` UTC, pool nhỏ, ping. Toàn bộ logic
// DDL vẫn dùng chung qua `internal/migrate.Up` — đó là phần phải không trôi.
//
// Nếu sau này `OpenPostgres` đổi cấu hình quan trọng (ví dụ bật `search_path`
// qua DSN param, hay set `ApplicationName`), phải sửa ở đây cùng lúc. Comment
// này là để người sau biết chỗ cần sửa, không phải để dỡ dỗi.
func openTestDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Tắt log SQL: test chạy hàng trăm query, in ra sẽ ngập output và làm
		// khó thấy phần log muốn. Bật lại bằng `LOG_LEVEL=debug` khi cần.
		Logger: logger.Discard,
		// Cột thời gian trong schema là TEXT do trigger sinh ở UTC; `NowFunc`
		// local sẽ khiến test ghi mốc lệch 7 tiếng so với production.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("mở Postgres test: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("lấy sql.DB test: %w", err)
	}
	// Pool nhỏ: 1 test chỉ cần 2 connection, và nhiều package test chạy SONG SONG
	// trên cùng 1 database (`MigrateLockKey` chỉ serialize MIGRATION, không
	// serialize phần test) nên tổng pool nhân lên số package.
	//
	// Vì sao 2 chứ không 5: `go test ./...` mặc định chạy `-p` = số CPU package
	// song song. Với 3 connection/pool × 2 pool/package × 10 package test =
	// 60 + các conn khoá advisory, đủ vượt `max_connections` (100) của image
	// `postgres:18`. Đã xảy ra: "FATAL: sorry, too many clients already".
	//
	// Vì sao KHÔNG phải 1: M1 §5 ghi rõ `MaxOpenConns = 1` làm repository test
	// treo kẹt — `rows` chưa trả về pool thì query con kẹt vô hạn. 2 là mức
	// nhỏ nhất còn an toàn cho luật đó.
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping Postgres test: %w", err)
	}
	return db, nil
}
