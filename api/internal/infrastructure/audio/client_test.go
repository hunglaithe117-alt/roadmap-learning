package audioinfra_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	audiodomain "langapp/internal/domain/audio"
	audioinfra "langapp/internal/infrastructure/audio"
	audiopb "langapp/proto/audio/v1"
)

// fakeServer là hiện thực `audiopb.AudioServer` cho test, chạy trên `bufconn`.
//
// Vì sao bufconn chứ không spin process thật: `services/audio-service` cần
// Piper (wheel 752MB) hoặc Whisper (image 8GB). Test transport chỉ cần chứng
// minh adapter chuyển đúng protobuf ↔ value object, không cần engine thật — và
// `go test ./...` phải chạy được trên máy không có image nào.
type fakeServer struct {
	audiopb.UnimplementedAudioServer

	engine  string
	real    bool
	words   []*audiopb.WordTimestamp
	audio   []byte
	lastReq *audiopb.TranscribeRequest
	// err để test nhánh lỗi: `status.Error(codes.DeadlineExceeded, …)`.
	err error
}

func (f *fakeServer) Synthesize(_ context.Context, req *audiopb.SynthesizeRequest) (*audiopb.SynthesizeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &audiopb.SynthesizeResponse{
		Audio:       f.audio,
		ContentType: "audio/wav",
		Engine:      f.engine,
		Real:        f.real,
	}, nil
}

func (f *fakeServer) Transcribe(_ context.Context, req *audiopb.TranscribeRequest) (*audiopb.TranscribeResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return &audiopb.TranscribeResponse{
		Text:   "ni hao",
		Lang:   "zh",
		Words:  f.words,
		Engine: f.engine,
		Real:   f.real,
	}, nil
}

// serveBufconn dựng gRPC server trên listener trong bộ nhớ, trả client conn.
func serveBufconn(t *testing.T, srv audiopb.AudioServer) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	audiopb.RegisterAudioServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Test adapter TTS chuyển đúng bytes + contentType + engine.
func Test_synthesizer_returns_audio_and_records_engine(t *testing.T) {
	fake := &fakeServer{engine: "piper", real: true, audio: []byte("RIFF-wav-bytes")}
	conn := serveBufconn(t, fake)
	s := audioinfra.NewSynthesizer(conn)

	audio, contentType, err := s.SynthesizeLang(context.Background(), "ni hao", "zh")
	require.NoError(t, err)
	require.Equal(t, []byte("RIFF-wav-bytes"), audio)
	require.Equal(t, "audio/wav", contentType)

	// `Info()` phải phản ánh engine service VỪA báo, để header `X-Engine` nói
	// đúng sau request đầu tiên.
	require.Equal(t, "piper", s.Info().Name)
	require.True(t, s.Info().Real, "Real phải theo service, không phải mặc định false")
}

// Trước request nào, `Info()` phải nói "audio-service" + `Real=false` — UI hiện
// badge "kết quả không thật" thay vì im lặng.
func Test_synthesizer_info_before_first_call_is_not_real(t *testing.T) {
	s := audioinfra.NewSynthesizer(serveBufconn(t, &fakeServer{engine: "piper", real: true}))
	info := s.Info()

	require.Equal(t, "audio-service", info.Name)
	require.False(t, info.Real, "chưa gọi engine thì không được khẳng định là thật")
}

// Adapter phải thoả 2 interface của domain — đây là hợp đồng mà handler REST và
// use case `practice` bind vào. Nếu đổi chữ ký mà quên 1 interface, test này đỏ
// ngay lúc biên dịch.
var (
	_ audiodomain.TTSSynthesizer      = (*audioinfra.Synthesizer)(nil)
	_ audiodomain.LangSynthesizer     = (*audioinfra.Synthesizer)(nil)
	_ audiodomain.STTTranscriber      = (*audioinfra.Transcriber)(nil)
	_ audiodomain.DetailedTranscriber = (*audioinfra.Transcriber)(nil)
)

func Test_transcriber_converts_words_and_optional_fields(t *testing.T) {
	start, end := 0.1, 0.5
	fake := &fakeServer{
		engine: "faster-whisper", real: true,
		words: []*audiopb.WordTimestamp{
			{Word: "ni", Start: &start, End: &end},
			{Word: "hao"}, // không có mốc thời gian — service có thể trả vậy
		},
	}
	tr := audioinfra.NewTranscriber(serveBufconn(t, fake))

	res, err := tr.TranscribeDetailed(context.Background(), []byte("audio"), "rec.wav", "audio/wav", "zh")
	require.NoError(t, err)

	require.Equal(t, "ni hao", res.Text)
	require.Equal(t, "zh", res.Lang)
	require.Len(t, res.Words, 2)
	require.InDelta(t, 0.1, *res.Words[0].Start, 0.0001)
	require.Nil(t, res.Words[1].Start, "mốc vắng mặt phải giữ nil, không ép thành 0")
}

// Tên file + content type phải tới được service — engine cần chúng để đoán định
// dạng (wav vs m4a vs webm).
func Test_transcriber_forwards_filename_and_content_type(t *testing.T) {
	fake := &fakeServer{engine: "stub", real: false}
	tr := audioinfra.NewTranscriber(serveBufconn(t, fake))

	_, err := tr.TranscribeDetailed(context.Background(), []byte("a"), "rec.m4a", "audio/mp4", "en")
	require.NoError(t, err)

	require.Equal(t, "rec.m4a", fake.lastReq.GetFilename())
	require.Equal(t, "audio/mp4", fake.lastReq.GetContentType())
	require.Equal(t, "en", fake.lastReq.GetLang())
}

