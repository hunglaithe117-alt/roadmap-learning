package contentinfra_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	app "langapp/internal/application/content"
)

// ── Tra từ điển ─────────────────────────────────────────────────────────────

// Hạn chế CẦN GHI RÕ (STACK-V2-PLAN §4.2): `to_tsvector('simple', '你好')` gộp
// CẢ chuỗi Hán làm MỘT token, nên tra Hán đa ký tự phải đi đường ILIKE. Đây
// KHÔNG phải hồi quy so với SQLite FTS5 v1 (M1 remediation đã đo: `MATCH '你'`
// cũng trả rỗng). Test dưới đây khẳng định CẢ 3 kiểu tra đều ra kết quả.
func Test_search_dict_finds_cjk_single_multi_and_pinyin(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES ('你好', 'ni3 hao3', 'xin chào')").Error)
	require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES ('谢谢', 'xie4 xie5', 'cảm ơn')").Error)

	single, err := svc.SearchDict(ctx, "你", 10)
	require.NoError(t, err)
	require.Len(t, single, 1, "tra 1 ký tự Hán phải ra kết quả qua ILIKE")
	assert.Equal(t, "你好", single[0].Hanzi)

	multi, err := svc.SearchDict(ctx, "你好", 10)
	require.NoError(t, err)
	require.Len(t, multi, 1, "tra Hán đa ký tự phải ra kết quả")

	pinyin, err := svc.SearchDict(ctx, "xie4", 10)
	require.NoError(t, err)
	require.Len(t, pinyin, 1, "tra pinyin qua tsvector phải ra chuỗi Hán")
	assert.Equal(t, "谢谢", pinyin[0].Hanzi)
}

func Test_search_dict_rejects_wildcard_only_input(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	for _, h := range []string{"你好", "谢谢", "妈妈"} {
		require.NoError(t, db.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES (?, 'py', 'nghia')", h).Error)
	}

	// `q` là input người dùng: nội suy thô `%` thành `ILIKE '%%%'` → khớp mọi
	// row, và 1 ký tự đó sẽ trả về toàn bảng (finding F5 của M1).
	// `"   "` không có ở đây: `SearchDict` trim rồi trả 400 — đó là validate
	// của use case, đã có test riêng. Ở đây chỉ kiểm hàm SQL không nổ.
	for _, q := range []string{"%", "_", "%%%", "\\"} {
		got, err := svc.SearchDict(context.Background(), q, 10)
		require.NoError(t, err, "q=%q", q)
		assert.Empty(t, got, "q=%q phải trả 0 dòng, không phải toàn bảng", q)
	}
}

func Test_search_english_finds_latin_prefix(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	require.NoError(t, db.Exec(
		"INSERT INTO en_dict (lang, term, reading, gloss) VALUES ('en', 'abandon', '/əˈbændən/', 'từ bỏ')").Error)

	got, err := svc.SearchEnglish(context.Background(), "aband", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "abandon", got[0].Term)
}

// Nợ #1 đóng ở M3: tra chính xác phải dùng `lower(term) = lower(?)`.
// `COLLATE NOCASE` là cú pháp SQLite và **lỗi cú pháp trong Postgres** —
// index `idx_en_dict_term_lower` được tạo ở migration 00002 đúng cho câu này.
func Test_lookup_english_exact_is_case_insensitive_via_lower_index(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	require.NoError(t, db.Exec(
		"INSERT INTO en_dict (lang, term, reading, gloss) VALUES ('en', 'Candid', '/ˈkændɪd/', 'thẳng thắn')").Error)

	got, err := svc.LookupStress(context.Background(), "candid")
	require.NoError(t, err)
	assert.Equal(t, "Candid", got.Term, "phải trả về headword gốc trong DB")
	assert.Equal(t, "dict", got.Source)

	// EXPLAIN phải chứng minh câu truy vấn match được `idx_en_dict_term_lower`.
	// Bảng test chỉ 1 dòng nhưng Postgres vẫn chọn Bitmap Index Scan vì
	// `lower(term) = lower($1)` khớp biểu thức index — đã verify plan thật, KHÔNG
	// cần `enable_seqscan = off`.
	// EXPLAIN trả nhiều dòng (1 dòng cho mỗi node của plan) — đọc hết rồi
	// nối lại, nếu chỉ đọc dòng đầu sẽ bỏ sót dòng chứa tên index.
	rows, err := db.Raw("EXPLAIN SELECT id FROM en_dict WHERE lower(term) = lower('candid')").Rows()
	require.NoError(t, err)
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	plan := strings.ToLower(strings.Join(lines, " | "))
	assert.Contains(t, plan, "idx_en_dict_term_lower",
		"tra chính xác phải dùng được index lower(term), plan thực tế: %s", plan)
}

func Test_search_english_rejects_empty_query(t *testing.T) {
	svc := newService(t, newTestDB(t))

	_, err := svc.SearchEnglish(context.Background(), "  ", 10)
	require.Error(t, err)
}

// ── Import HSK ──────────────────────────────────────────────────────────────

func Test_import_hsk_is_idempotent_over_two_runs(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	first, err := svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	assert.Equal(t, 36, first.CardsAdded, "HSK1 = 36 từ")
	assert.Equal(t, 36, first.DictAdded)
	assert.Equal(t, 36, first.CardsTotal)

	second, err := svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	assert.Equal(t, 0, second.CardsAdded, "import lần 2 KHÔNG được nhân bản")
	assert.Equal(t, 0, second.DictAdded)
	assert.Equal(t, 36, second.CardsTotal)

	var cards, dict int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM cards").Scan(&cards).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM dict").Scan(&dict).Error)
	assert.EqualValues(t, 36, cards)
	assert.EqualValues(t, 36, dict)
}

