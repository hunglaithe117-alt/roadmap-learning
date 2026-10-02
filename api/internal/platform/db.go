package platform

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// CẤM `db.Model(&X{}).Update(...)` — UPDATE không có khóa chính, GORM không
// sinh `WHERE id = ?`. Với `AllowGlobalUpdate: false` (đặt ở OpenPostgres) GORM
// KHÔNG chạy câu lệnh mà trả `ErrMissingWhereClause`; đó là lý do câu lệnh
// không bao giờ chạm bảng. Nếu ai đó bật `AllowGlobalUpdate: true` thì câu
// UPDATE chạy trên TOÀN BỘ bảng, và hậu quả kép:
//  1. Trigger `langapp_touch_updated_at` (BEFORE UPDATE) gọp lỡ vào mọi row →
//     mọi `updated_at` trong bảng bị đổi → merge LWW phía peer chọn nhầm bản
//     cũ là bản mới.
//  2. Không thể rollback từng row.
//
// Bắt buộc: `db.Model(&x).Updates(...)` theo INSTANCE (GORM tự thêm
// `WHERE id = ?`) hoặc `db.Where("id = ?", id).Updates(...)`.
// Test cho trigger nằm ở internal/platform/migrate_test.go.
const allowGlobalUpdateWarning = "global update bị chặn: dùng db.Model(&instance).Updates(...)"

// OpenPostgres mở pool GORM với DSN Postgres, đặt pool sizing và tắt logger
// mặc định của GORM (nó in SQL ra stdout, không đi qua slog).
//
// KHÔNG set PRAGMA ở đây: đây là Postgres, PRAGMA là cú pháp SQLite và sẽ
// lỗi syntax. Cấu hình tương đương của Postgres là `SET ...` / DSN params,
// để infrastructure lo khi thật sự cần.
func OpenPostgres(ctx context.Context, cfg Config) (*gorm.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	gormCfg := &gorm.Config{
		Logger: gormlogger.Discard,
		// Mặc định GORM là false; đặt tường minh để session kế thừa không
		// ai đó bật lên ở tầng trên.
		AllowGlobalUpdate: false,
		NowFunc:           func() time.Time { return time.Now().UTC() },
	}
	db, err := gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("platform: mở Postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("platform: lấy sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("platform: ping Postgres: %w", err)
	}
	slog.Debug("postgres pool sẵn sàng",
		"max_open", cfg.MaxOpenConns, "max_idle", cfg.MaxIdleConns,
		"conn_max_lifetime", cfg.ConnMaxLifetime)
	return db, nil
}

// NewGormLogger gắn log SQL của GORM vào slog thay vì bỏ hẳn. Chỉ bật ở
// debug: ở info, mỗi query in ra là nhiễu với app ~30 endpoint.
func NewGormLogger(level slog.Level) gormlogger.Interface {
	return gormlogger.New(
		slogWriter{},
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel(level),
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		},
	)
}

func gormLogLevel(level slog.Level) gormlogger.LogLevel {
	if level <= slog.LevelDebug {
		return gormlogger.Info
	}
	return gormlogger.Warn
}

// slogWriter là io.Writer mà GORM dùng khi muốn in SQL; chuyển thành slog
// record để mọi log đi cùng 1 format JSON.
type slogWriter struct{}

func (slogWriter) Write(p []byte) (int, error) {
	slog.Debug("gorm", "sql", strings.TrimSpace(string(p)))
	return len(p), nil
}

// Printf là interface logger.Writer của GORM. GORM chỉ gọi Printf khi
// ParameterizedQueries = false; cài slogWriter để log vẫn ra JSON thay vì
// mất im lặng.
func (slogWriter) Printf(format string, args ...any) {
	slog.Debug(fmt.Sprintf("gorm: "+format, args...))
}
