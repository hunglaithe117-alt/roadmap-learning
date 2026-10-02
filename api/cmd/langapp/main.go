// Command langapp là entrypoint của app STACK-V2 (M4).
//
// App CŨ ở `api/main.go` (package main, `net/http` + `modernc.org/sqlite`) vẫn
// build và test xanh, KHÔNG đụng 1 dòng. 2 entrypoint tồn tại song song cho tới
// M7 — đó là chủ ý để luồn port không bị gián đoạn: mỗi milestone có 1 app
// chạy được thay vì 1 app vừa sửa vừa chạy.
//
// Thứ tự boot (STACK-V2-PLAN §5, bắt buộc):
//
//  1. LoadConfig            (platform.Build, tự đọc env)
//  2. logger slog
//  3. OpenPostgres          (pool)
//  4. goose.Up              (migrate)
//  5. Seed                  (roadmap seed — 1 lần, idempotent)
//  6. DI graph              (repository + 6 service + audio client)
//  7. HTTP server
//  8. graceful shutdown     (SIGINT/SIGTERM → Shutdown → đóng DB)
//
// Bước 5 đứng TRƯỚC 6 theo yêu cầu M3-final §9, và cả hai đứng trước 7 để
// request đầu tiên đã thấy dữ liệu seed.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"langapp/internal/platform"
	gqltransport "langapp/internal/transport/graphql"
	httptransport "langapp/internal/transport/http"
)

func main() {
	// `context.Background` là gốc của cả tiến trình; `signal.NotifyContext` bám
	// vào nó để SIGTERM huỷ luôn `ctx` của `http.Server.BaseContext` — nhờ vậy
	// mọi lệnh DB/audio của request đang chạy bị huỷ theo thay vì treo.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		// Logger đã do `platform.Build` set làm default; nếu hỏng trước lúc đó
		// thì `slog.Default()` vẫn là JSON handler mặc định.
		slog.Error("langapp dừng vì lỗi", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// ── 1-4: config → logger → Postgres → migrate ───────────────────────────
	c, err := platform.Build(ctx)
	if err != nil {
		return err
	}
	// `Close` idempotent + nil-safe nên `defer` không cần if. Gọi 2 lần là vô
	// hại: lần thứ 2 `sql.DB.Close` trả `sql: database is closed` — nhưng đó là
	// lỗi DUY NHẤT được nuốt, vì nó do chính `defer` lần trước gây ra.
	defer func() {
		if cerr := c.Close(); cerr != nil && !errors.Is(cerr, http.ErrServerClosed) {
			c.Logger.Error("đóng connection thất bại", slog.String("err", cerr.Error()))
		}
	}()

	// ── 5: seed ─────────────────────────────────────────────────────────────
	// Lỗi seed KHÔNG làm app chết. Seed là dữ liệu khởi tạo (2 path / 10 stage
	// / 51 topic / 263 resource); app vẫn dùng được với DB rỗng, và người dùng
	// sẽ tự tạo path của mình. Giết container vì 1 topic seed hỏng thì tệ hơn
	// nhiều so với log cảnh báo.
	if err := c.BootStep(ctx, "seed roadmap", c.Seed); err != nil {
		c.Logger.Warn("seed roadmap thất bại, app vẫn chạy với dữ liệu sẵn có",
			slog.String("err", err.Error()))
	}

	// ── 6-7: transport + HTTP server ────────────────────────────────────────
	srv, err := buildHTTP(c, ctx)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		c.Logger.Info("langapp lắng nghe",
			slog.String("addr", srv.Addr),
			slog.String("gin_mode", c.Config.GinMode),
			slog.String("audio", c.Audio.EngineInfo().Name),
			slog.Bool("audio_real", c.Audio.IsReal()),
			slog.String("grpc_audio", c.Audio.Address),
			slog.Int64("goose_version", c.Migration.Version))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
		// ── 8: graceful shutdown ────────────────────────────────────────────
		c.Logger.Info("nhận tín hiệu dừng, đang shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// Không trả lỗi: `Shutdown` hết giờ nghĩa là còn request treo, nhưng
			// `defer c.Close()` + `os.Exit` vẫn dọn connection. Báo lỗi ở đây sẽ
			// khiến exit code khác 0 dù container đã dừng đúng cách.
			c.Logger.Warn("shutdown hết giờ, còn request đang chạy",
				slog.String("err", err.Error()))
		}
		// Chờ `ListenAndServe` kết thúc để không rò goroutine; `defer c.Close()`
		// phía trên chỉ chạy khi hàm này trả về.
		<-serveErr
		c.Logger.Info("đã dừng")
		return nil
	}
}

