package practiceinfra_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	app "langapp/internal/application/practice"
)

// seedCard tạo 1 thẻ để note `SHADOW|`/`ERR|` gắn vào.
func seedCard(t *testing.T, db *gorm.DB, front string) int64 {
	t.Helper()
	var deckID int64
	require.NoError(t, db.Raw(
		"INSERT INTO decks (name, lang, created_at, guid) VALUES ('D', 'zh', '2026-01-01', ?) RETURNING id",
		"gd-"+front).Scan(&deckID).Error)
	var cardID int64
	require.NoError(t, db.Raw(`INSERT INTO cards
		(deck_id, front, back, due_at, state, created_at, guid)
		VALUES (?, ?, 'b', '2026-01-02', 'new', '2026-01-01', ?) RETURNING id`,
		deckID, front, "gc-"+front).Scan(&cardID).Error)
	return cardID
}

func cardFront(t *testing.T, db *gorm.DB, cardID int64) string {
	t.Helper()
	var front string
	require.NoError(t, db.Raw("SELECT front FROM cards WHERE id = ?", cardID).Scan(&front).Error)
	return front
}

// ── Shadowing ───────────────────────────────────────────────────────────────

func Test_shadow_progress_roundtrip_through_notes(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardID := seedCard(t, db, "你好")

	_, err := svc.RecordShadowProgress(ctx, cardID, 3, 0.75)
	require.NoError(t, err)

	got, err := svc.GetShadowProgress(ctx, cardID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.Loops)
	assert.InDelta(t, 0.75, got.Rate, 1e-9)
	assert.Equal(t, fixedNow.Format(time.RFC3339), got.UpdatedAt)

	var text string
	require.NoError(t, db.Raw("SELECT text FROM notes WHERE card_id = ?", cardID).Scan(&text).Error)
	assert.True(t, strings.HasPrefix(text, "SHADOW|"), "phải dùng prefix reserved SHADOW|")
}

func Test_shadow_progress_rejects_rate_out_of_range(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	cardID := seedCard(t, db, "你好")

	_, err := svc.RecordShadowProgress(context.Background(), cardID, 1, 1.9)
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM notes").Scan(&n).Error)
	assert.EqualValues(t, 0, n, "rate ngoài khoảng không được ghi xuống DB")
}

func Test_shadow_progress_rolls_back_on_invalid_card(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	// Note gắn card_id không tồn tại → FK chặn → phải rollback, không để lại
	// note mồ côi.
	_, err := svc.RecordShadowProgress(context.Background(), 999999, 1, 1)
	require.Error(t, err, "FK notes.card_id phải chặn")

	var n int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM notes").Scan(&n).Error)
	assert.EqualValues(t, 0, n)
}

// ── Diff ────────────────────────────────────────────────────────────────────

func Test_diff_normalises_traditional_chinese_from_stt_output(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	// Whisper trả "學習中文" (phồn) trong khi deck mẫu dùng "学习中文" (giản).
	// Không chuẩn hóa thì 4 từ đều bị chấm sai dù đọc đúng.
	got, err := svc.DiffAgainstSample("学习中文", "學習中文")
	require.NoError(t, err)
	assert.Equal(t, "学习中文", got.Transcript)
	assert.Equal(t, 1.0, got.Score)
	assert.Empty(t, got.Wrong)
}

func Test_diff_reports_wrong_words_for_shadowing_mistake(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	got, err := svc.DiffAgainstSample("I want to make progress", "I want make progress")
	require.NoError(t, err)
	assert.Less(t, got.Score, 1.0)
	assert.Equal(t, []string{"to"}, got.Wrong)
}

// ── Sổ lỗi ─────────────────────────────────────────────────────────────────

func Test_error_book_roundtrip_filters_by_card(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardA := seedCard(t, db, "你好")
	cardB := seedCard(t, db, "谢谢")

	_, err := svc.AppendError(ctx, &cardA, "I want progress", "I want progres", []string{"progress"})
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, &cardB, "thank you", "thank yu", []string{"you"})
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, nil, "free practice", "free practise", []string{"practise"})
	require.NoError(t, err)

	all, err := svc.ListErrors(ctx, nil, 50)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	forA, err := svc.ListErrors(ctx, &cardA, 50)
	require.NoError(t, err)
	require.Len(t, forA, 1)
	assert.Equal(t, "I want progress", forA[0].Expected)
}

func Test_error_book_prefix_filter_excludes_thieu_checklist(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	_, err := svc.AppendError(ctx, nil, "a", "b", []string{"x"})
	require.NoError(t, err)
	// Checklist THIEU của context khác cùng nằm trong bảng `notes`.
	require.NoError(t, db.Exec(
		"INSERT INTO notes (card_id, text, created_at, guid) VALUES (NULL, ?, '2026-01-01', 'gn1')",
		`THIEU|{"session":"2026-01-01","scores":{}}`).Error)

	all, err := svc.ListErrors(ctx, nil, 50)
	require.NoError(t, err)
	assert.Len(t, all, 1, "checklist THIEU không được lẫn vào sổ lỗi")
}

