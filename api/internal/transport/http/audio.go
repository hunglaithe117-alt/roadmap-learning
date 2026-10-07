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

const (
	maxTTSTextLen = 500
	maxSTTBytes   = 10 << 20
	ttsTimeout    = 30 * time.Second
	sttTimeout    = 60 * time.Second
)

// audioFileField is the multipart form field name for uploaded audio files.
const audioFileField = "audio"

// ttsHandler synthesizes speech from query params and streams audio/wav.
func ttsHandler(engine audiodomain.TTSSynthesizer, log *slog.Logger) gin.HandlerFunc {
	if engine == nil {

		return func(c *gin.Context) {
			writeJSONError(c, http.StatusServiceUnavailable, "chưa cấu hình engine TTS")
		}
	}
	langEngine, _ := engine.(audiodomain.LangSynthesizer)
	return func(c *gin.Context) {
		info := engine.Info()
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
			writeEngineError(ctx, c, log, info, "tts", err)
			return
		}
		if contentType == "" {
			contentType = "audio/wav"
		}
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.Itoa(len(audio)))
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, contentType, audio)
	}
}

// sttHandler transcribes uploaded audio files and returns a JSON transcript.
func sttHandler(engine audiodomain.STTTranscriber, log *slog.Logger) gin.HandlerFunc {
	if engine == nil {
		return func(c *gin.Context) {
			writeJSONError(c, http.StatusServiceUnavailable, "chưa cấu hình engine STT")
		}
	}
	detailed, _ := engine.(audiodomain.DetailedTranscriber)
	return func(c *gin.Context) {
		info := engine.Info()
		c.Header(audiodomain.Header, info.Name)

		if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/") {
			writeJSONError(c, http.StatusBadRequest, "thiếu file audio (multipart)")
			return
		}
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
			writeEngineError(ctx, c, log, info, "stt", err)
			return
		}
		c.JSON(http.StatusOK, newSTTResponse(res, info))
	}
}

// sttResponse represents the JSON response for speech transcription.
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

// writeEngineError logs engine errors and returns a client error response.
func writeEngineError(ctx context.Context, c *gin.Context, log *slog.Logger, info audiodomain.EngineInfo, kind string, err error) {
	log.Error(kind+" engine failed",
		slog.String("engine", info.Name), slog.String("err", err.Error()))
	switch {
	case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		writeJSONError(c, 499, kind+" bị hủy")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		writeJSONError(c, http.StatusGatewayTimeout, kind+" timeout")
	default:
		writeJSONError(c, http.StatusBadGateway, kind+" thất bại")
	}
}

