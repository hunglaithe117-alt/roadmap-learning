package main

import (
	"context"
	"log/slog"

	audiopb "langapp/proto/audio/v1"
)

// server hiện thực `audiopb.AudioServer`. Mỗi method chỉ là adapter mỏng:
// nhận request protobuf → gọi synthesizer/transcriber → trả response protobuf.
// Không có business logic ở đây — nếu có, nó đã thuộc về app và phải nằm ở
// `internal/domain` hoặc `internal/application`.
type server struct {
	synth       synthesizer
	transcriber transcriber
	log         *slog.Logger
}

func (s *server) Synthesize(ctx context.Context, req *audiopb.SynthesizeRequest) (*audiopb.SynthesizeResponse, error) {
	audio, contentType, err := s.synth.SynthesizeLang(ctx, req.GetText(), req.GetLang())
	if err != nil {
		// Trả lỗi nguyên văn: app chuyển thành 502/504 theo mã, và message ở
		// log phía app phải nói đúng engine hỏng gì — không phải "internal error".
		s.log.Error("synthesize thất bại", slog.String("err", err.Error()))
		return nil, err
	}
	if contentType == "" {
		contentType = "audio/wav"
	}
	return &audiopb.SynthesizeResponse{
		Audio:       audio,
		ContentType: contentType,
		Engine:      s.synth.Name(),
		Real:        s.synth.Real(),
	}, nil
}

func (s *server) Transcribe(ctx context.Context, req *audiopb.TranscribeRequest) (*audiopb.TranscribeResponse, error) {
	if len(req.GetAudio()) == 0 {
		return nil, errEmptyAudio
	}
	res, err := s.transcriber.Transcribe(ctx, req.GetAudio(), req.GetFilename(), req.GetContentType(), req.GetLang())
	if err != nil {
		s.log.Error("transcribe thất bại", slog.String("err", err.Error()))
		return nil, err
	}
	out := &audiopb.TranscribeResponse{
		Text:       res.Text,
		Lang:       res.Lang,
		Confidence: res.Confidence,
		Engine:     s.transcriber.Name(),
		Real:       s.transcriber.Real(),
	}
	for _, w := range res.Words {
		out.Words = append(out.Words, &audiopb.WordTimestamp{
			Word:       w.Word,
			Start:      w.Start,
			End:        w.End,
			Confidence: w.Confidence,
		})
	}
	return out, nil
}

// StreamSynthesize tổng hợp rồi cắt WAV thành các `Chunk`.
//
// Vì sao KHÔNG stream từng byte của Piper: Piper chỉ ghi file khi xong, không
// có API xuất theo luồng. Giả vờ streaming bằng cách cắt buffer sau khi đã đợi
// hết chỉ làm client chờ lâu hơn chứ không giảm độ trễ. Cắt chunk vẫn có
// ích thật: client nhận được phần đầu tiên sớm hơn 1 message 10MB, và
// `maxAudioBytes` của client không cần nâng theo độ dài câu.
func (s *server) StreamSynthesize(req *audiopb.StreamSynthesizeRequest, stream audiopb.Audio_StreamSynthesizeServer) error {
	audio, _, err := s.synth.SynthesizeLang(stream.Context(), req.GetText(), req.GetLang())
	if err != nil {
		return err
	}
	for off := 0; off < len(audio); off += chunkSize {
		end := min(off+chunkSize, len(audio))
		if err := stream.Send(&audiopb.Chunk{Data: audio[off:end], Last: end == len(audio)}); err != nil {
			return err
		}
	}
	return nil
}
