package httptransport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	audiodomain "langapp/internal/domain/audio"
)

// Hằng giữ nguyên hành vi v1 (api/audio.go):
//   - `maxTTSTextLen` 500 ký tự — chặn 1 câu tổng hợp quá dài.
//   - `maxSTTBytes` 10MB — trần upload.
//   - `ttsTimeout` 30s, `sttTimeout` 60s.
//
// Timeout đặt ở ĐÂY (transport) chứ không chỉ ở client gRPC: handler là nơi duy
// nhất biết đây là request web, nên request web có deadline. Use case
// `practice` gọi cùng engine qua port thì có deadline riác của nó (xem
// `audioinfra.WithTTSTimeout`).
const (
	maxTTSTextLen = 500
	maxSTTBytes   = 10 << 20
	ttsTimeout    = 30 * time.Second
	sttTimeout    = 60 * time.Second
)

// audioFileField là tên field multipart mà client gửi file lên. Giữ "audio"
// như v1 — client React hiện tại đang gửi đúng tên này, đổi tên là phá UI cũ
// trước khi M5 kịp port.
const audioFileField = "audio"

// ttsHandler `GET /api/tts?text=…&lang=zh|en` → stream `audio/wav`.
//
// Gọi qua `audiodomain.TTSSynthesizer`, KHÔNG gọi `os/exec` hay HTTP client từ
// handler: 2 điều đó là việc của `services/audio-service` và
// `internal/infrastructure/audio` (STACK-V2-PLAN §2).
func ttsHandler(engine audiodomain.TTSSynthesizer, log *slog.Logger) gin.HandlerFunc {
	if engine == nil {
		return func(c *gin.Context) {
			writeJSONError(c, http.StatusServiceUnavailable, "chưa cấu hình engine TTS")
		}
	}
	// `LangSynthesizer` là phần mở rộng: engine không chọn được voice thì bỏ
	// qua `lang` và dùng voice mặc định, KHÔNG phải lỗi (giữ hành vi v1).
	langEngine, _ := engine.(audiodomain.LangSynthesizer)
	return func(c *gin.Context) {
		// Header `X-Engine` đọc TỨC THÌ, KHÔNG bắt 1 bản ở ngoài closure.
		// `engine.Info()` của adapter gRPC đổi sau request đầu tiên (service
		// báo tên engine thật qua `SynthesizeResponse.Real`), nên bản chụp lúc
		// khởi tạo sẽ ghim header ở "audio-service" mãi (F3).
		info := engine.Info()
		// Header luôn có, kể cả khi lỗi: client cần biết engine nào đã xử lý
		// để hiện badge "kết quả không thật" khi là stub.
		c.Header(audiodomain.Header, info.Name)

		text := strings.TrimSpace(c.Query("text"))
		if text == "" {
			writeJSONError(c, http.StatusBadRequest, "thiếu text")
			return
		}
		if len([]rune(text)) > maxTTSTextLen {
			writeJSONError(c, http.StatusBadRequest, "text quá dài (tối đa 500 ký tự)")
			return
		}
		lang := strings.ToLower(strings.TrimSpace(c.Query("lang")))

		ctx, cancel := context.WithTimeout(c.Request.Context(), ttsTimeout)
		defer cancel()

		var (
			audio       []byte
			contentType string
			err         error
		)
		if langEngine != nil {
			audio, contentType, err = langEngine.SynthesizeLang(ctx, text, lang)
		} else {
			audio, contentType, err = engine.Synthesize(ctx, text)
		}
		if err != nil {
			writeEngineError(c, ctx, log, info, "tts", err)
			return
		}
		if contentType == "" {
			contentType = "audio/wav"
		}
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.Itoa(len(audio)))
		// `no-store`: audio đổi theo bản Piper/voice model đang chạy, cache
		// trình duyệt sẽ phát nhầm phiên bản cũ sau khi đổi voice.
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, contentType, audio)
	}
}

