package main

import (
	"context"
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	domaincontent "langapp/internal/domain/content"
	audiopb "langapp/proto/audio/v1"
)

// serveBufconn dựng `server` thật (cùng code chạy production) trên listener
// trong bộ nhớ.
//
// Không spin process `audio-service` thật vì cần Piper/Whisper; test chỉ cần
// chứng minh service dịch đúng protobuf ↔ engine, và `go test ./...` phải chạy
// được trên máy không có model.
func serveBufconn(t *testing.T, srv *server) audiopb.AudioClient {
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
	return audiopb.NewAudioClient(conn)
}

func testServer(t *testing.T, synth synthesizer, tr transcriber) audiopb.AudioClient {
	t.Helper()
	return serveBufconn(t, &server{synth: synth, transcriber: tr, log: newLogger("error")})
}

// ── Synthesize ──────────────────────────────────────────────────────────────

func Test_synthesize_returns_stub_wav_and_marks_engine_not_real(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	resp, err := cli.Synthesize(context.Background(), &audiopb.SynthesizeRequest{
		Text: "ni hao", Lang: "zh",
	})
	require.NoError(t, err)

	require.Equal(t, "stub", resp.GetEngine(), "client dựa vào engine để gắn header X-Engine")
	require.False(t, resp.GetReal(), "stub phải tự báo real=false để UI hiện badge giả")
	require.Equal(t, "audio/wav", resp.GetContentType())
	require.Equal(t, "RIFF", string(resp.GetAudio()[0:4]))
}

func Test_synthesize_rejects_empty_text(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	_, err := cli.Synthesize(context.Background(), &audiopb.SynthesizeRequest{Text: "  "})
	require.Error(t, err, "text rỗng phải là lỗi chứ không phải WAV im lặng")
}

// Piper từ chối text > 500 rune — chặn 1 request tổng hợp hàng trăm KB và giữ
// 1 child process trong lúc đó.
func Test_piper_synthesizer_rejects_text_over_500_runes(t *testing.T) {
	p := piperSynthesizer{bin: "piper", model: "/models/x.onnx"}
	_, _, err := p.SynthesizeLang(context.Background(), string(make([]rune, 501)), "")
	require.Error(t, err)
}

// Voice theo lang: `PIPER_MODEL_ZH` cho zh, `_EN` cho en, rơi về voice chung khi
// lang lạ. Đây là hành vi v1 (`ModelFor`) — đổi thì voice của deck tiếng Anh đọc
// bằng giọng Trung.
func Test_piper_model_for_picks_voice_per_lang_with_fallback(t *testing.T) {
	p := piperSynthesizer{
		bin: "piper", model: "/models/shared.onnx",
		modelZH: "/models/zh.onnx", modelEN: "/models/en.onnx",
	}
	require.Equal(t, "/models/zh.onnx", p.ModelFor("zh"))
	require.Equal(t, "/models/zh.onnx", p.ModelFor("zh-CN"), "gợi ý lang có biến thể vẫn phải ra voice đúng")
	require.Equal(t, "/models/en.onnx", p.ModelFor("en_US"))
	require.Equal(t, "/models/shared.onnx", p.ModelFor("vi"), "lang lạ rơi về voice chung, KHÔNG phải lỗi")
	require.Equal(t, "/models/shared.onnx", p.ModelFor(""), "không gửi lang cũng dùng voice chung")
}

// Khi chỉ có voice EN (không có ZH) thì lệnh zh rơi về voice EN chứ không lỗi —
// giữ hành vi v1.
func Test_piper_model_for_falls_back_when_lang_voice_missing(t *testing.T) {
	p := piperSynthesizer{bin: "piper", model: "/models/en.onnx", modelEN: "/models/en.onnx"}
	require.Equal(t, "/models/en.onnx", p.ModelFor("zh"))
}

// ── Transcribe ───────────────────────────────────────────────────────────────

func Test_transcribe_returns_stub_transcript(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	resp, err := cli.Transcribe(context.Background(), &audiopb.TranscribeRequest{
		Audio: []byte("RIFF..."), Filename: "a.wav", ContentType: "audio/wav",
	})
	require.NoError(t, err)
	require.Equal(t, "ni hao", resp.GetText())
	require.Equal(t, "zh", resp.GetLang())
	require.False(t, resp.GetReal())
}