func Test_top_errors_counts_recurring_mistakes(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	_, err := svc.AppendError(ctx, nil, "x", "y", []string{"the", "a"})
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, nil, "x", "y", []string{"the"})
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, nil, "x", "y", []string{"the", "a"})
	require.NoError(t, err)

	got, err := svc.TopErrors(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "the", got[0].Word)
	assert.Equal(t, 3, got[0].Count)
	assert.Equal(t, "a", got[1].Word)
	assert.Equal(t, 2, got[1].Count)
}

func Test_suggest_errors_joins_cards_in_one_query(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardID := seedCard(t, db, "你好")

	for i := 0; i < 3; i++ {
		_, err := svc.AppendError(ctx, &cardID, "a", "b", nil)
		require.NoError(t, err)
	}
	// Lỗi luyện tự do không gắn thẻ → không được gợi ý ôn.
	_, err := svc.AppendError(ctx, nil, "c", "d", nil)
	require.NoError(t, err)

	got, err := svc.SuggestErrorsFromErrors(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, cardID, got[0].CardID)
	assert.Equal(t, 3, got[0].Errors)
	assert.Equal(t, "你好", got[0].Front, "JOIN phải lấy mặt trước trong cùng 1 query")
}

func Test_suggest_errors_drops_soft_deleted_card(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardID := seedCard(t, db, "你好")
	_, err := svc.AppendError(ctx, &cardID, "a", "b", nil)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE cards SET deleted = 1 WHERE id = ?", cardID).Error)

	got, err := svc.SuggestErrorsFromErrors(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, got, "gợi ý ôn 1 thẻ đã xoá là vô nghĩa")
}

func Test_mark_error_resolved_appends_marker_note(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	entry, err := svc.AppendError(ctx, nil, "a", "b", []string{"x"})
	require.NoError(t, err)
	_, err = svc.MarkErrorResolved(ctx, entry.ID)
	require.NoError(t, err)

	var texts []string
	require.NoError(t, db.Raw("SELECT text FROM notes ORDER BY id").Scan(&texts).Error)
	require.Len(t, texts, 2, "sổ lỗi append-only: xoá cứng sẽ không sync sang máy peer")
	assert.True(t, app.IsResolvedNote(texts[1]))
	assert.False(t, app.IsResolvedNote(texts[0]))
}

func Test_notes_are_not_shared_between_cards(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardA := seedCard(t, db, "你好")
	cardB := seedCard(t, db, "谢谢")
	_, err := svc.AppendError(ctx, &cardA, "a", "b", nil)
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, &cardB, "c", "d", nil)
	require.NoError(t, err)

	forA, err := svc.ListErrors(ctx, &cardA, 50)
	require.NoError(t, err)
	require.Len(t, forA, 1)
	assert.Equal(t, "a", forA[0].Expected)
}

func Test_error_payload_keeps_v1_json_keys(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	_, err := svc.AppendError(context.Background(), nil, "mẫu", "đọc", []string{"Sai"})
	require.NoError(t, err)

	var text string
	require.NoError(t, db.Raw("SELECT text FROM notes").Scan(&text).Error)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(text, "ERR|")), &payload))
	for _, key := range []string{"expected", "transcript", "wrong"} {
		assert.Contains(t, payload, key, "UI v1 đọc key này, đổi tên là hỏng client cũ")
	}
	assert.Equal(t, []any{"sai"}, payload["wrong"], "wrong phải chuẩn hóa trim + hạ chữ thường")
}

func Test_transcribe_then_diff_flow_keeps_zh_together(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	tr, err := svc.TranscribeRecording(ctx, []byte("RIFFfake"), "rec.wav", "audio/wav")
	require.NoError(t, err)
	assert.Equal(t, "學習中文", tr.Text, "bước STT trả nguyên bản engine")

	got, err := svc.DiffAgainstSample("学习中文", tr.Text)
	require.NoError(t, err)
	assert.Equal(t, 1.0, got.Score)
}

func Test_speak_sample_returns_wav_bytes(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	audio, contentType, err := svc.SpeakSample(context.Background(), "你好", "zh")
	require.NoError(t, err)
	assert.Equal(t, "audio/wav", contentType)
	assert.NotEmpty(t, audio)
}

func Test_shadow_progress_does_not_appear_in_error_list(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	cardID := seedCard(t, db, "你好")

	_, err := svc.RecordShadowProgress(ctx, cardID, 5, 1.0)
	require.NoError(t, err)
	_, err = svc.AppendError(ctx, &cardID, "a", "b", nil)
	require.NoError(t, err)

	errors, err := svc.ListErrors(ctx, nil, 50)
	require.NoError(t, err)
	assert.Len(t, errors, 1, "note SHADOW| không được lọt vào sổ lỗi")
	_ = cardFront(t, db, cardID)
}