// sttHandler `POST /api/stt` multipart field `audio` → JSON transcript.
func sttHandler(engine audiodomain.STTTranscriber, log *slog.Logger) gin.HandlerFunc {
	if engine == nil {
		return func(c *gin.Context) {
			writeJSONError(c, http.StatusServiceUnavailable, "chưa cấu hình engine STT")
		}
	}
	// `DetailedTranscriber` là phần mở rộng: engine không có word-level
	// timestamp thì trả Transcript rỗng phần Words, không phải lỗi (hành vi v1).
	detailed, _ := engine.(audiodomain.DetailedTranscriber)
	return func(c *gin.Context) {
		info := engine.Info()
		c.Header(audiodomain.Header, info.Name)

		if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/") {
			writeJSONError(c, http.StatusBadRequest, "thiếu file audio (multipart)")
			return
		}
		// `MaxBytesReader` chặn TRƯỚC khi parse: `ParseMultipartForm` với
		// `maxSTTBytes` vẫn đọc hết body vào `maxMemory` rồi mới ghi tạm, nên
		// không có reader chặn thì 1 upload 2GB sẽ đi hết vào đĩa tạm.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSTTBytes+1024)
		if err := c.Request.ParseMultipartForm(maxSTTBytes); err != nil {
			writeJSONError(c, http.StatusBadRequest, "multipart không hợp lệ: "+err.Error())
			return
		}
		file, header, err := c.Request.FormFile(audioFileField)
		if err != nil {
			writeJSONError(c, http.StatusBadRequest, "thiếu file audio (field \""+audioFileField+"\")")
			return
		}
		defer file.Close()
		audio, err := io.ReadAll(io.LimitReader(file, maxSTTBytes+1))
		if err != nil {
			writeJSONError(c, http.StatusBadRequest, "không đọc được file audio")
			return
		}
		if len(audio) == 0 {
			writeJSONError(c, http.StatusBadRequest, "file audio rỗng")
			return
		}
		if len(audio) > maxSTTBytes {
			writeJSONError(c, http.StatusBadRequest, "audio quá lớn (tối đa 10MB)")
			return
		}

		var (
			filename    string
			contentType string
		)
		if header != nil {
			filename = header.Filename
			contentType = header.Header.Get("Content-Type")
		}
		// Gợi ý ngôn ngữ: query trước, form field sau (client gửi 1 trong 2).
		langHint := strings.ToLower(strings.TrimSpace(c.Query("lang")))
		if langHint == "" {
			langHint = strings.ToLower(strings.TrimSpace(c.Request.FormValue("lang")))
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), sttTimeout)
		defer cancel()

		var res audiodomain.Transcript
		if detailed != nil {
			res, err = detailed.TranscribeDetailed(ctx, audio, filename, contentType, langHint)
		} else {
			res, err = engine.Transcribe(ctx, audio, filename, contentType)
		}
		if err != nil {
			writeEngineError(c, ctx, log, info, "stt", err)
			return
		}
		c.JSON(http.StatusOK, newSTTResponse(res, info))
	}
}

// sttResponse là JSON trả về, giữ nguyên 5 field của `api/audio.go` v1
// (`transcript`, `lang`, `engine`, `words`, `confidence`) để client cũ đọc được
// trước khi M5 port sang Vue.
type sttResponse struct {
	Transcript string                      `json:"transcript"`
	Lang       string                      `json:"lang"`
	Engine     string                      `json:"engine"`
	Words      []audiodomain.WordTimestamp `json:"words,omitempty"`
	Confidence *float64                    `json:"confidence,omitempty"`
}

func newSTTResponse(t audiodomain.Transcript, info audiodomain.EngineInfo) sttResponse {
	return sttResponse{
		Transcript: t.Text,
		Lang:       t.Lang,
		Engine:     info.Name,
		Words:      t.Words,
		Confidence: t.Confidence,
	}
}

// writeEngineError log chi tiết server-side nhưng chỉ gửi message ngắn cho
// client. `X-Engine` luôn được set (đã set ở đầu handler) để UI biết engine nào
// hỏng.
func writeEngineError(c *gin.Context, ctx context.Context, log *slog.Logger, info audiodomain.EngineInfo, kind string, err error) {
	log.Error(kind+" engine thất bại",
		slog.String("engine", info.Name), slog.String("err", err.Error()))
	switch {
	case errors.Is(err, context.Canceled), ctx.Err() == context.Canceled:
		writeJSONError(c, 499, kind+" bị hủy")
	case errors.Is(err, context.DeadlineExceeded), ctx.Err() == context.DeadlineExceeded:
		writeJSONError(c, http.StatusGatewayTimeout, kind+" timeout")
	default:
		writeJSONError(c, http.StatusBadGateway, kind+" thất bại")
	}
}
