package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	audiodomain "langapp/internal/domain/audio"
)

func init() { gin.SetMode(gin.TestMode) }

// testRouter dựng router với engine stub, không đụng DB.
func testRouter(webDist string) *gin.Engine {
	r := NewRouter(Options{
		GinMode: "test",
		WebDist: webDist,
		TTS:     StubTTS{},
		STT:     StubSTT{},
		Log:     discardLogger(),
	})
	return r.Engine
}

// StubTTS sinh WAV tối thiểu — chỉ cần bytes có "RIFF" để handler trả về và
// test khẳng định `Content-Type`/`X-Engine`, không cần âm thanh thật.
type StubTTS struct{}

// Info trả metadata của engine TTS stub.
func (StubTTS) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindTTS, false)
}

// Synthesize trả bytes giả có tiền tố "RIFF" cùng tên text.
func (StubTTS) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	return ttsBytes(text), "audio/wav", nil
}

// SynthesizeLang trả bytes giả có gắn thêm `lang` để test kiểm việc truyền lang.
func (StubTTS) SynthesizeLang(ctx context.Context, text, lang string) ([]byte, string, error) {
	return ttsBytes(text + "|" + lang), "audio/wav", nil
}

func ttsBytes(text string) []byte {
	return append([]byte("RIFF____WAVEfake:"+text), 0)
}

// StubSTT là engine STT stub trả transcript cố định cho test HTTP.
type StubSTT struct{}

// Info trả metadata của engine STT stub.
func (StubSTT) Info() audiodomain.EngineInfo {
	return audiodomain.NewEngineInfo("stub", audiodomain.KindSTT, false)
}

// Transcribe trả transcript cố định "ni hao" / "zh".
func (StubSTT) Transcribe(ctx context.Context, audio []byte, filename, contentType string) (audiodomain.Transcript, error) {
	return audiodomain.Transcript{Text: "ni hao", Lang: "zh"}, nil
}

// ── /api/tts ────────────────────────────────────────────────────────────────

// Test tts trả Content-Type audio/wav và header X-Engine nói đúng engine.
//
// `X-Engine` là hợp đồng với UI: `domain/audio.EngineInfo.Real=false` ⇒ client
// hiện badge "kết quả không thật". Thiếu header này thì UI hiển thị audio của
// stub như audio thật — kiểu lỗi im lặng mà test header là chặn được.
func Test_tts_returns_wav_with_engine_header(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tts?text=ni+hao&lang=zh", nil)
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "audio/wav", w.Header().Get("Content-Type"))
	require.Equal(t, "stub", w.Header().Get(audiodomain.Header))
	require.Contains(t, w.Body.String(), "RIFF")
	// `lang` phải tới được engine (qua `LangSynthesizer`) chứ không bị bỏ.
	require.Contains(t, w.Body.String(), "|zh")
}

