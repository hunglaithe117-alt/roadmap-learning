package main

import (
	"context"
	"log/slog"

	audiopb "langapp/proto/audio/v1"
)

// server implements audiopb.AudioServer.
type server struct {
	synth       synthesizer
	transcriber transcriber
	log         *slog.Logger
}

// Synthesize synthesizes speech audio from text using the configured synthesizer.
func (s *server) Synthesize(ctx context.Context, req *audiopb.SynthesizeRequest) (*audiopb.SynthesizeResponse, error) {
	audio, contentType, err := s.synth.SynthesizeLang(ctx, req.GetText(), req.GetLang())
	if err != nil {
		s.log.Error("synthesize failed", slog.String("err", err.Error()))
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

// Transcribe transcribes speech audio to text using the configured transcriber.
func (s *server) Transcribe(ctx context.Context, req *audiopb.TranscribeRequest) (*audiopb.TranscribeResponse, error) {
	if len(req.GetAudio()) == 0 {
		return nil, ErrEmptyAudio
	}
	res, err := s.transcriber.Transcribe(ctx, req.GetAudio(), req.GetFilename(), req.GetContentType(), req.GetLang())
	if err != nil {
		s.log.Error("transcribe failed", slog.String("err", err.Error()))
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

// StreamSynthesize synthesizes audio and streams it in chunks.
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
