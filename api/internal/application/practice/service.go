package practice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/content"
	pdp "langapp/internal/domain/practice"
)

// Service là use case của context practice.
type Service struct {
	repo Repository
	uow  UnitOfWork
	stt  STTPort
	tts  TTSPort
	now  NowFunc
}

// NewService dựng service. nowFn nil → UTC thật.
func NewService(repo Repository, uow UnitOfWork, stt STTPort, tts TTSPort, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, stt: stt, tts: tts, now: nowFn}
}

// ── Shadowing ───────────────────────────────────────────────────────────────

// ShadowProgress là tiến độ shadowing hiện tại của 1 thẻ (JSON lưu trong note
// SHADOW|).
type ShadowProgress struct {
	CardID    int64
	Loops     int
	Rate      float64
	UpdatedAt string
}

// RecordShadowProgress lưu tiến độ 1 vòng A-B của thẻ.
//
// `loops` là số vòng TÍCH LUỸ sau vòng vừa nghe (client đếm, server không tự
// +1): nếu server tự tăng thì 1 request retry sẽ làm nhảy 2 vòng. `rate` đi
// qua domain.NormalizeRate nên 0 → 1.0 (client không gửi) còn ngoài
// [0.5, 1.5] → 400.
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

// GetShadowProgress trả tiến độ mới nhất của thẻ. Chưa luyện lần nào → trả
// session 0 vòng / rate 1.0 / mốc rỗng, KHÔNG phải 404 (app v1 trả đúng như
// vậy: UI luôn cần 1 giá trị để hiển thị).
func (s *Service) GetShadowProgress(ctx context.Context, cardID int64) (ShadowProgress, error) {
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
		// Note hỏng (JSON sai) không được làm hỏng cả endpoint: coi như chưa
		// có session nào.
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

// ── Ghi âm → STT → diff ─────────────────────────────────────────────────────

// TranscribeRecording chạy audio qua engine STT và trả transcript.
//
// KHÔNG chuẩn hóa phồn→giản ở bước này: `DiffAgainstSample` mới là nơi cần
// (chỉ khi có câu mẫu để so). Người dùng muốn xem transcript nguyên bản engine
// trả vẫn phải thấy đúng những gì engine nghe.
func (s *Service) TranscribeRecording(ctx context.Context, audio []byte, filename, contentType string) (Transcript, error) {
	if len(audio) == 0 {
		return Transcript{}, newError(StatusBadRequest, "audio rỗng")
	}
	if s.stt == nil {
		return Transcript{}, newError(StatusInternalServerError, "chưa cấu hình engine nhận dạng giọng nói")
	}
	t, err := s.stt.Transcribe(ctx, audio, filename, contentType)
	if err != nil {
		return Transcript{}, fmt.Errorf("nhận dạng giọng nói: %w", err)
	}
	return t, nil
}

// DiffToken là 1 từ trong kết quả so khớp.
type DiffToken struct {
	Text   string
	Status pdp.WordStatus
}

// DiffResult là kết quả chấm 1 lần ghi âm.
type DiffResult struct {
	// Transcript là bản ĐÃ chuẩn hóa phồn→giản, đúng bằng thứ đã so.
	// Không chuẩn hóa thì Whisper trả "學習" còn deck mẫu dùng "学习" → chấm
	// sai oan dù đọc đúng (xem api/simplify.go v1).
	Transcript string
	Diff       []DiffToken
	Wrong      []string
	Score      float64
}

// DiffAgainstSample so transcript với câu mẫu.
//
// Toàn bộ thuật toán (LCS + gộp cặp missing/extra thành "mẫu→đọc" + chuẩn
// hóa phồn→giản TRƯỚC khi so) nằm ở `domain/practice.WordDiff` — port từ
// web/src/player/diff.ts. Tầng này chỉ gọi lại, không viết bản thứ hai.
func (s *Service) DiffAgainstSample(sample, transcript string) (DiffResult, error) {
	if strings.TrimSpace(sample) == "" {
		return DiffResult{}, newError(StatusBadRequest, "thiếu câu mẫu")
	}
	// Chuẩn hóa TRƯỚC khi so (WordDiff tự làm nốt bên trong, nhưng transcript
	// trả về cho client phải là bản đã chuẩn hóa — nếu không, UI hiện chữ
	// phồn trong khi điểm đã chấm theo chữ giản).
	got := domain.ToSimplified(strings.TrimSpace(transcript))
	diff, wrong, score := pdp.Compare(sample, got)
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

// SpeakSample tổng hợp mẫu để người dùng nghe chuẩn bị shadow. Cần TTSPort;
// không cấu hình thì 500 tường minh (không âm thầm trả rỗng).
func (s *Service) SpeakSample(ctx context.Context, sample, lang string) ([]byte, string, error) {
	if strings.TrimSpace(sample) == "" {
		return nil, "", newError(StatusBadRequest, "thiếu câu mẫu")
	}
	if s.tts == nil {
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

// AppendError lưu 1 lần sai vào sổ lỗi.
//
// `wrong` đi qua `domain.NormalizeWrong` (trim + hạ chữ thường, bỏ rỗng) ở
// CẢ 2 đầu: ghi thì để số đếm TopErrors nhất quán, đọc thì để note cũ ghi tay
// vẫn đếm được.
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

// ListErrors trả sổ lỗi mới nhất trước, tuỳ chọn lọc theo thẻ.
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

// TopErrorCount là 1 từ bị đọc sai kèm số lần.
type TopErrorCount struct {
	Word  string
	Count int
}

// TopErrorScanLimit là số note ERR| đọc để đếm. 500 là giữ nguyên hằng của v1
// (TopErrorsHandler v1) — đếm trên toàn bộ lịch sử thì số lần mới đúng, nhưng
// quét nhiều hơn thì chậm hơn; 500 note là mốc cân bằng app single-user.
const TopErrorScanLimit = 500

// TopErrors trả các từ sai nhiều nhất, tie-break theo chữ cái để thứ tự ổn
// định giữa 2 lần gọi.
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

// SuggestErrorsFromErrors gợi ý thẻ nên ôn lại: thẻ có nhiều lỗi ERR| nhất.
// Client gọi tiếp endpoint ôn SRS để chấm như bình thường.
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

// MarkErrorResolved đánh dấu 1 lỗi đã xử lý.
//
// Sổ lỗi là append-only trong `notes` (không có cột `deleted`), nên "đánh dấu
// đã xử lý" = ghi 1 note MỚI cùng tham chiếu id lỗi. Cách này giữ được tính
// append-only mà sync union theo guid vẫn đúng — nếu xoá cứng note gốc thì
// máy peer không bao giờ nhận được trạng thái đã xử lý.
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

// IsResolvedNote báo note có phải dạng đánh dấu đã xử lý không. Dùng để lọc
// khỏi danh sách lỗi: 1 note ERR| có 2 hình dạng JSON (ghi lỗi / đánh dấu
// xử lý) và cả hai đều dùng chung prefix.
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

// ── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) timestamp() string { return s.now().UTC().Format(time.RFC3339) }

// inTx chạy fn trong UnitOfWork; uow nil (test chỉ cần validate) → chạy thẳng.
func (s *Service) inTx(ctx context.Context, fn func(tx Tx) error) error {
	if s.uow == nil {
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
