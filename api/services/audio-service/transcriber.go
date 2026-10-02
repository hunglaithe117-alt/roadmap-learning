package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxAudioBytes là trần 1 request STT — khớp `maxSTTBytes` 10MB của v1.
	maxAudioBytes = 10 << 20
	// sttTimeout là trần cho 1 lần gọi Whisper. Whisper small CPU int8 mất
	// vài chục giây cho audio dài; 60s là trần của v1.
	sttTimeout = 60 * time.Second
)

// transcript là kết quả STT đã chuẩn hoá phồn→giản, khớp `STTResult` v1.
type transcript struct {
	Text       string
	Lang       string
	Words      []wordStamp
	Confidence *float64
}

type wordStamp struct {
	Word       string
	Start      *float64
	End        *float64
	Confidence *float64
}

// transcriber là đằng sau `Audio.Transcribe`. Cùng lý do tách interface như
// `synthesizer`: tiến trình service không import tầng trong của app.
type transcriber interface {
	Transcribe(ctx context.Context, audio []byte, filename, contentType, langHint string) (transcript, error)
	Name() string
	Real() bool
}

// newTranscriberFromEnv trả stub khi WHISPER_URL rỗng. Stub tự báo `real=false`
// để UI hiện badge "kết quả không thật" thay vì im lặng — hành vi này đã có ở
// `domain/audio.EngineInfo.Real`.
func newTranscriberFromEnv(log *slog.Logger) (transcriber, error) {
	if u := strings.TrimSpace(envOr("WHISPER_URL", "")); u != "" {
		return &whisperTranscriber{
			url:    u,
			client: &http.Client{Timeout: sttTimeout},
		}, nil
	}
	log.Info("WHISPER_URL rỗng, STT dùng stub transcript")
	return stubTranscriber{}, nil
}

// whisperTranscriber POST multipart (field "audio_file") tới image
// onerahmet/openai-whisper-asr-webservice. Port nguyên văn
// `FasterWhisperEngine.TranscribeDetailed` ở api/audio.go v1 — giữ nguyên
// query param (language/vad_filter/beam_size/output) và cả 2 hình dạng payload
// (`text`/`transcript`, `words`/`segments[].words`) vì sidecar đã trả cả hai
// tuỳ phiên bản.
type whisperTranscriber struct {
	url    string
	client *http.Client
}

func (w *whisperTranscriber) Name() string { return "faster-whisper" }
func (w *whisperTranscriber) Real() bool   { return true }

func (w *whisperTranscriber) Transcribe(ctx context.Context, audio []byte, filename, contentType, langHint string) (transcript, error) {
	endpoint, err := url.Parse(w.url)
	if err != nil {
		return transcript{}, fmt.Errorf("WHISPER_URL sai cú pháp: %v", err)
	}
	q := endpoint.Query()
	if l := whisperLang(langHint); l != "" {
		q.Set("language", l)
	}
	q.Set("vad_filter", "true")
	q.Set("beam_size", "1")
	if q.Get("output") == "" {
		q.Set("output", "json")
	}
	endpoint.RawQuery = q.Encode()

	if filename == "" {
		filename = "audio.wav"
	}
	// Pipe thay vì buffer: audio tới 10MB, dựng cả multipart trong RAM rồi
	// copy thêm 1 lần là tốn 20MB cho 1 request.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	formType := mw.FormDataContentType()
	wch := make(chan error, 1)
	go func() {
		var werr error
		defer func() {
			if werr != nil {
				pw.CloseWithError(werr)
			} else {
				pw.Close()
			}
			wch <- werr
		}()
		part, werr2 := mw.CreateFormFile("audio_file", filename)
		if werr2 != nil {
			werr = werr2
			return
		}
		if _, werr = part.Write(audio); werr != nil {
			return
		}
		werr = mw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), pr)
	if err != nil {
		pr.Close()
		<-wch
		return transcript{}, err
	}
	req.Header.Set("Content-Type", formType)
	resp, err := w.client.Do(req)
	if err != nil {
		pr.Close()
		<-wch
		return transcript{}, err
	}
	defer resp.Body.Close()
	if werr := <-wch; werr != nil {
		return transcript{}, werr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return transcript{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return transcript{}, fmt.Errorf("whisper %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeWhisper(body, langHint)
}

func decodeWhisper(body []byte, langHint string) (transcript, error) {
	var out struct {
		Text       string `json:"text"`
		Transcript string `json:"transcript"`
		Language   string `json:"language"`
		Lang       string `json:"lang"`
		Segments   []struct {
			Text  string `json:"text"`
			Words []struct {
				Word  string   `json:"word"`
				Text  string   `json:"text"`
				Start *float64 `json:"start"`
				End   *float64 `json:"end"`
			} `json:"words"`
		} `json:"segments"`
		Words []struct {
			Word       string   `json:"word"`
			Start      *float64 `json:"start"`
			End        *float64 `json:"end"`
			Confidence *float64 `json:"confidence"`
		} `json:"words"`
		Confidence *float64 `json:"confidence"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return transcript{}, fmt.Errorf("whisper trả JSON sai cú pháp: %v", err)
	}

	text := toSimplified(out.Text)
	if text == "" {
		text = toSimplified(out.Transcript)
	}
	if text == "" {
		parts := make([]string, 0, len(out.Segments))
		for _, s := range out.Segments {
			// Chuẩn hoá từng segment TRƯỚC join: giữ nguyên biên từ cho
			// word timing (giữ chú thích E1 ở api/audio.go v1).
			if t := strings.TrimSpace(toSimplified(s.Text)); t != "" {
				parts = append(parts, t)
			}
		}
		text = strings.Join(parts, " ")
	}

	lang := out.Language
	if lang == "" {
		lang = out.Lang
	}
	if lang == "" {
		lang = whisperLang(langHint)
	}

	var words []wordStamp
	for _, w := range out.Words {
		word := toSimplified(w.Word)
		if word == "" {
			continue
		}
		words = append(words, wordStamp{Word: word, Start: w.Start, End: w.End, Confidence: w.Confidence})
	}
	if words == nil {
		for _, s := range out.Segments {
			for _, sw := range s.Words {
				word := sw.Word
				if word == "" {
					word = sw.Text
				}
				if word == "" {
					continue
				}
				words = append(words, wordStamp{Word: toSimplified(word), Start: sw.Start, End: sw.End})
			}
		}
	}
	return transcript{Text: strings.TrimSpace(text), Lang: lang, Words: words, Confidence: out.Confidence}, nil
}

// whisperLang map gợi ý drill sang tham số `language` của sidecar. Gợi ý rỗng
// hoặc lạ = để Whisper tự nhận dạng (bỏ hẳn tham số).
func whisperLang(hint string) string {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "zh", "zh-cn", "zh_cn", "cn", "chinese":
		return "zh"
	case "en", "en-us", "en_us", "english":
		return "en"
	default:
		return ""
	}
}

// stubTranscriber trả transcript cố định — port `StubSTTEngine` v1 (default
// "ni hao" / "zh").
type stubTranscriber struct{}

func (stubTranscriber) Name() string { return "stub" }
func (stubTranscriber) Real() bool   { return false }

func (stubTranscriber) Transcribe(ctx context.Context, audio []byte, filename, contentType, langHint string) (transcript, error) {
	if err := ctx.Err(); err != nil {
		return transcript{}, err
	}
	if len(audio) == 0 {
		return transcript{}, fmt.Errorf("empty audio")
	}
	return transcript{Text: "ni hao", Lang: "zh"}, nil
}
