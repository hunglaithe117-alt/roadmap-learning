package practice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pdp "langapp/internal/domain/practice"
)

var errFake = errors.New("lỗi giả lập")

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fakeRepo là repository `notes` trong bộ nhớ. Tầng application không được
// import tầng hạ tầng (luật DDD §2 — grep quét cả `_test.go`).
type fakeRepo struct {
	notes     []Note
	nextID    int64
	failOn    string
	suggestOK []CardErrorCount
}

func newFakeRepo() *fakeRepo { return &fakeRepo{nextID: 1} }

func (f *fakeRepo) AppendNote(_ context.Context, _ Tx, n *Note) error {
	if f.failOn == "AppendNote" {
		return errFake
	}
	n.ID = f.nextID
	f.nextID++
	f.notes = append(f.notes, *n)
	return nil
}

func (f *fakeRepo) LatestShadowNote(_ context.Context, cardID int64) (*Note, error) {
	for i := len(f.notes) - 1; i >= 0; i-- {
		n := f.notes[i]
		if n.CardID != nil && *n.CardID == cardID && strings.HasPrefix(n.Text, pdp.ShadowPrefix) {
			return &n, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) ListErrorNotes(_ context.Context, cardID *int64, limit int) ([]Note, error) {
	out := []Note{}
	for i := len(f.notes) - 1; i >= 0 && len(out) < limit; i-- {
		n := f.notes[i]
		if !strings.HasPrefix(n.Text, pdp.ErrorPrefix) {
			continue
		}
		if cardID != nil && (n.CardID == nil || *n.CardID != *cardID) {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeRepo) CountErrorNotesByCard(context.Context, int) ([]CardErrorCount, error) {
	return f.suggestOK, nil
}

// fakeSTT là engine nhận dạng giọng nói giả lập. Nó trả CHỮ PHỒN cố ý — đó là
// hành vi thật của Whisper, và là lý do `DiffAgainstSample` phải chuẩn hóa
// trước khi so.
type fakeSTT struct{ text string }

func (f fakeSTT) Transcribe(context.Context, []byte, string, string) (Transcript, error) {
	return Transcript{Text: f.text, Lang: "zh"}, nil
}

type fakeTTS struct{}

func (fakeTTS) Synthesize(_ context.Context, text, lang string) ([]byte, string, error) {
	return []byte("WAV:" + lang + ":" + text), "audio/wav", nil
}

func newService(repo Repository) *Service {
	return NewService(repo, nil, fakeSTT{text: "學習中文"}, fakeTTS{},
		func() time.Time { return fixedNow })
}

// ── Shadowing ───────────────────────────────────────────────────────────────

func Test_record_shadow_progress_clamps_rate_and_writes_note(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)

	got, err := svc.RecordShadowProgress(context.Background(), 7, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, got.Loops)
	assert.Equal(t, 1.0, got.Rate, "rate 0 = client không gửi → mặc định 1.0")

	require.Len(t, repo.notes, 1)
	require.NotNil(t, repo.notes[0].CardID)
	assert.Equal(t, int64(7), *repo.notes[0].CardID)
	assert.True(t, strings.HasPrefix(repo.notes[0].Text, pdp.ShadowPrefix),
		"phải dùng prefix reserved SHADOW|")
	assert.NotEmpty(t, repo.notes[0].GUID, "guid rỗng sẽ đụng UNIQUE ở note thứ 2")
}

func Test_record_shadow_progress_rejects_rate_out_of_range(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)

	_, err := svc.RecordShadowProgress(context.Background(), 7, 3, 2.0)
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Empty(t, repo.notes, "lỗi validate phải chặn TRƯỚC khi ghi")
}

func Test_record_shadow_progress_rejects_negative_loops(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)

	_, err := svc.RecordShadowProgress(context.Background(), 7, -1, 1)
	require.Error(t, err)
	assert.Empty(t, repo.notes)
}

func Test_get_shadow_progress_returns_default_when_never_practised(t *testing.T) {
	svc := newService(newFakeRepo())

	got, err := svc.GetShadowProgress(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Loops)
	assert.Equal(t, 1.0, got.Rate)
	assert.Empty(t, got.UpdatedAt, "chưa luyện = không có mốc, không phải 404")
}

func Test_get_shadow_progress_returns_latest_note(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.RecordShadowProgress(context.Background(), 7, 1, 1.0)
	require.NoError(t, err)
	_, err = svc.RecordShadowProgress(context.Background(), 7, 5, 0.75)
	require.NoError(t, err)

	got, err := svc.GetShadowProgress(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 5, got.Loops, "phải lấy note MỚI NHẤT")
	assert.Equal(t, 0.75, got.Rate)
}

func Test_get_shadow_progress_ignores_corrupted_note(t *testing.T) {
	repo := newFakeRepo()
	repo.notes = []Note{{
		ID: 1, CardID: ptrInt64(7), Text: pdp.ShadowPrefix + "KHÔNG PHẢI JSON",
		CreatedAt: "2026-09-28T00:00:00Z",
	}}
	svc := newService(repo)

	got, err := svc.GetShadowProgress(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Loops, "note hỏng không được làm hỏng endpoint")
}

func ptrInt64(n int64) *int64 { return &n }

// ── Diff ────────────────────────────────────────────────────────────────────

func Test_diff_against_sample_normalises_traditional_before_comparing(t *testing.T) {
	svc := newService(newFakeRepo())

	// STT trả "學習中文" (phồn), câu mẫu "学习中文" (giản). Không chuẩn hóa thì
	// 4 từ đều sai dù đọc đúng — đây là bug v1 đã phải vá bằng `simplify.go`.
	got, err := svc.DiffAgainstSample("学习中文", "學習中文")
	require.NoError(t, err)
	assert.Equal(t, "学习中文", got.Transcript, "transcript trả về phải là bản đã chuẩn hóa")
	assert.Equal(t, 1.0, got.Score)
	assert.Empty(t, got.Wrong, "đọc đúng thì không được ghi từ sai")
}

func Test_diff_against_sample_reports_wrong_words(t *testing.T) {
	svc := newService(newFakeRepo())

	got, err := svc.DiffAgainstSample("I want to make progress", "I want make progress")
	require.NoError(t, err)
	assert.NotEqual(t, 1.0, got.Score)
	// LCS khớp "make progress" và báo thiếu "to" — đúng hành vi port từ
	// web/src/player/diff.ts (thiếu 1 từ function word giữa 2 từ khớp).
	assert.Equal(t, []string{"to"}, got.Wrong)
}

func Test_diff_against_sample_requires_sample(t *testing.T) {
	svc := newService(newFakeRepo())

	_, err := svc.DiffAgainstSample("  ", "x")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_transcribe_recording_returns_engine_output_verbatim(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, fakeSTT{text: "學習中文"}, nil, nil)

	got, err := svc.TranscribeRecording(context.Background(), []byte("audio"), "a.wav", "audio/wav")
	require.NoError(t, err)
	assert.Equal(t, "學習中文", got.Text, "bước này KHÔNG chuẩn hóa — chỉ diff mới cần")
	assert.Equal(t, "zh", got.Lang)
}

func Test_transcribe_recording_rejects_empty_audio(t *testing.T) {
	svc := newService(newFakeRepo())

	_, err := svc.TranscribeRecording(context.Background(), nil, "a.wav", "audio/wav")
	require.Error(t, err)
}

func Test_speak_sample_uses_tts_port(t *testing.T) {
	svc := newService(newFakeRepo())

	audio, contentType, err := svc.SpeakSample(context.Background(), "你好", "zh")
	require.NoError(t, err)
	assert.Equal(t, "audio/wav", contentType)
	assert.Contains(t, string(audio), "你好")
}

// ── Sổ lỗi ─────────────────────────────────────────────────────────────────

func Test_append_error_normalises_wrong_words(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)

	got, err := svc.AppendError(context.Background(), nil, "I want progress", "I want progres", []string{" Progress ", "", "PROGRESS"})
	require.NoError(t, err)
	assert.Equal(t, []string{"progress", "progress"}, got.Wrong,
		"trim + hạ chữ thường, giữ nguyên số lần (sổ lỗi cần đếm được)")
	assert.Equal(t, []string{"progress", "progress"}, got.Wrong, "nil phải thành mảng rỗng, không phải null")
}