// 2 máy cùng import HSK1 phải cho cùng guid → merge khớp, không nhân đôi thẻ.
func Test_import_hsk_uses_stable_guids_for_two_machines(t *testing.T) {
	local, peer := twoMachines(t)
	localSvc := newService(t, local)
	peerSvc := newService(t, peer)

	_, err := localSvc.ImportHSK(context.Background(), app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	_, err = peerSvc.ImportHSK(context.Background(), app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)

	var localGUIDs, peerGUIDs []string
	require.NoError(t, local.Raw("SELECT guid FROM cards ORDER BY front").Scan(&localGUIDs).Error)
	require.NoError(t, peer.Raw("SELECT guid FROM cards ORDER BY front").Scan(&peerGUIDs).Error)
	require.Len(t, localGUIDs, 36)
	assert.Equal(t, localGUIDs, peerGUIDs,
		"guid seed phải ỔN ĐỊNH theo (deck, lang, front) — 2 máy cùng seed cho cùng guid")
}

func Test_import_hsk_rejects_deck_name_different_from_level(t *testing.T) {
	svc := newService(t, newTestDB(t))

	_, err := svc.ImportHSK(context.Background(), app.ImportInput{Level: "HSK1", Deck: "Từ của tôi"})
	require.Error(t, err)
	var appErr *app.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, app.StatusBadRequest, appErr.Status)
}

func Test_import_hsk_rejects_deck_with_wrong_language(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Exec(
		"INSERT INTO decks (name, lang, created_at, guid) VALUES ('HSK1', 'en', '2026-01-01', 'g1')").Error)
	svc := newService(t, db)

	_, err := svc.ImportHSK(context.Background(), app.ImportInput{Level: "HSK1"})
	require.Error(t, err)
	var appErr *app.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, app.StatusBadRequest, appErr.Status)
}

func Test_import_hsk_revives_soft_deleted_card_in_place(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	first, err := svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	// User xoá thẻ "你好" trong deck HSK1 rồi import lại: phải HỒI SINH
	// chính row đó (giữ id + guid) chứ không tạo row thứ 2 — nếu không, 2 máy
	// sẽ có 2 thẻ khác id cùng front và lần sync sau nhân đôi.
	var cardID int64
	require.NoError(t, db.Raw("SELECT id FROM cards WHERE front = '你好' AND deck_id = ?",
		first.DeckID).Scan(&cardID).Error)
	require.NoError(t, db.Exec("UPDATE cards SET deleted = 1 WHERE id = ?", cardID).Error)

	second, err := svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	assert.Equal(t, 1, second.CardsAdded, "thẻ hồi sinh đếm là 1 thẻ mới")
	assert.Equal(t, 36, second.CardsTotal, "tổng thẻ sống không đổi")

	var live int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM cards WHERE front = '你好' AND deleted = 0").
		Scan(&live).Error)
	assert.EqualValues(t, 1, live, "không được có 2 row cùng front")

	var revivedID int64
	require.NoError(t, db.Raw("SELECT id FROM cards WHERE front = '你好' AND deleted = 0").
		Scan(&revivedID).Error)
	assert.Equal(t, cardID, revivedID, "phải hồi sinh chính row cũ, giữ id")
}

func Test_import_hsk_does_not_overwrite_user_edited_card(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	first, err := svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	// User sửa nghĩa của thẻ "你好" rồi import lại — nội dung user phải sống.
	require.NoError(t, db.Exec("UPDATE cards SET back = 'nghĩa của tôi' WHERE front = '你好'").Error)

	_, err = svc.ImportHSK(ctx, app.ImportInput{Level: "HSK1"})
	require.NoError(t, err)

	var back string
	require.NoError(t, db.Raw("SELECT back FROM cards WHERE front = '你好' AND deck_id = ?",
		first.DeckID).Scan(&back).Error)
	assert.Equal(t, "nghĩa của tôi", back, "import lại không được đè nội dung user sửa")
}

