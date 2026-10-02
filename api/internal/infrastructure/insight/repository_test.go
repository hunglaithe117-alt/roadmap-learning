package insightinfra_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	insightapp "langapp/internal/application/insight"
	insightinfra "langapp/internal/infrastructure/insight"
	"langapp/internal/platform/testdb"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, context.Background())
}

func newService(t *testing.T, db *gorm.DB) *insightapp.Service {
	t.Helper()
	return insightapp.NewService(insightinfra.NewRepository(db),
		func() time.Time { return fixedNow })
}

// seedCard tạo 1 thẻ với `due_at` cho trước.
func seedCard(t *testing.T, db *gorm.DB, front, dueAt, state string, deleted int) int64 {
	t.Helper()
	var deckID int64
	require.NoError(t, db.Raw(
		"INSERT INTO decks (name, lang, created_at, guid) VALUES ('D', 'zh', '2026-01-01', ?) "+
			"ON CONFLICT (guid) DO UPDATE SET name = 'D' RETURNING id", "gd-"+front).Scan(&deckID).Error)
	var cardID int64
	require.NoError(t, db.Raw(`INSERT INTO cards
		(deck_id, front, back, due_at, state, created_at, guid, deleted)
		VALUES (?, ?, 'b', ?, ?, '2026-01-01', ?, ?) RETURNING id`,
		deckID, front, dueAt, state, "gc-"+front, deleted).Scan(&cardID).Error)
	return cardID
}

