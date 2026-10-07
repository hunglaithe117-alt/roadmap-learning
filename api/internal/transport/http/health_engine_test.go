package httptransport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	audiodomain "langapp/internal/domain/audio"
)

// countingTTS là engine TTS giả, nhưng `Info()` TRẢ VỀ GIÁ TRỊ KHÁC NHAU theo
// thời điểm — mô phỏng đúng `engineCache` của adapter gRPC (đổi sau request đầu).
type countingTTS struct {
	name  string
	real  bool
	calls int
}

func (c *countingTTS) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo(c.name, audiodomain.KindTTS, c.real)
}

func (c *countingTTS) Synthesize(context.Context, string) ([]byte, string, error) {
	c.calls++
	// Sau lần gọi đầu, "service" báo engine thật — giống cách adapter đọc
	// `SynthesizeResponse.Engine` / `.Real` rồi `bindEngine`.
	if c.calls > 1 {
		c.name = "piper"
		c.real = true
	}
	return []byte("RIFFaudio"), "audio/wav", nil
}

// F3a — header `X-Engine` phải phản ánh engine THẬT sau khi service đã báo tên.
//
// Trước remediation, handler bắt `info := engine.Info()` ở NGOÀI closure, nên
// header bị ghim ở giá trị lúc khởi tạo vĩnh viễn.
//
// Thứ tự thời gian mô phỏng đúng `engineCache`: `Info()` được đọc TRƯỚC khi gọi
// engine (để gắn header kể cả khi lỗi), còn `engineCache.bind` chạy SAU khi RPC
// trả. Nên tên engine thật xuất hiện ở request KẾ TIẾP — và đó là điều cần
// khẳng định: có đọc lại mỗi request, không ghim 1 bản.
func Test_x_engine_header_follows_engine_after_first_request(t *testing.T) {
	engine := &countingTTS{name: "audio-service", real: false}
	e := NewRouter(Options{GinMode: "test", TTS: engine, STT: StubSTT{}, Log: discardLogger()}).Engine

	// Request 1: engine chưa báo tên thật.
	w1 := httptest.NewRecorder()
	e.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/api/tts?text=a", nil))
	require.Equal(t, http.StatusOK, w1.Code)
	require.Equal(t, "audio-service", w1.Header().Get(audiodomain.Header),
		"trước khi service báo engine, header phải là tên dự kiến")

	// Request 2: `Info()` đọc trước lệnh gọi nên vẫn là tên dự kiến…
	w2 := httptest.NewRecorder()
	e.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/tts?text=a", nil))
	require.Equal(t, "audio-service", w2.Header().Get(audiodomain.Header))

	// …nhưng engine ĐÃ báo "piper" ⇒ request 3 phải phản ánh tên thật.
	w3 := httptest.NewRecorder()
	e.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/tts?text=a", nil))
	require.Equal(t, http.StatusOK, w3.Code)
	require.Equal(t, "piper", w3.Header().Get(audiodomain.Header),
		"header phải đọc lại mỗi request; ghim bản lúc boot sẽ kẹt ở audio-service vĩnh viễn")
}

// Stub thì header phải nói "stub" ở MỌI request — UI dựa vào đó hiện badge
// "kết quả không thật".
func Test_x_engine_header_always_reports_stub_for_stub_engine(t *testing.T) {
	e := testRouter("")
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("request %d", i), func(t *testing.T) {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tts?text=a", nil))
			require.Equal(t, "stub", w.Header().Get(audiodomain.Header), "request %d", i)
		})
	}
}

// F3b — `/api/health` phải phản ánh trạng thái engine ĐỌNG TỨC THÌ.
//
// Lỗi của M4: `Options.Audio` nhận `*EngineInfo` chụp lúc boot, nên `status`
// là `degraded` MÃI. Healthcheck grep `b'"status":"ok"'` ⇒ `app-v2` unhealthy vĩnh
// viễn, và M7 thêm service nào `depends_on: service_healthy` là treo.
func Test_health_reflects_engine_state_read_live_not_snapshot(t *testing.T) {
	engine := &countingTTS{name: "audio-service", real: false}
	// Provider đọc engine mỗi lần — đúng như `main.go` truyền vào.
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{}, TTS: engine, STT: StubSTT{},
		Audio: func() (string, bool) { info := engine.Info(); return info.Name, info.Real },
		Log:   discardLogger(),
	}).Engine

	// Trước khi engine thật: degraded (đúng — audio đang là stub).
	w1 := httptest.NewRecorder()
	e.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusOK, w1.Code)
	require.Contains(t, w1.Body.String(), `"status":"degraded"`)

	// Engine báo tên thật ⇒ phải chuyển sang ok.
	engine.name, engine.real = "piper", true
	w2 := httptest.NewRecorder()
	e.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusOK, w2.Code)
	require.Contains(t, w2.Body.String(), `"status":"ok"`,
		"engine thật phải cho status ok, nếu không HEALTHCHECK của Docker đỏ vĩnh viễn")
}

// `degraded` phải là câu trả lời SỰ THẬT cho stub, không phải hằng số rác — stub
// đọc `Real=false` ngay lúc dựng, không cần chờ request nào.
func Test_health_is_ok_when_audio_provider_reports_real_engine(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{},
		Audio: func() (string, bool) { return "piper", true },
		TTS:   StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"ok"`)
}

// Không cấu hình audio (`AUDIO_GRPC_ADDR` rỗng) thì `Audio` vẫn được truyền
// (main luôn truyền) và phải báo `degraded` — app chạy được, chỉ audio là giả.
func Test_health_degrades_when_stub_engine_reports_not_real(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{},
		Audio: func() (string, bool) { return "stub", false },
		TTS:   StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusOK, w.Code, "degraded vẫn 200 — app dùng được, trả 503 sẽ khiến Docker restart vô ích")
	require.Contains(t, w.Body.String(), `"status":"degraded"`)
}

// DB chết thì `degraded` không được biến thành `ok` — thứ tự kiểm tra phải là
// DB trước, audio sau.
func Test_health_returns_503_regardless_of_audio_state_when_db_down(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{err: context.DeadlineExceeded},
		Audio: func() (string, bool) { return "piper", true },
		TTS:   StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// F4 — `BaseContext` phải là context của PROCESS, không phải `Background()`.
//
// Không thể test qua `http.Server` thật (cần listen thật), nên test ở mức hành
// vi: `BaseContext` phải trả về đúng context mà ta truyền vào, và context đó phải
// bị huỷ khi SIGTERM.
func Test_server_base_context_is_the_process_context(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	r := NewRouter(Options{GinMode: "test", TTS: StubTTS{}, STT: StubSTT{}, Log: discardLogger()})
	srv := r.Server(base, DefaultServerTimeouts())
	require.NotNil(t, srv.BaseContext, "thiếu BaseContext thì request không bị huỷ khi dừng container")

	got := srv.BaseContext(nil)
	require.NoError(t, got.Err(), "context phải còn sống lúc khởi tạo")

	cancel()
	require.ErrorIs(t, got.Err(), context.Canceled,
		"BaseContext phải bị huỷ theo context của process, nếu không thì lệnh DB/audio của request đang chạy không dừng")
}