func Test_append_error_requires_both_texts(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)

	_, err := svc.AppendError(context.Background(), nil, "", "x", nil)
	require.Error(t, err)
	assert.Empty(t, repo.notes)
}

func Test_list_errors_filters_by_card(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.AppendError(context.Background(), ptrInt64(1), "a b", "a c", nil)
	require.NoError(t, err)
	_, err = svc.AppendError(context.Background(), ptrInt64(2), "d e", "d f", nil)
	require.NoError(t, err)
	_, err = svc.AppendError(context.Background(), nil, "g h", "g i", nil)
	require.NoError(t, err)

	all, err := svc.ListErrors(context.Background(), nil, 50)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	one, err := svc.ListErrors(context.Background(), ptrInt64(1), 50)
	require.NoError(t, err)
	require.Len(t, one, 1)
	assert.Equal(t, "a b", one[0].Expected)
}

func Test_top_errors_counts_occurrences_and_breaks_ties_alphabetically(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.AppendError(context.Background(), nil, "x", "y", []string{"the", "a"})
	require.NoError(t, err)
	_, err = svc.AppendError(context.Background(), nil, "x", "y", []string{"the", "a"})
	require.NoError(t, err)
	_, err = svc.AppendError(context.Background(), nil, "x", "y", []string{"the"})
	require.NoError(t, err)

	got, err := svc.TopErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, TopErrorCount{Word: "the", Count: 3}, got[0])
	assert.Equal(t, TopErrorCount{Word: "a", Count: 2}, got[1])

	limited, err := svc.TopErrors(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

func Test_top_errors_ignores_resolved_markers(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.AppendError(context.Background(), nil, "x", "y", []string{"gone"})
	require.NoError(t, err)

	// Note đánh dấu đã xử lý KHÔNG có mảng wrong → không đóng góp cho đếm.
	_, err = svc.MarkErrorResolved(context.Background(), 1)
	require.NoError(t, err)

	got, err := svc.TopErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "gone", got[0].Word, "lỗi gốc vẫn còn; chỉ note đánh dấu là không đếm")
}

