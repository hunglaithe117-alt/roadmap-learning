// Package audio defines interfaces and value objects for text-to-speech and speech-to-text.
package audio

import "context"

// TTSSynthesizer synthesizes speech audio from text.
type TTSSynthesizer interface {
	// Info describes the active engine metadata.
	Info() EngineInfo
	Synthesize(ctx context.Context, text string) (audio []byte, contentType string, err error)
}

// LangSynthesizer is an optional extension for language-specific speech synthesis.
type LangSynthesizer interface {
	SynthesizeLang(ctx context.Context, text, lang string) (audio []byte, contentType string, err error)
}

// STTTranscriber transcribes speech audio into text.
type STTTranscriber interface {
	Info() EngineInfo
	Transcribe(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error)
}

// DetailedTranscriber is an optional extension providing word-level timestamps.
type DetailedTranscriber interface {
	TranscribeDetailed(ctx context.Context, audio []byte, filename, contentType, lang string) (Transcript, error)
}