// Lỗi từ service phải giữ NGUYÊN mã gRPC để handler map được 502/504 — nuốt
// lỗi ở đây thì mọi lỗi engine đều ra 500.
//
// So bằng `status.Code` chứ không phải `errors.Is(err, context.DeadlineExceeded)`:
// lỗi đi qua gRPC thành `status.Error(codes.DeadlineExceeded, …)`, KHÔNG phải
// sentinel của context — `errors.Is` sẽ luôn false dù lỗi đúng.
func Test_transcriber_propagates_grpc_status_code(t *testing.T) {
	tr := audioinfra.NewTranscriber(serveBufconn(t, &fakeServer{
		err: status.Error(codes.DeadlineExceeded, "quá hạn"),
	}))
	_, err := tr.Transcribe(context.Background(), []byte("a"), "a.wav", "audio/wav")
	require.Error(t, err)
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
}

// ── Engine / stub fallback ──────────────────────────────────────────────────

// `AUDIO_GRPC_ADDR` rỗng ⇒ STUB cho cả 2 chiều. Nhờ vậy `go test ./...` và
// `docker compose up` với Postgres-only đều chạy được mà không cần
// audio-service (image Whisper 8GB).
func Test_new_from_env_without_addr_returns_stub_for_both_directions(t *testing.T) {
	e, err := audioinfra.NewFromEnv("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close() })

	require.False(t, e.IsReal(), "stub phải báo Real=false")
	require.Equal(t, "stub", e.EngineInfo().Name)
	require.Equal(t, audiodomain.KindTTS, e.TTSSynth.Info().Kind)
	require.Equal(t, audiodomain.KindSTT, e.STTSTT.Info().Kind)
}

// Stub phải trả WAV thật để `<audio>` của trình duyệt phát được offline, và phải
// trùng hẳn với hành vi v1 (16000Hz / 0.5s / 440Hz) — đổi thì mọi máy chưa bật
// audio-service đổi hành vi âm thanh.
func Test_stub_synthesizer_emits_riff_wav_with_v1_parameters(t *testing.T) {
	audio, contentType, err := audioinfra.StubSynthesizer{}.Synthesize(context.Background(), "ni hao")
	require.NoError(t, err)
	require.Equal(t, "audio/wav", contentType)
	require.Equal(t, "RIFF", string(audio[0:4]))
	require.Equal(t, "WAVE", string(audio[8:12]))

	// Offset trong header WAV PCM (xem `sineWAV`): [20:22] audioFormat,
	// [22:24] numChannels, [24:28] sampleRate, [28:32] byteRate,
	// [32:34] blockAlign, [34:36] bitsPerSample, [40:44] kích thước data.
	require.Equal(t, uint16(1), le16(audio, 20), "PCM")
	require.Equal(t, uint16(1), le16(audio, 22), "mono")
	require.Equal(t, uint32(16000), le32(audio, 24))
	require.Equal(t, uint32(32000), le32(audio, 28), "byteRate = 16000Hz × 1 kênh × 2 byte")
	require.Equal(t, uint16(16), le16(audio, 34), "16 bit/sample")
	require.Equal(t, 0.5, audioDurationSeconds(audio), "stub v1 dài 0.5s")
}

func audioDurationSeconds(wav []byte) float64 {
	byteRate := float64(le32(wav, 28))
	dataLen := float64(le32(wav, 40))
	return dataLen / byteRate
}

func le32(b []byte, off int) uint32 {
	return uint32(b[off]) | uint32(b[off+1])<<8 | uint32(b[off+2])<<16 | uint32(b[off+3])<<24
}

func le16(b []byte, off int) uint16 { return uint16(b[off]) | uint16(b[off+1])<<8 }

func Test_stub_synthesizer_rejects_empty_text(t *testing.T) {
	_, _, err := audioinfra.StubSynthesizer{}.Synthesize(context.Background(), "   ")
	require.Error(t, err, "text rỗng phải là lỗi chứ không phải WAV im lặng")
}

func Test_stub_transcriber_returns_v1_defaults(t *testing.T) {
	res, err := audioinfra.StubTranscriber{}.Transcribe(context.Background(), []byte("a"), "a.wav", "audio/wav")
	require.NoError(t, err)
	require.Equal(t, "ni hao", res.Text, "mặc định giữ nguyên v1")
	require.Equal(t, "zh", res.Lang)
}

func Test_stub_transcriber_rejects_empty_audio(t *testing.T) {
	_, err := audioinfra.StubTranscriber{}.Transcribe(context.Background(), nil, "a.wav", "audio/wav")
	require.Error(t, err)
}

// Dial với addr rỗng trả (nil, nil) — tín hiệu "không có service", KHÔNG phải
// lỗi. Nếu nó trả lỗi thì `NewFromEnv` phải bắt thêm 1 nhánh.
func Test_dial_with_empty_addr_returns_nil_without_error(t *testing.T) {
	conn, err := audioinfra.Dial("")
	require.NoError(t, err)
	require.Nil(t, conn)
}

// Timeout phải áp ở ADAPTER, không ở handler — nhờ vậy cả use case `practice` gọi
// cùng engine qua port cũng có trần.
func Test_with_tts_timeout_produces_deadline_error(t *testing.T) {
	// Server không trả lời ⇒ client phải hết giờ, không treo.
	hang := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(hang) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///hang",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return hang.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	s := audioinfra.NewSynthesizer(conn, audioinfra.WithTTSTimeout(80*time.Millisecond))
	start := time.Now()
	_, _, err = s.Synthesize(context.Background(), "ni hao")
	require.Error(t, err)
	require.Less(t, time.Since(start), 3*time.Second, "phải hết giờ nhanh, không treo")
}

func Test_engine_close_is_nil_safe(t *testing.T) {
	require.NoError(t, (*audioinfra.Engine)(nil).Close())
	var e *audioinfra.Engine
	require.NoError(t, e.Close())
}
