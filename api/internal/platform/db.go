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

// OpenPostgres opens a GORM connection pool with the given configuration.
func OpenPostgres(ctx context.Context, cfg Config) (*gorm.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	gormCfg := &gorm.Config{
		Logger: gormlogger.Discard,
		// Explicitly disable global updates without a WHERE clause.
		AllowGlobalUpdate: false,
		NowFunc:           func() time.Time { return time.Now().UTC() },
	}
	db, err := gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("platform: open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("platform: get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("platform: ping postgres: %w", err)
	}
	slog.Debug("postgres pool ready",
		"max_open", cfg.MaxOpenConns, "max_idle", cfg.MaxIdleConns,
		"conn_max_lifetime", cfg.ConnMaxLifetime)
	return db, nil
}

// NewGormLogger routes GORM SQL logging to slog at debug level.
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

// slogWriter adapts slog to io.Writer for GORM logs.
type slogWriter struct{}

func (slogWriter) Write(p []byte) (int, error) {
	slog.Debug("gorm", "sql", strings.TrimSpace(string(p)))
	return len(p), nil
}

func (slogWriter) Printf(format string, args ...any) {
	slog.Debug(fmt.Sprintf("gorm: "+format, args...))
}