// Audio 0 byte phải là `InvalidArgument` để app map sang 400 (input sai) chứ
// không phải 502 — service KHÔNG có mã 400 nào khác.
func Test_transcribe_rejects_empty_audio_with_invalid_argument(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	_, err := cli.Transcribe(context.Background(), &audiopb.TranscribeRequest{})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

// Whisper trả chữ phồn ("學習") còn deck mẫu dùng giản ("学习"). Chuẩn hoá PHẢI
// xảy ra TRƯỚC khi đi qua ranh giới gRPC, vì app không biết engine nào đứng sau.
func Test_whisper_response_is_normalized_to_simplified_before_leaving_service(t *testing.T) {
	body := []byte(`{"text":"我們學習漢語","language":"zh"}`)
	res, err := decodeWhisper(body, "")
	require.NoError(t, err)

	require.Equal(t, "我们学习汉语", res.Text)
	require.Equal(t, "zh", res.Lang)
}

func Test_whisper_lang_param_comes_from_hint_or_is_omitted(t *testing.T) {
	require.Equal(t, "zh", whisperLang("ZH"), "gợi ý phải không phân biệt hoa thường")
	require.Equal(t, "en", whisperLang("english"))
	require.Equal(t, "", whisperLang("vi"), "lang lạ = để Whisper tự nhận dạng, không gửi tham số")
	require.Equal(t, "", whisperLang(""), "không gửi lang = tự nhận dạng")
}

// Words lấy từ `words` cấp cao nhất; nếu payload chỉ có `segments[].words` thì
// phải gom từ đó (sidecar trả cả 2 tuỳ phiên bản).
func Test_decode_whisper_falls_back_to_segment_words(t *testing.T) {
	body := []byte(`{"text":"a b","language":"en","segments":[{"text":"a b","words":[{"word":"a","start":0,"end":0.4},{"word":"b","start":0.4,"end":0.9}]}]}`)
	res, err := decodeWhisper(body, "")
	require.NoError(t, err)

	require.Len(t, res.Words, 2)
	require.Equal(t, "a", res.Words[0].Word)
	require.InDelta(t, 0.4, *res.Words[0].End, 0.0001)
	require.InDelta(t, 0.9, *res.Words[1].End, 0.0001)
}

// Bảng phồn→giản phải đủ cho ký tự test data dùng; thiếu 1 ký tự là chấm sai oan
// trong `practice` mà không ai thấy.
func Test_to_simplified_covers_every_traditional_char_in_migration_seed(t *testing.T) {
	for trad, simp := range map[rune]rune{
		'學': '学', '習': '习', '漢': '汉', '語': '语', '們': '们', '這': '这', '個': '个',
	} {
		t.Run(string(trad), func(t *testing.T) {
			require.Equal(t, string(simp), toSimplified(string(trad)))
		})
	}
	require.True(t, hasTraditional("我們學習"), "HasTraditional phải phát hiện chữ phồn")
	require.Equal(t, "我们学习", toSimplified("我们学习"), "đã giản thì idempotent")
	require.Equal(t, "", toSimplified(""))
}

// F5 — audio-service và app phải dùNG CHUNG 1 bảng phồn→giản.
//
// Trước remediation có 2 bảng ~100 mục rời nhau, KHÔNG test nào ràng buộc:
// `practice.DiffAgainstSample` dùng `domain.ToSimplified` còn audio-service dùng
// bản của nó ⇒ lệch bảng là chấm sai oan mà CI vẫn xanh.
//
// Test này quét TOÀN BỘ khoảng CJK (U+4E00–U+9FFF) và đối chiếu `toSimplified`
// với `domain/content.ToSimplified`. Duyệt hết khoảng thay vì liệt kê mục là
// chủ ý: một danh sách ký tự viết tay trong test chỉ chứng minh được mấy chữ
// đã biết, còn quét cả khoảng thì BẤT KỲ mục nào lệch — kể cả mục thêm về sau
// — đều làm đỏ. Chi phí ~21k vòng lặp, không cần DB.
//
// Không thêm hàm liệt kê bảng vào `domain/content` vì remediation này không
// được sửa tầng `domain`; đối chiếu hành vi là cách ràng buộc đúng mà không
// cần mở API mới.
func Test_audio_service_shares_the_domain_simplify_table(t *testing.T) {
	var mismatches int
	for r := rune(0x4E00); r <= 0x9FFF; r++ {
		in := string(r)
		got, want := toSimplified(in), domaincontent.ToSimplified(in)
		if got != want {
			t.Errorf("toSimplified(%q) = %q, domain/content cho %q ⇒ lệch bảng, chấm sai oan", in, got, want)
			mismatches++
			if mismatches > 10 {
				t.Fatal("quá nhiều mục lệch, dừng để log không ngập")
			}
		}
		if h1, h2 := hasTraditional(in), domaincontent.HasTraditional(in); h1 != h2 {
			t.Errorf("hasTraditional(%q) = %v, domain/content cho %v", in, h1, h2)
			mismatches++
			if mismatches > 10 {
				t.Fatal("quá nhiều mục lệch, dừng để log không ngập")
			}
		}
	}
}

func Test_to_simplified_returns_same_string_when_nothing_changes(t *testing.T) {
	// Không đổi ⇒ trả ĐÚNG chuỗi gốc, không cấp phát `[]rune` mới (đa số
	// transcript là ASCII).
	in := "hello world 123"
	require.Equal(t, in, toSimplified(in))
}

// ── StreamSynthesize ────────────────────────────────────────────────────────

// collectChunks đọc hết stream thật qua bufconn và trả về các chunk.
func collectChunks(t *testing.T, cli audiopb.AudioClient, text string) []*audiopb.Chunk {
	t.Helper()
	stream, err := cli.StreamSynthesize(context.Background(), &audiopb.StreamSynthesizeRequest{
		Text: text, Lang: "zh",
	})
	require.NoError(t, err)
	var chunks []*audiopb.Chunk
	for {
		c, err := stream.Recv()
		if err != nil {
			return chunks
		}
		chunks = append(chunks, c)
	}
}

// Chunk cuối phải có `last = true` để client biết đã hết; nếu không client treo
// chờ chunk thứ N+1.
func Test_stream_synthesize_marks_last_chunk(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	chunks := collectChunks(t, cli, "ni hao")
	require.NotEmpty(t, chunks)
	require.True(t, chunks[len(chunks)-1].GetLast(), "chunk cuối phải last=true")
	for i, c := range chunks[:len(chunks)-1] {
		require.False(t, c.GetLast(), "chunk %d (chưa cuối) không được last=true", i)
	}
}

// Ghép lại các chunk phải ra ĐÚNG WAV gốc — mất byte là client nghe sai mà không
// có lỗi nào báo.
func Test_stream_synthesize_chunks_reassemble_to_original_wav(t *testing.T) {
	cli := testServer(t, stubSynthesizer{}, stubTranscriber{})

	chunks := collectChunks(t, cli, "ni hao")
	var joined []byte
	for _, c := range chunks {
		require.LessOrEqual(t, len(c.GetData()), chunkSize, "chunk vượt kích thước đã định")
		joined = append(joined, c.GetData()...)
	}
	full, _, err := stubSynthesizer{}.SynthesizeLang(context.Background(), "ni hao", "zh")
	require.NoError(t, err)
	require.Equal(t, full, joined)
}

func Test_max_audio_bytes_matches_v1_limit(t *testing.T) {
	require.Equal(t, 10<<20, maxAudioBytes, "trần upload phải khớp `maxSTTBytes` 10MB của v1")
	require.Equal(t, 500, maxTextRunes, "trần text TTS phải khớp `maxTTSTextLen` của v1")
}

func Test_stub_sine_wav_is_16bit_mono(t *testing.T) {
	wav := sineWAV(16000, 0.5, 440)
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[20:22]), "PCM")
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[22:24]), "mono")
	require.Equal(t, uint16(16), binary.LittleEndian.Uint16(wav[34:36]), "16 bit")
}

func Test_run_rejects_bad_listen_address(t *testing.T) {
	// Bind 1 cổng đang bận phải fail lúc boot chứ không lắng nghe sai chỗ.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })
	t.Setenv("AUDIO_GRPC_ADDR", busy.Addr().String())

	err = run(newLogger("error"))
	require.Error(t, err, "addr đang bận phải báo lỗi lúc boot")
}