func Test_suggest_errors_joins_cards_in_repository(t *testing.T) {
	repo := newFakeRepo()
	repo.suggestOK = []CardErrorCount{{CardID: 1, Front: "你好", Back: "xin chào", Errors: 5}}
	svc := newService(repo)

	got, err := svc.SuggestErrorsFromErrors(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 5, got[0].Errors)
	assert.Equal(t, "你好", got[0].Front, "JOIN phải lấy mặt trước/sau trong cùng 1 query")
}

func Test_mark_error_resolved_appends_note_not_delete(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	orig, err := svc.AppendError(context.Background(), ptrInt64(3), "a", "b", nil)
	require.NoError(t, err)

	markerID, err := svc.MarkErrorResolved(context.Background(), orig.ID)
	require.NoError(t, err)
	assert.NotEqual(t, orig.ID, markerID)
	require.Len(t, repo.notes, 2, "sổ lỗi append-only: xoá cứng sẽ không sync sang máy peer")

	assert.True(t, IsResolvedNote(repo.notes[1].Text))
	assert.False(t, IsResolvedNote(repo.notes[0].Text))
}

func Test_is_resolved_note_requires_payload(t *testing.T) {
	assert.False(t, IsResolvedNote("ERR|{\"expected\":\"x\"}"))
	assert.False(t, IsResolvedNote("THIEU|{\"resolved\":1}"))
	assert.False(t, IsResolvedNote("ERR|không phải json"))
	assert.True(t, IsResolvedNote("ERR|{\"resolved\":42}"))
}

func Test_mark_error_resolved_rejects_bad_id(t *testing.T) {
	svc := newService(newFakeRepo())

	_, err := svc.MarkErrorResolved(context.Background(), 0)
	require.Error(t, err)
}

// ── Prefix reserved ─────────────────────────────────────────────────────────

// 3 prefix của 3 context phải KHÔNG trùng nhau — nếu trùng thì query "note của
// tôi" lẫn sang sổ lỗi và checklist.
func Test_reserved_prefixes_do_not_collide(t *testing.T) {
	prefixes := map[string]string{
		"content":  THIEUContentPrefix,
		"practice": string(pdp.ShadowPrefix),
		"error":    string(pdp.ErrorPrefix),
	}
	require.Len(t, prefixes, 3)
	seen := map[string]string{}
	for owner, p := range prefixes {
		other, dup := seen[p]
		require.False(t, dup, "prefix %q bị dùng bởi cả %s và %s", p, owner, other)
		seen[p] = owner
	}
}

// THIEUContentPrefix khai lại ở đây để test chứng minh prefix của `content`
// không trùng 2 prefix kia mà không import chéo package (application/content
// và application/practice là 2 context độc lập).
const THIEUContentPrefix = "THIEU|"

func Test_note_payload_json_keys_match_v1_contract(t *testing.T) {
	// UI v1 đọc `expected`/`transcript`/`wrong` và `loops`/`rate`; đổi tên key
	// là hỏng client cũ.
	repo := newFakeRepo()
	svc := newService(repo)
	_, err := svc.AppendError(context.Background(), nil, "a", "b", []string{"c"})
	require.NoError(t, err)
	var errPayload map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(repo.notes[0].Text, pdp.ErrorPrefix)), &errPayload))
	for _, k := range []string{"expected", "transcript", "wrong"} {
		assert.Contains(t, errPayload, k)
	}

	_, err = svc.RecordShadowProgress(context.Background(), 1, 2, 1.0)
	require.NoError(t, err)
	var shPayload map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(repo.notes[1].Text, pdp.ShadowPrefix)), &shPayload))
	for _, k := range []string{"loops", "rate"} {
		assert.Contains(t, shPayload, k)
	}
}