func Test_import_hsk_rejects_unknown_level(t *testing.T) {
	svc := newService(t, newTestDB(t))

	_, err := svc.ImportHSK(context.Background(), app.ImportInput{Level: "HSK9"})
	require.Error(t, err)
	var appErr *app.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, app.StatusBadRequest, appErr.Status)
}

func Test_import_hsk_levels_2_to_4_load_real_vocabulary(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	for level, min := range map[string]int{"HSK2": 100, "HSK3": 200, "HSK4": 300} {
		res, err := svc.ImportHSK(ctx, app.ImportInput{Level: level})
		require.NoError(t, err, "level %s", level)
		assert.GreaterOrEqual(t, res.CardsAdded, min, "seed %s phải có dữ liệu thật", level)
	}
}

// ── Seed tiếng Anh ──────────────────────────────────────────────────────────

func Test_seed_english_twice_is_idempotent(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	first, err := svc.SeedEnglish(ctx)
	require.NoError(t, err)
	assert.Equal(t, 26, first.EnDict, "từ điển tích hợp có 26 headword")
	assert.Positive(t, first.PVOAdded)
	assert.Positive(t, first.TMRNDAdded)

	second, err := svc.SeedEnglish(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, second.PVOAdded, "seed lần 2 không được thêm thẻ")
	assert.Equal(t, 0, second.TMRNDAdded)
	assert.Equal(t, 26, second.EnDict)

	var decks, cards, enDict int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM decks WHERE lang = 'en'").Scan(&decks).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM cards").Scan(&cards).Error)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM en_dict").Scan(&enDict).Error)
	assert.EqualValues(t, 2, decks, "PVO + TMRND")
	assert.EqualValues(t, 26, enDict)
	assert.Equal(t, int64(first.PVOAdded+first.TMRNDAdded), cards)
}

func Test_seed_english_keeps_user_edited_cards(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	first, err := svc.SeedEnglish(ctx)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		"UPDATE cards SET back = 'nghĩa của tôi' WHERE front = 'make progress'").Error)

	second, err := svc.SeedEnglish(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, second.PVOAdded)

	var back string
	require.NoError(t, db.Raw("SELECT back FROM cards WHERE front = 'make progress'").Scan(&back).Error)
	assert.Equal(t, "nghĩa của tôi", back)
	_ = first
}

func Test_seed_english_uses_stable_guids_for_two_machines(t *testing.T) {
	local, peer := twoMachines(t)
	_, err := newService(t, local).SeedEnglish(context.Background())
	require.NoError(t, err)
	_, err = newService(t, peer).SeedEnglish(context.Background())
	require.NoError(t, err)

	var a, b []string
	require.NoError(t, local.Raw("SELECT guid FROM cards ORDER BY front").Scan(&a).Error)
	require.NoError(t, peer.Raw("SELECT guid FROM cards ORDER BY front").Scan(&b).Error)
	assert.Equal(t, a, b, "guid thẻ seed phải ổn định ở cả 2 máy")
}

// ── Thanh điệu ──────────────────────────────────────────────────────────────

func Test_set_tone_writes_column_and_touch_trigger_fires(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	deckID := seedDeck(t, db, "HSK1", "zh")
	cardID := seedCard(t, db, deckID, "你好", "ni3 hao3", "2026-01-01T00:00:00Z")

	got, err := svc.SetTone(ctx, cardID, "3 3")
	require.NoError(t, err)
	assert.Equal(t, "3 3", got)

	var tone, updatedAt string
	require.NoError(t, db.Raw("SELECT tone, updated_at FROM cards WHERE id = ?", cardID).
		Row().Scan(&tone, &updatedAt))
	assert.Equal(t, "3 3", tone)
	assert.NotEqual(t, "2026-01-01T00:00:00Z", updatedAt,
		"ghi tone phải chạm updated_at để peer nhận biết thẻ đổi")
}

func Test_set_tone_rejects_invalid_pattern_before_writing(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	deckID := seedDeck(t, db, "HSK1", "zh")
	cardID := seedCard(t, db, deckID, "你好", "ni3 hao3", "2026-01-01T00:00:00Z")

	_, err := svc.SetTone(context.Background(), cardID, "3 x")
	require.Error(t, err)

	var tone *string
	require.NoError(t, db.Raw("SELECT tone FROM cards WHERE id = ?", cardID).Scan(&tone).Error)
	assert.Nil(t, tone, "pattern sai không được ghi rác xuống DB")
}

func Test_set_tone_rejects_missing_card(t *testing.T) {
	svc := newService(t, newTestDB(t))

	_, err := svc.SetTone(context.Background(), 999999, "3")
	require.Error(t, err)
	var appErr *app.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, app.StatusNotFound, appErr.Status)
}

