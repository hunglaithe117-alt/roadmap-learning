package audio

import "time"

// Transcript represents speech recognition output.
type Transcript struct {
	Text       string
	Lang       string
	Words      []WordTimestamp
	Confidence *float64
}

// WordTimestamp holds timestamp information for a recognized word.
type WordTimestamp struct {
	Word       string
	Start      *float64
	End        *float64
	Confidence *float64
}

// Duration returns the word audio duration if both start and end bounds are present.
func (w WordTimestamp) Duration() (time.Duration, bool) {
	if w.Start == nil || w.End == nil {
		return 0, false
	}
	return time.Duration((*w.End - *w.Start) * float64(time.Second)), true
}

// EngineInfo describes the audio engine processing the request.
type EngineInfo struct {
	Name string
	Kind Kind
	Real bool
}

// Kind identifies the engine capability type.
type Kind string

const (
	// KindTTS denotes a text-to-speech engine.
	KindTTS Kind = "tts"
	// KindSTT denotes a speech-to-text engine.
	KindSTT Kind = "stt"
)

// Header is the HTTP response header indicating the active audio engine.
const Header = "X-Engine"

// NewEngineInfo creates an EngineInfo value.
func NewEngineInfo(name string, kind Kind, isReal bool) EngineInfo {
	return EngineInfo{Name: name, Kind: kind, Real: isReal}
}