// shutdownGrace là khoảng chờ cho request đang chạy. Lớn hơn
// `ServerTimeouts.Write` (60s) để 1 request TTS dài có cơ hội xong; nhỏ hơn
// `docker compose stop` mặc định (10s) thì... lớn hơn. Chọn 15s vì `pg_dump`/
// restore có trần 1 phút ở handler backup, và cắt ngang giữa lúc đó để lại DB
// ở trạng thái nửa vời.
const shutdownGrace = 15 * time.Second

// buildHTTP dựng `http.Server` (đã cấu hình timeout) từ container.
//
// `ctx` phải là context GỐC của process (`signal.NotifyContext`) — truyền sai thì
// `BaseContext` không bị huỷ khi SIGTERM và các request đang chạy bị cắt ngang
// thay vì dừng dần (F4).
func buildHTTP(c *platform.Container, ctx context.Context) (*http.Server, error) {
	sqlDB, err := c.SQLDB()
	if err != nil {
		return nil, err
	}

	resolver := gqltransport.NewResolver(c.SRS, c.Content, c.Roadmap, c.Practice, c.Insight, c.Sync)
	// `Dev` bật introspection + playground + CORS cho Vite. `ENV=production` là
	// chốt chặn duy nhất — xem `gqltransport.Options`.
	dev := c.Config.GinMode != "release"
	gqlOpts := gqltransport.Options{Introspection: dev, Dev: dev, Log: c.Logger}
	// BackupPort: M4 KHÔNG implement (`VACUUM INTO` không tồn tại ở Postgres,
	// `pg_dump` thuộc M7). Truyền nil ⇒ `/api/backup` + `/api/restore` trả 501
	// với message nói rõ thay vì 500.
	//
	// Khi M7 bật: truyền `c.Backup` — và `c.Backup` khai là
	// `syncapp.BackupPort` (INTERFACE), KHÔNG phải `*pgBackup`. Lý do ở B1 của
	// `stack-v2-typednil.md`: nếu khai con trỏ rồi truyền con trỏ nil (thiếu
	// binary `pg_dump`), interface vẫn non-nil ⇒ `port == nil` sai ⇒
	// `port.Dump(ctx)` deref nil ⇒ panic 500 thay vì 501. Handler đã dùng
	// `backupDisabled` để chặn cả 2 hình dạng, nhưng khai interface từ đầu thì
	// wiring sai không hề biên dịch được — lớp chặn thứ 2.
	router := httptransport.NewRouter(httptransport.Options{
		GinMode:    c.Config.GinMode,
		WebDist:    webDist(),
		Dev:        dev,
		TTS:        c.Audio.TTSSynth,
		STT:        c.Audio.STTSTT,
		Backup:     nil,
		GraphQL:    gqltransport.NewServer(resolver, gqlOpts),
		Playground: gqltransport.PlaygroundHandler(gqlOpts),
		DB:         sqlDB,
		// Func đọc TỨC THÌ, không truyền `&c.Audio.EngineInfo` — bản sao lúc
		// boot không đổi nên `/api/health` sẽ báo `degraded` mãi và giết
		// HEALTHCHECK của Docker (F3). `IsReal()` đọc thẳng `engineCache`.
		Audio: func() (string, bool) {
			info := c.Audio.TTSEngine()
			return info.Name, c.Audio.IsReal()
		},
		Log:                c.Logger,
		MaxMultipartMemory: 0, // 0 ⇒ dùng mặc định 10MB của package
	})

	// `ctx` là context gốc của process (đã bám `signal.NotifyContext`) ⇒ khi
	// nhận SIGTERM, `BaseContext` của `http.Server` bị huỷ theo, mọi lệnh DB và
	// gọi gRPC của request đang chạy dừng ngay thay vì phải chờ `Shutdown` hết
	// 15s rồi cắt ngang giữa chừng. Truyền `context.Background()` ở đây làm doc
	// ở `main` sai và `/api/tts` dở dang đúng lúc dừng container (F4).
	return router.Server(httptransport.DefaultServerTimeouts(), ctx), nil
}

// webDist đọc `WEB_DIST`. Rỗng = KHÔNG mount static (mọi đường dẫn lạ trả 404
// chứ không trả HTML) — đúng cho `go test` và cho lúc chưa build web.
func webDist() string { return os.Getenv("WEB_DIST") }
