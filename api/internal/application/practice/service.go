package practice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/content"
	pdp "langapp/internal/domain/practice"
	"langapp/internal/typednil"
)

// Service coordinates speaking practice use cases.
type Service struct {
	repo Repository
	uow  UnitOfWork
	stt  STTPort
	tts  TTSPort
	now  NowFunc
}

// NewService constructs a practice service.
func NewService(repo Repository, uow UnitOfWork, stt STTPort, tts TTSPort, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, stt: stt, tts: tts, now: nowFn}
}

// ShadowProgress tracks shadowing loops and playback rate for a card.
type ShadowProgress struct {
	CardID    int64
	Loops     int
	Rate      float64
	UpdatedAt string
}

// RecordShadowProgress records an A-B loop shadowing progress entry.
func (s *Service) RecordShadowProgress(ctx context.Context, cardID int64, loops int, rate float64) (ShadowProgress, error) {
	if cardID <= 0 {
		return ShadowProgress{}, newError(StatusBadRequest, "thiếu card_id")
	}
	if err := pdp.ValidateLoops(loops); err != nil {
		return ShadowProgress{}, newError(StatusBadRequest, "%s", err.Error())
	}
	normalized, err := pdp.NormalizeRate(rate)
	if err != nil {
		return ShadowProgress{}, newError(StatusBadRequest, "%s", err.Error())
	}
	now := s.timestamp()
	payload, err := json.Marshal(map[string]any{"loops": loops, "rate": normalized})
	if err != nil {
		return ShadowProgress{}, fmt.Errorf("mã hoá tiến độ: %w", err)
	}
	var out ShadowProgress
	txErr := s.inTx(ctx, func(tx Tx) error {
		n := Note{
			CardID: &cardID, Text: pdp.ShadowPrefix + string(payload),
			CreatedAt: now, GUID: NewGUID(),
		}
		if err := s.repo.AppendNote(ctx, tx, &n); err != nil {
			return fmt.Errorf("lưu tiến độ: %w", err)
		}
		out = ShadowProgress{CardID: cardID, Loops: loops, Rate: normalized, UpdatedAt: now}
		return nil
	})
	if txErr != nil {
		return ShadowProgress{}, txErr
	}
	return out, nil
}

// LoadShadowProgress retrieves the latest shadowing progress for a card.
func (s *Service) LoadShadowProgress(ctx context.Context, cardID int64) (ShadowProgress, error) {
	if cardID <= 0 {
		return ShadowProgress{}, newError(StatusBadRequest, "thiếu card_id")
	}
	note, err := s.repo.LatestShadowNote(ctx, cardID)
	if err != nil {
		return ShadowProgress{}, fmt.Errorf("đọc tiến độ: %w", err)
	}
	if note == nil {
		return ShadowProgress{CardID: cardID, Rate: pdp.DefaultRate}, nil
	}
	progress, ok := parseShadowNote(*note)
	if !ok {
		return ShadowProgress{CardID: cardID, Rate: pdp.DefaultRate}, nil
	}
	return progress, nil
}

func parseShadowNote(n Note) (ShadowProgress, bool) {
	var payload struct {
		Loops int     `json:"loops"`
		Rate  float64 `json:"rate"`
	}
	if !strings.HasPrefix(n.Text, pdp.ShadowPrefix) {
		return ShadowProgress{}, false
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(n.Text, pdp.ShadowPrefix)), &payload); err != nil {
		return ShadowProgress{}, false
	}
	cardID := int64(0)
	if n.CardID != nil {
		cardID = *n.CardID
	}
	return ShadowProgress{
		CardID: cardID, Loops: payload.Loops, Rate: payload.Rate, UpdatedAt: n.CreatedAt,
	}, true
}

// TranscribeRecording sends audio to the STT engine and returns the transcript.
func (s *Service) TranscribeRecording(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error) {
	if len(audio) == 0 {
		return Transcript{}, newError(StatusBadRequest, "audio rỗng")
	}
	if typednil.Is(s.stt) {
		return Transcript{}, newError(StatusInternalServerError, "chưa cấu hình engine nhận dạng giọng nói")
	}
	t, err := s.stt.Transcribe(ctx, audio, filename, contentType)
	if err != nil {
		return Transcript{}, fmt.Errorf("nhận dạng giọng nói: %w", err)
	}
	return t, nil
}

