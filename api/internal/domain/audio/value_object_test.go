package audio

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func f64(v float64) *float64 { return &v }

func Test_word_timestamp_duration_needs_both_bounds(t *testing.T) {
	got, ok := WordTimestamp{Word: "héllo", Start: f64(1.0), End: f64(1.5)}.Duration()
	require.True(t, ok)
	assert.Equal(t, 500*time.Millisecond, got)

	_, ok = WordTimestamp{Word: "x", Start: f64(1.0)}.Duration()
	assert.False(t, ok, "thiếu End thì không suy ra được độ dài")

	_, ok = WordTimestamp{Word: "x", End: f64(1.0)}.Duration()
	assert.False(t, ok, "thiếu Start thì không suy ra được độ dài")
}

func Test_transcript_allows_absent_word_detail(t *testing.T) {
	// Engine không có word-level timestamp -> Words nil, Confidence nil; đây là
	// trạng thái hợp lệ, KHÔNG phải lỗi.
	tr := Transcript{Text: "hello", Lang: "en"}
	assert.Nil(t, tr.Words)
	assert.Nil(t, tr.Confidence)
}

func Test_new_engine_info_marks_stub(t *testing.T) {
	real := NewEngineInfo("piper", KindTTS, true)
	assert.Equal(t, "piper", real.Name)
	assert.Equal(t, KindTTS, real.Kind)
	assert.True(t, real.Real)

	stub := NewEngineInfo("stub", KindTTS, false)
	assert.False(t, stub.Real, "engine stub phải đánh dấu để UI cảnh báo kết quả không thật")
}

func Test_engine_header_name_is_stable(t *testing.T) {
	assert.Equal(t, "X-Engine", Header, "client đọc header này kể cả khi request lỗi")
}

// Compile-time check: 2 port phải dùng được như interface thật. Ở M4
// transport/grpc sẽ thay bằng client sinh từ proto — nếu interface đổi lạ,
// chỗ này gõ compile ngay thay vì lúc chạy.
var (
	_ TTSSynthesizer      = (TTSSynthesizer)(nil)
	_ STTTranscriber      = (STTTranscriber)(nil)
	_ LangSynthesizer     = (LangSynthesizer)(nil)
	_ DetailedTranscriber = (DetailedTranscriber)(nil)
)