func seedReview(t *testing.T, db *gorm.DB, cardID int64, grade int, at string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO reviews (card_id, grade, reviewed_at, next_due_at, guid)
		VALUES (?, ?, ?, ?, ?)`, cardID, grade, at, at, uuidLike(cardID, at, grade)).Error)
}

// uuidLike sinh guid review khác nhau cho mỗi lần gọi — `ux_reviews_guid` là
// UNIQUE nên 2 review cùng guid rỗng sẽ đụng nhau.
func uuidLike(cardID int64, at string, grade int) string {
	return "gr-" + at + "-" + strconv.FormatInt(cardID, 10) + "-" + strconv.Itoa(grade)
}

// ── Stats ───────────────────────────────────────────────────────────────────

func Test_stats_reads_reviews_cards_and_streak_from_real_tables(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	// 3 review trong cửa sổ week: 2 đạt (grade >= 3), 1 chưa.
	cardID := seedCard(t, db, "你好", "2026-01-02T00:00:00Z", "review", 0)
	seedReview(t, db, cardID, 3, "2026-09-27T10:00:00Z")
	seedReview(t, db, cardID, 4, "2026-09-26T10:00:00Z")
	seedReview(t, db, cardID, 1, "2026-09-25T10:00:00Z")
	// Review ngoài cửa sổ month: không được tính vào done_window.
	seedReview(t, db, cardID, 3, "2025-01-01T10:00:00Z")

	// 2 thẻ đã tới hạn (kể cả thẻ vừa ôn xong — `due_at` cũ), 1 thẻ tương lai,
	// 1 thẻ đã xoá mềm (dù `due_at` cũ vẫn không tính).
	seedCard(t, db, "谢谢", "2026-01-02T00:00:00Z", "review", 0)
	seedCard(t, db, "妈妈", "2027-01-02T00:00:00Z", "review", 0)
	seedCard(t, db, "爸爸", "2026-01-02T00:00:00Z", "review", 1)

	got, err := svc.Stats(ctx, "week")
	require.NoError(t, err)
	assert.Equal(t, 3, got.DoneWindow, "chỉ 3 review trong cửa sổ week")
	assert.Equal(t, 3, got.TotalAll, "thẻ đã xoá mềm không tính")
	assert.Equal(t, 2, got.DueNow,
		"2 thẻ `due_at` <= now; thẻ tương lai và thẻ xoá mềm không tính")
	assert.InDelta(t, 2.0/3.0, got.Accuracy, 1e-9, "2/3 review đạt")
	assert.Equal(t, "UTC", got.Timezone)
}

func Test_stats_month_window_ignores_range_boundary(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	cardID := seedCard(t, db, "你好", "2026-01-02T00:00:00Z", "review", 0)
	// 20 ngày trước: nằm trong month (30 ngày) nhưng ngoài week (7 ngày).
	seedReview(t, db, cardID, 3, "2026-09-08T10:00:00Z")

	week, err := svc.Stats(context.Background(), "week")
	require.NoError(t, err)
	assert.Equal(t, 0, week.DoneWindow)

	month, err := svc.Stats(context.Background(), "month")
	require.NoError(t, err)
	assert.Equal(t, 1, month.DoneWindow)
}

func Test_stats_on_empty_database_reports_zero_not_error(t *testing.T) {
	svc := newService(t, newTestDB(t))

	got, err := svc.Stats(context.Background(), "week")
	require.NoError(t, err)
	assert.Equal(t, 0, got.DoneWindow)
	assert.Equal(t, 0, got.TotalAll)
	assert.Equal(t, 0.0, got.Accuracy, "0/0 phải ra 0, không phải NaN")
	assert.Equal(t, 0, got.Streak)
}

// ── Streak ──────────────────────────────────────────────────────────────────

func Test_streak_uses_utc_days_not_local_timezone(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	cardID := seedCard(t, db, "你好", "2026-01-02T00:00:00Z", "review", 0)

	// 23:30 UTC ngày 27 và 00:30 UTC ngày 28 là 2 ngày UTC khác nhau, dù
	// theo giờ VN (+7) đều rơi vào cùng 1 ngày 28/09. Streak theo giờ local
	// sẽ báo 1, theo UTC (hợp đồng v1) phải báo 2.
	seedReview(t, db, cardID, 3, "2026-09-27T23:30:00Z")
	seedReview(t, db, cardID, 3, "2026-09-28T00:30:00Z")

	got, err := svc.ComputeStreak(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 2, got, "streak chuẩn UTC, không phải local")
}

func Test_streak_breaks_after_two_idle_days(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	cardID := seedCard(t, db, "你好", "2026-01-02T00:00:00Z", "review", 0)
	seedReview(t, db, cardID, 3, "2026-09-25T10:00:00Z")
	seedReview(t, db, cardID, 3, "2026-09-26T10:00:00Z")

	got, err := svc.ComputeStreak(context.Background(), fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 0, got, "hôm nay + hôm qua không học thì streak về 0")
}

// ── TopErrors ───────────────────────────────────────────────────────────────

func Test_top_errors_counts_words_across_error_notes(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	for i, wrong := range [][]string{{"the", "a"}, {"the"}, {"the", "a"}} {
		require.NoError(t, db.Exec(
			"INSERT INTO notes (card_id, text, created_at, guid) VALUES (NULL, ?, '2026-09-20', ?)",
			`ERR|{"expected":"x","transcript":"y","wrong":["`+joinWrong(wrong)+`"]}`,
			"gn"+itoaInt(i)).Error)
	}
	// Note THIEU của context khác không được tính.
	require.NoError(t, db.Exec(
		"INSERT INTO notes (card_id, text, created_at, guid) VALUES (NULL, ?, '2026-09-20', 'gn-thieu')",
		`THIEU|{"session":"2026-09-20","scores":{}}`).Error)

	got, err := svc.TopErrors(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "the", got[0].Word)
	assert.Equal(t, 3, got[0].Count)
}

// Test_top_errors_reports_card_of_the_newest_error — nợ server M5 §8.4: `TopError`
// chỉ có `{word, count}` nên "bấm lỗi → nhảy review card" không làm được.
//
// `cardId` lấy từ `notes.card_id` (cột ĐÃ CÓ từ migration 00001) chứ không
// phải từ mảng `wrong` trong JSON: mảng đó chỉ lưu TỪ, nên cấu trúc lưu KHÔNG
// cần đổi. Test này chứng minh đường đó chạy được trên DB thật.
func Test_top_errors_reports_card_of_the_newest_error(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	cardA := seedCard(t, db, "你好", "2026-01-02T00:00:00Z", "review", 0)
	cardB := seedCard(t, db, "谢谢", "2026-01-02T00:00:00Z", "review", 0)

	// Note MỚI (id lớn hơn) gắn cardA; note CŨ gắn cardB. Cùng 1 từ sai.
	insertErr := func(cardID *int64, wrong, guid string) {
		t.Helper()
		require.NoError(t, db.Exec(
			"INSERT INTO notes (card_id, text, created_at, guid) VALUES (?, ?, '2026-09-20', ?)",
			cardID, `ERR|{"expected":"x","transcript":"y","wrong":["`+wrong+`"]}`, guid).Error)
	}
	insertErr(&cardB, "ni", "gn-old")
	insertErr(&cardA, "ni", "gn-new")
	insertErr(nil, "zi", "gn-free")

	got, err := svc.TopErrors(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)

	byWord := map[string]insightapp.TopError{}
	for _, e := range got {
		byWord[e.Word] = e
	}
	ni := byWord["ni"]
	require.NotNil(t, ni.CardID, "lỗi gắn thẻ phải trả cardId")
	assert.Equal(t, cardA, *ni.CardID, "phải là thẻ của lần sai MỚI NHẤT, không phải thẻ của note cũ")
	assert.Equal(t, "你好", ni.Front)

	zi := byWord["zi"]
	assert.Nil(t, zi.CardID, "lỗi luyện tự do không gắn thẻ ⇒ cardId null")
	assert.Empty(t, zi.Front)
}

func Test_top_errors_card_id_is_null_when_card_was_soft_deleted(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	cardID := seedCard(t, db, "谢谢", "2026-01-02T00:00:00Z", "review", 1) // deleted = 1
	require.NoError(t, db.Exec(
		"INSERT INTO notes (card_id, text, created_at, guid) "+
			"VALUES (?, 'ERR|{\"wrong\":[\"xie\"]}', '2026-09-20', 'gn-del')", cardID).Error)

	got, err := svc.TopErrors(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1, "thẻ đã xoá mềm không được làm mất lần sai")
	assert.Nil(t, got[0].CardID, "thẻ đã xoá thì không nhảy review được ⇒ cardId null")
	assert.Empty(t, got[0].Front)
}

func joinWrong(words []string) string {
	out := ""
	for i, w := range words {
		if i > 0 {
			out += `","`
		}
		out += w
	}
	return out
}

func itoaInt(n int) string { return strconv.Itoa(n) }

// ── Progress ────────────────────────────────────────────────────────────────

func Test_progress_over_range_counts_completed_topics(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	var pathID int64
	require.NoError(t, db.Raw(
		"INSERT INTO roadmap_paths (slug, language, title, created_at, guid) "+
			"VALUES ('zh', 'zh', 'T', '2026-01-01', 'gp1') RETURNING id").Scan(&pathID).Error)
	var stageID int64
	require.NoError(t, db.Raw(
		"INSERT INTO roadmap_stages (path_id, slug, title, created_at, guid) "+
			"VALUES (?, 'g1', 'S', '2026-01-01', 'gs1') RETURNING id", pathID).Scan(&stageID).Error)

	// 2 node xong trong tháng này, 1 node xong từ 1 năm trước, 1 node xoá mềm.
	for i, at := range []string{"2026-09-20T10:00:00Z", "2026-09-25T10:00:00Z", "2025-01-01T10:00:00Z"} {
		require.NoError(t, db.Exec(
			"INSERT INTO roadmap_topics (stage_id, title, created_at, guid, completed_at) "+
				"VALUES (?, ?, '2026-01-01', ?, ?)", stageID, "T"+itoaInt(i), "gt"+itoaInt(i), at).Error)
	}
	require.NoError(t, db.Exec(
		"INSERT INTO roadmap_topics (stage_id, title, created_at, guid, completed_at, deleted) "+
			"VALUES (?, 'T-x', '2026-01-01', 'gt-x', '2026-09-26T10:00:00Z', 1)", stageID).Error)

	got, err := svc.ProgressOverRange(context.Background(), "2026-09-01")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Completed, "node xoá mềm không tính, node cũ ngoài khoảng không tính")
}

func Test_progress_rejects_bad_since_format(t *testing.T) {
	svc := newService(t, newTestDB(t))

	_, err := svc.ProgressOverRange(context.Background(), "01/09/2026")
	require.Error(t, err)
	var appErr *insightapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, insightapp.StatusBadRequest, appErr.Status)
}