// ── THIEU ───────────────────────────────────────────────────────────────────

func Test_append_thieu_writes_note_with_reserved_prefix(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)

	scores := map[string]int{"A": 3, "B": 4, "C": 5, "D": 2, "E": 3, "F": 4, "G": 5, "H": 2}
	got, err := svc.AppendThieu(context.Background(), app.ThieuInput{
		Session: "2026-09-28", Scores: scores, Note: "buổi sáng",
	})
	require.NoError(t, err)
	assert.Equal(t, 28.0/8, got.Average)

	var text string
	var cardID *int64
	require.NoError(t, db.Raw("SELECT text, card_id FROM notes WHERE id = ?", got.ID).
		Row().Scan(&text, &cardID))
	assert.True(t, strings.HasPrefix(text, "THIEU|"),
		"phải dùng prefix reserved THIEU| để không lẫn với sổ lỗi ERR|")
	assert.Nil(t, cardID, "checklist không gắn thẻ")
}

func Test_list_thieu_reads_back_what_was_written(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db)
	ctx := context.Background()
	scores := map[string]int{"A": 5, "B": 5, "C": 5, "D": 5, "E": 5, "F": 5, "G": 5, "H": 5}

	_, err := svc.AppendThieu(ctx, app.ThieuInput{Session: "2026-09-27", Scores: scores})
	require.NoError(t, err)
	_, err = svc.AppendThieu(ctx, app.ThieuInput{Session: "2026-09-28", Scores: scores})
	require.NoError(t, err)

	list, err := svc.ListThieu(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "2026-09-28", list[0].Session, "mới nhất trước")
	assert.Equal(t, 5.0, list[0].Average)
}

// ── Bài đọc + nét chữ + chunking ────────────────────────────────────────────

func Test_reader_articles_are_bundled_and_filterable(t *testing.T) {
	svc := newService(t, newTestDB(t))
	ctx := context.Background()

	all, err := svc.GetReaderArticles(ctx, "", "")
	require.NoError(t, err)
	assert.Len(t, all, 8, "8 bài bundle (2 bài × HSK1/HSK2/A1/A2)")

	zh, err := svc.GetReaderArticles(ctx, "HSK1", "")
	require.NoError(t, err)
	assert.Len(t, zh, 2)

	one, err := svc.GetReaderArticles(ctx, "", "a1-1")
	require.NoError(t, err)
	require.Len(t, one, 1)
	assert.NotEmpty(t, one[0].Text)
	assert.NotEmpty(t, one[0].Source, "mỗi bài phải ghi rõ nguồn")
}

func Test_stroke_data_lookup_matches_level(t *testing.T) {
	svc := newService(t, newTestDB(t))
	ctx := context.Background()

	_, detail, err := svc.GetStrokes(ctx, "HSK1", "好")
	require.NoError(t, err)
	assert.Equal(t, "好", detail.Hanzi)
	assert.Equal(t, 6, detail.StrokeCount)
	assert.Equal(t, "hǎo", detail.PinyinMarks, "dấu thanh phải do domain/content vẽ")
	for i, step := range detail.Strokes {
		assert.Equal(t, i+1, step.Order)
		assert.NotEmpty(t, step.Code)
	}

	idx, _, err := svc.GetStrokes(ctx, "HSK1", "")
	require.NoError(t, err)
	assert.Len(t, idx, 12, "index level HSK1 có 12 chữ")

	_, _, err = svc.GetStrokes(ctx, "HSK2", "好")
	require.Error(t, err, "chữ thuộc level khác không được trả về")
}

func Test_chunk_splits_content_and_function_words(t *testing.T) {
	svc := newService(t, newTestDB(t))

	got, err := svc.Chunk(context.Background(), "I want to make progress")
	require.NoError(t, err)
	require.Len(t, got, 5)
	assert.Equal(t, "function", got[0].Kind)
	assert.Equal(t, "content", got[4].Kind)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func seedDeck(t *testing.T, db *gorm.DB, name, lang string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO decks (name, lang, created_at, guid, updated_at)
		VALUES (?, ?, '2026-01-01T00:00:00Z', 'gd-' || ?, '2026-01-01T00:00:00Z')
		RETURNING id`, name, lang, name).Scan(&id).Error)
	return id
}

func seedCard(t *testing.T, db *gorm.DB, deckID int64, front, pinyin, createdAt string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO cards
		(deck_id, front, back, pinyin, due_at, state, created_at, guid, updated_at)
		VALUES (?, ?, 'b', ?, '2026-01-02T00:00:00Z', 'new', ?, 'gc-' || ?, ?)
		RETURNING id`, deckID, front, pinyin, createdAt, front, createdAt).Scan(&id).Error)
	return id
}