// DiffToken represents a token in a diff comparison.
type DiffToken struct {
	Text   string
	Status pdp.WordStatus
}

// DiffResult holds speech diff comparison results.
type DiffResult struct {
	Transcript string
	Diff       []DiffToken
	Wrong      []string
	Score      float64
}

// DiffAgainstSample compares an audio transcript against an expected sample sentence.
func (s *Service) DiffAgainstSample(ctx context.Context, sample, transcript string) (DiffResult, error) {
	if err := ctx.Err(); err != nil {
		return DiffResult{}, err
	}
	if strings.TrimSpace(sample) == "" {
		return DiffResult{}, newError(StatusBadRequest, "thiếu câu mẫu")
	}
	got := domain.ToSimplified(strings.TrimSpace(transcript))
	diff, wrong, score, err := pdp.Compare(ctx, sample, got)
	if err != nil {
		return DiffResult{}, err
	}
	tokens := make([]DiffToken, 0, len(diff))
	for _, t := range diff {
		tokens = append(tokens, DiffToken{Text: t.Text, Status: t.Status})
	}
	return DiffResult{
		Transcript: got,
		Diff:       tokens,
		Wrong:      pdp.NormalizeWrong(wrong),
		Score:      score,
	}, nil
}

// SpeakSample synthesizes audio for a sample practice sentence.
func (s *Service) SpeakSample(ctx context.Context, sample, lang string) ([]byte, string, error) {
	if strings.TrimSpace(sample) == "" {
		return nil, "", newError(StatusBadRequest, "thiếu câu mẫu")
	}
	if typednil.Is(s.tts) {
		return nil, "", newError(StatusInternalServerError, "chưa cấu hình engine tổng hợp giọng nói")
	}
	audio, contentType, err := s.tts.Synthesize(ctx, sample, lang)
	if err != nil {
		return nil, "", fmt.Errorf("tổng hợp giọng nói: %w", err)
	}
	return audio, contentType, nil
}

// ── Sổ lỗi ─────────────────────────────────────────────────────────────────

// ErrorEntry là 1 dòng sổ lỗi đã đọc từ note ERR|.
type ErrorEntry struct {
	ID         int64
	CardID     *int64
	Expected   string
	Transcript string
	Wrong      []string
	CreatedAt  string
}

// DefaultErrorLimit là số lỗi mặc định khi list; tối đa 200 (giữ LIMIT của v1).
const (
	DefaultErrorLimit = 50
	MaxErrorLimit     = 200
)

// AppendError records a speaking error into the error notebook.
func (s *Service) AppendError(ctx context.Context, cardID *int64, expected, transcript string, wrong []string) (ErrorEntry, error) {
	exp := strings.TrimSpace(expected)
	tr := strings.TrimSpace(transcript)
	if exp == "" || tr == "" {
		return ErrorEntry{}, newError(StatusBadRequest, "thiếu câu mẫu hoặc transcript")
	}
	normalized := pdp.NormalizeWrong(wrong)
	if normalized == nil {
		normalized = []string{}
	}
	now := s.timestamp()
	payload, err := json.Marshal(map[string]any{
		"expected": exp, "transcript": tr, "wrong": normalized,
	})
	if err != nil {
		return ErrorEntry{}, fmt.Errorf("mã hoá lỗi: %w", err)
	}
	var out ErrorEntry
	txErr := s.inTx(ctx, func(tx Tx) error {
		n := Note{
			CardID: cardID, Text: pdp.ErrorPrefix + string(payload),
			CreatedAt: now, GUID: NewGUID(),
		}
		if err := s.repo.AppendNote(ctx, tx, &n); err != nil {
			return fmt.Errorf("lưu lỗi: %w", err)
		}
		out = ErrorEntry{
			ID: n.ID, CardID: cardID, Expected: exp, Transcript: tr,
			Wrong: normalized, CreatedAt: now,
		}
		return nil
	})
	if txErr != nil {
		return ErrorEntry{}, txErr
	}
	return out, nil
}

