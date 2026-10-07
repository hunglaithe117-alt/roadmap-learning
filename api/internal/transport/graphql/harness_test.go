package graphql_test

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	roadmapapp "langapp/internal/application/roadmap"
	audioinfra "langapp/internal/infrastructure/audio"
	platform "langapp/internal/platform"
	platformtestdb "langapp/internal/platform/testdb"
	gqltransport "langapp/internal/transport/graphql"
)

// harness là bộ dựng cho test `graph/client`: 1 schema Postgres thật + DI thật
// qua `platform.Wire` + handler gqlgen dựng bằng `gqltransport.NewServer`.
//
// Vì sao `client.Client` thay vì `httptest`: plan §3 yêu cầu "~30% graph/client —
// kiểm schema↔resolver wiring + dataloader batching, KHÔNG cần httptest".
// `client.MustPost` chạy đúng pipeline của gqlgen (transport → extension →
// validation → execution → resolver) nên bắt được lỗi wiring mà gọi resolver
// trực tiếp không thấy.
//
// Vì sao dùng `platform.Wire` thay vì dựng tay: adapter sai trong test là kiểu
// "test xanh còn app hỏng" — và 3 adapter (deck reader, STT, TTS) là chỗ dễ sai
// nhất. `platform.Wire` là composition root thật.
type harness struct {
	container *platform.Container
}

// newHarness dựng schema Postgres đã migrate + toàn bộ service.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return wireOn(t, platformtestdb.Open(t, context.Background()))
}

// newSession dựng harness mà test có thể MỞ THÊM schema trong cùng database.
//
// Vì sao cần: `pg_advisory_lock` KHÔNG xếp hồng — session thứ 2 chờ vô hạn
// (đã ghi ở `testdb.Acquire`). Test đo N+1 cần 2 schema (1 topic vs 51 topic) nên
// phải `Acquire` MỘT LẦN rồi `OpenSchema` nhiều lần; gọi `newHarness` 2 lần sẽ treo
// vô hạn, không phải chậm.
func newSession(t *testing.T) (*harness, *platformtestdb.Session) {
	t.Helper()
	sess := platformtestdb.Acquire(t, context.Background())
	return sessionHarness(t, sess), sess
}

func sessionHarness(t *testing.T, sess *platformtestdb.Session) *harness {
	t.Helper()
	db, _ := sess.OpenSchema(t, context.Background())
	return wireOn(t, db)
}

// newCountingHarness là `newHarness` + trả về bộ đếm statement.
//
// Bộ đếm phải được gắn vào pool TRƯỚC khi `platform.Wire` dựng service — đếm ở
// tầng Go thì không đo được SQL thật, và đếm sau khi wire thì service đang giữ
// pool không có logger đếm.
func newCountingHarness(t *testing.T) (*harness, *sqlCounter) {
	t.Helper()
	return countingHarness(t, platformtestdb.Open(t, context.Background()))
}

func countingHarness(t *testing.T, db *gorm.DB) (*harness, *sqlCounter) {
	t.Helper()
	counter := &sqlCounter{}
	counted := db.Session(&gorm.Session{Logger: &countingLogger{counter: counter}})
	return wireOn(t, counted), counter
}

func wireOn(t *testing.T, db *gorm.DB) *harness {
	t.Helper()
	// Audio STUB: test không được phụ thuộc audio-service (image Whisper 8GB).
	// `NewFromEnv("")` trả stub cho cả 2 chiều — đúng như production khi
	// `AUDIO_GRPC_ADDR` rỗng.
	audio, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	// Trả audio về 1 lần để không rò connection pool gRPC. KHÔNG đóng `c`:
	// `testdb` đã đóng pool trong `t.Cleanup`, đóng 2 lần là thừa.
	t.Cleanup(func() { _ = audio.Close() })

	c, err := platform.Wire(db, testLogger(), audio)
	require.NoError(t, err)
	return &harness{container: c}
}

// client trả `*client.Client` trên `NewServer` với introspection BẬT (dev).
func (h *harness) client(t *testing.T) *client.Client {
	t.Helper()
	opts := gqltransport.Options{Introspection: true, Dev: true, Log: testLogger()}
	return client.New(gqltransport.NewServer(h.containerResolver(), opts))
}

// prodClient là client với introspection TẮT — dùng cho test "introspection bị
// chặn ở production".
func (h *harness) prodClient(t *testing.T) *client.Client {
	t.Helper()
	opts := gqltransport.Options{Introspection: false, Dev: false, Log: testLogger()}
	return client.New(gqltransport.NewServer(h.containerResolver(), opts))
}

func (h *harness) containerResolver() *gqltransport.Resolver {
	c := h.container
	return gqltransport.NewResolver(c.SRS, c.Content, c.Roadmap, c.Practice, c.Insight, c.Sync)
}

// sqlCounter đếm số statement GORM gửi cho 1 khối code — công cụ đo N+1.
// `dataloadgen` bảo đảm batching ở TẦNG GO; bảo đảm duy nhất là đếm statement
// THẬT gửi cho Postgres.
type sqlCounter struct{ n atomic.Int64 }

func (c *sqlCounter) Count() int64 { return c.n.Load() }

type countingLogger struct {
	counter *sqlCounter
	inner   logger.Interface
}

func (l *countingLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &countingLogger{counter: l.counter, inner: l.inner.LogMode(level)}
}

func (l *countingLogger) Info(ctx context.Context, msg string, args ...any) {
	if l.inner != nil {
		l.inner.Info(ctx, msg, args...)
	}
}

func (l *countingLogger) Warn(ctx context.Context, msg string, args ...any) {
	if l.inner != nil {
		l.inner.Warn(ctx, msg, args...)
	}
}

func (l *countingLogger) Error(ctx context.Context, msg string, args ...any) {
	if l.inner != nil {
		l.inner.Error(ctx, msg, args...)
	}
}

func (l *countingLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	l.counter.n.Add(1)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// roadmapInput là input tạo path dùng chung cho test cần 1 path có sẵn.
func roadmapInput(slug string) roadmapapp.PathInput {
	return roadmapapp.PathInput{Slug: slug, Title: "path " + slug, Language: "zh"}
}