func Test_tts_rejects_missing_text(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tts", nil))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":"thiếu text"}`, w.Body.String())
}

func Test_tts_rejects_text_over_500_runes(t *testing.T) {
	// 501 RUNE (không phải byte) — tiếng Việt có dấu nên 501 ký tự là 501×2+
	// byte. Handler đếm rune; nếu đếm byte thì 300 chữ "á" cũng bị chặn oan.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tts?text="+url.QueryEscape(strings.Repeat("á", 501)), nil)
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "quá dài")
}

func Test_tts_accepts_text_of_exactly_500_runes(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tts?text="+url.QueryEscape(strings.Repeat("á", 500)), nil)
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// ── /api/stt ────────────────────────────────────────────────────────────────

func Test_stt_accepts_multipart_upload(t *testing.T) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile(audioFileField, "rec.wav")
	require.NoError(t, err)
	_, err = part.Write([]byte("RIFFfake-audio"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/stt?lang=zh", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got sttResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "ni hao", got.Transcript)
	require.Equal(t, "zh", got.Lang)
	require.Equal(t, "stub", got.Engine)
}

func Test_stt_rejects_non_multipart_body(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/stt", strings.NewReader("raw"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func Test_stt_rejects_empty_file(t *testing.T) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile(audioFileField, "empty.wav")
	require.NoError(t, err)
	_, err = part.Write(nil)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/stt", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "rỗng")
}

// ── CORS ────────────────────────────────────────────────────────────────────

// Test CORS preflight trả 204 + header allow. Gin KHÔNG tự làm preflight —
// không có middleware này thì mọi `POST /query` từ Vite dev (port khác origin)
// bị browser chặn trước khi tới server.
func Test_cors_preflight_returns_204_with_allow_origin(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/tts", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	// Router mặc định `Dev=false` nên KHÔNG allow origin nào; bật Dev để test
	// đúng nhánh allow-list.
	devRouter := NewRouter(Options{
		GinMode: "test", Dev: true, TTS: StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine
	devRouter.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "Origin", w.Header().Get("Vary"))
	require.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
}

// Origin KHÔNG nằm trong allow-list thì KHÔNG được set header CORS. Set `*` sẽ
// biến thành "mọi trang web đều gọi được API" — đúng thứ cần tránh.
func Test_cors_denies_unknown_origin_without_allow_header(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/tts", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, req)

	require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// ── static SPA ──────────────────────────────────────────────────────────────

// spaMarker là chuỗi đặc trưng của `index.html` trong test — dùng marker thay vì
// so nguyên file để test không vỡ khi ai đó thêm `<script>` vào template.
const spaMarker = "LANGAPP-SPA-ROOT"

func writeWebDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"),
		[]byte("<!doctype html><div id=app>"+spaMarker+"</div>"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "index.js"), []byte("console.log(1)"), 0o600))
	return dir
}

// Test static `/` trả 200 với index.html.
func Test_static_root_returns_index_html(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(writeWebDist(t)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), spaMarker)
}

// Test route lạ của SPA phải rơi về index.html chứ không 404.
//
// `createWebHashHistory` khiến client route nằm sau `#` nên server không thấy —
// nhưng nếu ai đó đổi sang `createWebHistory`, hoặc user gõ trực tiếp
// `/roadmap/zh`, thì 404 sẽ làm SPA không mở được.
func Test_static_spa_route_falls_back_to_index_html(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(writeWebDist(t)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), spaMarker)
}

// File tĩnh có thật thì phải trả FILE ĐÓ chứ không phải index.html — nếu không,
// mọi asset JS/CSS đều nhận HTML ⇒ trang trắng.
func Test_static_serves_real_asset_instead_of_index(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(writeWebDist(t)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/index.js", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "console.log(1)", w.Body.String())
}

// Không cấu hình WEB_DIST thì đường dẫn lạ trả 404 chứ không trả HTML rỗng —
// để phân biệt "chưa build web" với "app hỏng".
func Test_static_404_when_web_dist_not_configured(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter("").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/roadmap/zh", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "không tìm thấy")
}

// ── `/api/*` lạ KHÔNG được rơi vào fallback SPA ──────────────────────────────
//
// Đường dẫn lạ ngoài `/api` vẫn trả `index.html` (client-side routing). Nhưng
// `/api/*` là namespace API: trả 200 + HTML là client tưởng thành công rồi hỏng
// lúc parse, lỗi hiện ra muộn và sai chỗ. Nhánh `WebDist == ""` đã trả 404 JSON;
// test này khoá nhánh có SPA cho cùng hành vi.
//
// Cũng khoá việc `/api/backup` + `/api/restore` đã bị gỡ THẬT: trước khi có
// guard này, chúng rơi vào `staticSPA` và trả **200 + index.html**, nên "đã xoá"
// không kiểm chứng được bằng HTTP. Route 501 cũ cũng vậy.
func Test_api_unknown_path_404_json_not_spa(t *testing.T) {
	for _, path := range []string{"/api/backup", "/api/restore", "/api/khong-ton-tai", "/api"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			testRouter(writeWebDist(t)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

			require.Equal(t, http.StatusNotFound, w.Code, "%s phải 404", path)
			require.Contains(t, w.Result().Header.Get("Content-Type"), "application/json", "%s phải trả JSON", path)
			require.NotContains(t, w.Body.String(), "<!doctype html>", "%s không được rơi vào index.html", path)
		})
	}
}