// ListErrors returns error notebook entries, latest first, optionally filtered by cardID.
func (s *Service) ListErrors(ctx context.Context, cardID *int64, limit int) ([]ErrorEntry, error) {
	notes, err := s.repo.ListErrorNotes(ctx, cardID, clampLimit(limit, DefaultErrorLimit, MaxErrorLimit))
	if err != nil {
		return nil, fmt.Errorf("đọc sổ lỗi: %w", err)
	}
	out := make([]ErrorEntry, 0, len(notes))
	for _, n := range notes {
		if e, ok := parseErrorNote(n); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// TopErrorCount records the occurrence count of a missed word.
type TopErrorCount struct {
	Word  string
	Count int
}

// TopErrorScanLimit is the scan limit used to compute aggregate top errors.
const TopErrorScanLimit = 500

// TopErrors aggregates the most frequent errors with alphabetical tie-breaking.
func (s *Service) TopErrors(ctx context.Context, limit int) ([]TopErrorCount, error) {
	entries, err := s.ListErrors(ctx, nil, TopErrorScanLimit)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, e := range entries {
		for _, w := range pdp.NormalizeWrong(e.Wrong) {
			counts[w]++
		}
	}
	out := make([]TopErrorCount, 0, len(counts))
	for w, c := range counts {
		out = append(out, TopErrorCount{Word: w, Count: c})
	}
	sortTopErrors(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// SuggestErrorsFromErrors suggests cards with the highest error counts for review.
func (s *Service) SuggestErrorsFromErrors(ctx context.Context, limit int) ([]CardErrorCount, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.repo.CountErrorNotesByCard(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("đọc gợi ý ôn: %w", err)
	}
	return rows, nil
}

// MarkErrorResolved marks an error entry as resolved by appending a resolution note.
func (s *Service) MarkErrorResolved(ctx context.Context, errorID int64) (int64, error) {
	if errorID <= 0 {
		return 0, newError(StatusBadRequest, "id lỗi không hợp lệ")
	}
	now := s.timestamp()
	payload, err := json.Marshal(map[string]any{"resolved": errorID})
	if err != nil {
		return 0, fmt.Errorf("mã hoá đánh dấu: %w", err)
	}
	var out int64
	txErr := s.inTx(ctx, func(tx Tx) error {
		n := Note{
			Text: pdp.ErrorPrefix + string(payload), CreatedAt: now, GUID: NewGUID(),
		}
		if err := s.repo.AppendNote(ctx, tx, &n); err != nil {
			return fmt.Errorf("lưu đánh dấu: %w", err)
		}
		out = n.ID
		return nil
	})
	if txErr != nil {
		return 0, txErr
	}
	return out, nil
}

// IsResolvedNote reports whether an error note represents a resolution marker.
func IsResolvedNote(text string) bool {
	if !strings.HasPrefix(text, pdp.ErrorPrefix) {
		return false
	}
	var payload struct {
		Resolved int64 `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(text, pdp.ErrorPrefix)), &payload); err != nil {
		return false
	}
	return payload.Resolved > 0
}

func parseErrorNote(n Note) (ErrorEntry, bool) {
	if !strings.HasPrefix(n.Text, pdp.ErrorPrefix) {
		return ErrorEntry{}, false
	}
	var payload struct {
		Expected   string   `json:"expected"`
		Transcript string   `json:"transcript"`
		Wrong      []string `json:"wrong"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(n.Text, pdp.ErrorPrefix)), &payload); err != nil {
		return ErrorEntry{}, false
	}
	if payload.Wrong == nil {
		payload.Wrong = []string{}
	}
	return ErrorEntry{
		ID: n.ID, CardID: n.CardID, Expected: payload.Expected,
		Transcript: payload.Transcript, Wrong: payload.Wrong, CreatedAt: n.CreatedAt,
	}, true
}

func (s *Service) timestamp() string { return s.now().UTC().Format(time.RFC3339) }

func (s *Service) inTx(ctx context.Context, fn func(tx Tx) error) error {
	if typednil.Is(s.uow) {
		return fn(nil)
	}
	return s.uow.Do(ctx, fn)
}

func clampLimit(limit, def, hi int) int {
	if limit <= 0 {
		return def
	}
	if limit > hi {
		return hi
	}
	return limit
}
