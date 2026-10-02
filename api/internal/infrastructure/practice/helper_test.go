package practiceinfra_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	app "langapp/internal/application/practice"
	practiceinfra "langapp/internal/infrastructure/practice"
	"langapp/internal/platform/testdb"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, context.Background())
}

// fakeSTT là engine nhận dạng giọng nói giả lập trả CHỮ PHỒN — đúng hành vi
// thật của Whisper, và là lý do `DiffAgainstSample` phải chuẩn hóa trước khi so.
type fakeSTT struct{ text string }

func (f fakeSTT) Transcribe(context.Context, []byte, string, string) (app.Transcript, error) {
	return app.Transcript{Text: f.text, Lang: "zh"}, nil
}

type fakeTTS struct{}

func (fakeTTS) Synthesize(_ context.Context, text, lang string) ([]byte, string, error) {
	return []byte("WAV:" + text), "audio/wav", nil
}

func newService(t *testing.T, db *gorm.DB) *app.Service {
	t.Helper()
	return app.NewService(practiceinfra.NewRepository(db), practiceinfra.NewUnitOfWork(db),
		fakeSTT{text: "學習中文"}, fakeTTS{}, func() time.Time { return fixedNow })
}