// Nhưng đường dẫn lạ NGOÀI `/api` vẫn phải ra SPA — nếu không, deep-link kiểu
// `/roadmap/<slug>` gõ tay sẽ chết.
func Test_non_api_unknown_path_still_serves_spa(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(writeWebDist(t)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/roadmap/khong-ton-tai", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "<!doctype html>")
}

// ── server timeout ──────────────────────────────────────────────────────────

// gosec G112: `http.Server` không set timeout ⇒ connection treo vô hạn.
func Test_server_sets_all_four_timeouts(t *testing.T) {
	r := NewRouter(Options{GinMode: "test", TTS: StubTTS{}, STT: StubSTT{}, Log: discardLogger()})
	srv := r.Server(context.Background(), DefaultServerTimeouts())

	require.NotZero(t, srv.ReadHeaderTimeout, "ReadHeaderTimeout = 0 sẽ dính gosec G112 và Slowloris")
	require.NotZero(t, srv.ReadTimeout, "ReadTimeout = 0 sẽ dính gosec G112")
	require.NotZero(t, srv.WriteTimeout, "WriteTimeout = 0 sẽ dính gosec G112")
	require.NotZero(t, srv.IdleTimeout, "IdleTimeout = 0 sẽ dính gosec G112")
}

// `SetTrustedProxies(nil)` — mặc định của Gin là `0.0.0.0/0`, tức TIN mọi
// `X-Forwarded-For` là của client. Test assert qua HÀNH VI (`ClientIP`) chứ không
// qua field: `Engine.TrustedPlatform` là chuỗi rỗng ở cả 2 cấu hình nên assert
// nó không bắt được gì.
func Test_router_does_not_trust_forwarded_for_from_any_proxy(t *testing.T) {
	e := testRouter("")
	e.GET("/__clientip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/__clientip", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	require.Equal(t, "10.1.2.3", w.Body.String(),
		"ClientIP lấy từ X-Forwarded-For ⇒ đang TIN mọi proxy, kể cả mạng ngoài")
}

// ── /api/health ─────────────────────────────────────────────────────────────

type fakePinger struct{ err error }

func (f fakePinger) PingContext(ctx context.Context) error { return f.err }

// Healthcheck của Dockerfile grep đúng `{"status":"ok"}`; đổi shape là
// HEALTHCHECK đỏ trong khi app vẫn chạy.
func Test_health_returns_ok_when_db_reachable(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test",
		DB:      fakePinger{},
		TTS:     StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"ok"`)
}

// Audio stub vẫn 200 (app DÙNG ĐƯỢC) nhưng báo `degraded` — trả 503 sẽ khiến
// Docker restart app vô ích vì thiếu 1 tuỳ chọn.
func Test_health_reports_degraded_but_200_when_audio_is_stub(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{},
		Audio: func() (string, bool) { return "stub", false },
		TTS:   StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"degraded"`)
}

func Test_health_returns_503_when_db_unreachable(t *testing.T) {
	e := NewRouter(Options{
		GinMode: "test", DB: fakePinger{err: context.DeadlineExceeded},
		TTS: StubTTS{}, STT: StubSTT{},
		Log: discardLogger(),
	}).Engine

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ── endpoint count (chống thêm endpoint JSON lọt vào REST) ──────────────────

// Danh sách route đã đăng ký dưới `/api`. Test này chặn việc sau này thêm 1
// endpoint JSON vào REST vì "tiện tay" — làm client quay lại gọi tuần tự và
// phá vỡ đúng lý do STACK-V2 chuyển sang GraphQL.
//
// Nó cũng khoá việc ĐĂNG LẠI `/api/backup` + `/api/restore`: 2 endpoint ấy đã
// bị gỡ vì `BackupPort` chưa bao giờ có hiện thực (chỉ trả 501). Thêm lại vào
// danh sách `want` bên dưới ⇒ test này đỏ, đúng ý.
func Test_api_group_registers_exactly_the_three_binary_endpoints_plus_health(t *testing.T) {
	e := testRouter("")
	got := map[string]bool{}
	for _, r := range e.Routes() {
		got[r.Method+" "+r.Path] = true
	}
	want := []string{
		"GET /api/health",
		"GET /api/tts",
		"POST /api/stt",
	}
	for _, w := range want {
		t.Run(w, func(t *testing.T) {
			require.True(t, got[w], "thiếu route %s", w)
		})
	}
	for r := range got {
		t.Run(r, func(t *testing.T) {
			require.Contains(t, want, r, "route %s không nằm trong danh sách 3 nhị phân + health", r)
		})
	}
}
