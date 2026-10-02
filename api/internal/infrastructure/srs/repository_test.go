package srsinfra_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	srsapp "langapp/internal/application/srs"
	srs "langapp/internal/infrastructure/srs"
)

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func Test_deck_card_crud_roundtrip_on_postgres(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	assert.NotZero(t, deck.ID, "insert phải trả về id (LastInsertId)")
	assert.Equal(t, "HSK1", deck.Name)
	assert.Equal(t, "zh", deck.Lang)
	assert.NotEmpty(t, deck.GUID, "guid rỗng sẽ đụng UNIQUE ở lần tạo thứ 2")

	decks, err := svc.ListDecks(ctx)
	require.NoError(t, err)
	require.Len(t, decks, 1)
	assert.Equal(t, deck.ID, decks[0].ID)

	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{
		Front: "你好", Back: "xin chào", Pinyin: "ni3 hao3",
	})
	require.NoError(t, err)
	assert.Equal(t, "new", card.State, "thẻ mới phải state=new để DueFilter kéo vào hàng đợi")
	assert.Equal(t, fixedNow.Add(24*time.Hour).Format(time.RFC3339), card.DueAt)

	cards, err := svc.ListCards(ctx, deck.ID)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "你好", cards[0].Front)

	// Roundtrip đọc lại từ DB: mọi cột phải khớp, không chỉ struct trong tay.
	var row struct {
		Front, Back, Pinyin, State string
	}
	require.NoError(t, db.Raw(
		"SELECT front, back, pinyin, state FROM cards WHERE id = ?", card.ID).Scan(&row).Error)
	assert.Equal(t, "你好", row.Front)
	assert.Equal(t, "ni3 hao3", row.Pinyin)
	assert.Equal(t, "new", row.State)
}

// Transaction rollback: lỗi giữa chừng phải hoàn tác CẢ 2 phần ghi. Đây là
// lý do RecordReview bọc UPDATE card + INSERT reviews trong 1 tx.
func Test_record_review_rolls_back_both_writes_on_failure(t *testing.T) {
	db := newTestDB(t)
	repo := srs.NewRepository(db)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "en")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f1", Back: "b1"})
	require.NoError(t, err)
	_ = repo

	// Cưỡng ép INSERT reviews fail bằng trigger tạm chặn — mô phỏng "ghi được
	// card nhưng lịch sử hỏng", đúng thứ transaction phải phát hiện.
	require.NoError(t, db.Exec(`
		CREATE OR REPLACE FUNCTION t_block_review() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN
		  RAISE EXCEPTION 'blocked for test'; END; $$`).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER t_block_review_ins BEFORE INSERT ON reviews
		FOR EACH ROW EXECUTE FUNCTION t_block_review()`).Error)
	t.Cleanup(func() { db.Exec("DROP TRIGGER IF EXISTS t_block_review_ins ON reviews") })

	_, err = svc.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: 3})
	require.Error(t, err, "INSERT reviews bị chặn thì cả request phải fail")

	// Rollback trọn vẹn: card KHÔNG được tăng reps, và không có review nào.
	var reps, reviews int
	require.NoError(t, db.Raw("SELECT reps FROM cards WHERE id = ?", card.ID).Scan(&reps).Error)
	assert.Equal(t, 0, reps, "UPDATE card phải bị rollback cùng INSERT reviews")
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM reviews").Scan(&reviews).Error)
	assert.Equal(t, 0, reviews)
}

func Test_record_review_writes_card_and_review_together(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)

	res, err := svc.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Reps)
	assert.Equal(t, 1, res.IntervalDays, "reps=0 + grade Good đi chuỗi fallback 1-3-7-14-30")
	assert.Equal(t, card.ID, res.CardID)

	var cardRow struct {
		Reps  int
		State string
	}
	require.NoError(t, db.Raw("SELECT reps, state FROM cards WHERE id = ?", card.ID).
		Scan(&cardRow).Error)
	reps, storedState := cardRow.Reps, cardRow.State
	assert.Equal(t, 1, reps)
	assert.Equal(t, "review", storedState, "sau 1 lần ôn state phải rời 'new'")
}

// M1 remediation: `ux_reviews_guid` UNIQUE trên cột NOT NULL DEFAULT ”. 2
// review liên tiếp cùng guid rỗng là đụng nhau. Test này chặn regression khi
// ai đó "tiện tay" bỏ NewGUID ở RecordReview.
func Test_two_consecutive_reviews_get_distinct_guids(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, err := svc.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: 3})
		require.NoError(t, err, "lần %d phải insert được, guid phải khác rỗng/khác nhau", i+1)
	}

	var guids []string
	require.NoError(t, db.Raw("SELECT guid FROM reviews ORDER BY id").Scan(&guids).Error)
	require.Len(t, guids, 2)
	assert.NotEmpty(t, guids[0], "guid rỗng sẽ đụng UNIQUE ngay lần 2")
	assert.NotEqual(t, guids[0], guids[1], "2 review phải có 2 guid khác nhau")
}

func Test_record_review_rejects_grade_outside_scale(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)

	for _, grade := range []int{0, 5, -1} {
		_, err := svc.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: grade})
		require.Error(t, err, "grade %d ngoài thang 1-4 phải bị chặn ở application", grade)
		var appErr *srsapp.Error
		assert.ErrorAs(t, err, &appErr)
		assert.Equal(t, 400, appErr.Status)
	}
}

func Test_soft_delete_filters_card_out_of_list(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	keep, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "keep", Back: "b"})
	require.NoError(t, err)
	drop, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "drop", Back: "b"})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteCard(ctx, drop.ID))

	cards, err := svc.ListCards(ctx, deck.ID)
	require.NoError(t, err)
	require.Len(t, cards, 1, "thẻ đã xoá mềm không được xuất hiện trong list")
	assert.Equal(t, keep.ID, cards[0].ID)

	// Tombstone phải còn trong DB (sync cần nó để lan truyền xoá).
	var deleted int
	require.NoError(t, db.Raw("SELECT deleted FROM cards WHERE id = ?", drop.ID).Scan(&deleted).Error)
	assert.Equal(t, 1, deleted, "xoá mềm = deleted=1, KHÔNG phải DELETE hẳn")
}

func Test_soft_delete_deck_cascades_to_cards(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	_, err = svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteDeck(ctx, deck.ID))

	decks, err := svc.ListDecks(ctx)
	require.NoError(t, err)
	assert.Empty(t, decks, "deck đã xoá mềm không được xuất hiện trong list")

	// ListCards kiểm tra deck cha trước nên trả 404 sau khi deck đã xoá — kiểm
	// cascade ở mức row: thẻ phải mang tombstone, không bị xoá hẳn (sync cần).
	var counts struct {
		Total   int
		Deleted int
	}
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE deleted = 1) AS deleted FROM cards WHERE deck_id = ?",
		deck.ID).Scan(&counts).Error)
	assert.Equal(t, 1, counts.Total, "thẻ vẫn phải còn trong DB (tombstone, không DELETE hẳn)")
	assert.Equal(t, 1, counts.Deleted, "thẻ của deck đã xoá cũng phải mang tombstone")
}

// Trigger `langapp_touch_updated_at` là cơ sở của merge LWW: UPDATE mà không
// set updated_at phải được trigger chạm sang mốc hiện tại.
func Test_updated_at_trigger_touches_on_card_update(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)

	// Mốc tạo là now cố định của test (2026-09-28) — khác mốc thật của trigger.
	before, err := readUpdatedAt(t, db, "cards", card.ID)
	require.NoError(t, err)
	assert.Equal(t, fixedNow.Format(time.RFC3339), before)

	back := "b2"
	_, err = svc.UpdateCard(ctx, card.ID, srsapp.CardPatch{Back: &back})
	require.NoError(t, err)

	after, err := readUpdatedAt(t, db, "cards", card.ID)
	require.NoError(t, err)
	assert.NotEqual(t, before, after,
		"UPDATE card phải để trigger chạm updated_at (nếu GORM tự ghi hoặc ta Omit sai, mốc sẽ không đổi)")
}

func readUpdatedAt(t *testing.T, db *gorm.DB, table string, id int64) (string, error) {
	t.Helper()
	var v string
	return v, db.Raw("SELECT updated_at FROM "+table+" WHERE id = ?", id).Scan(&v).Error
}

func Test_due_cards_includes_new_state_and_sorts_by_due(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	// 3 thẻ: 1 quá hạn, 1 mới (due_at tương lai nhưng state=new), 1 tương lai
	// đã ôn (không đến hạn).
	overdue, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "overdue", Back: "b"})
	require.NoError(t, err)
	fresh, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "fresh", Back: "b"})
	require.NoError(t, err)
	future, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "future", Back: "b"})
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		"UPDATE cards SET due_at = ?, state = 'review' WHERE id = ?",
		fixedNow.Add(-48*time.Hour).Format(time.RFC3339), overdue.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE cards SET due_at = ?, state = 'review' WHERE id = ?",
		fixedNow.Add(48*time.Hour).Format(time.RFC3339), future.ID).Error)

	due, err := svc.DueCards(ctx, deck.ID)
	require.NoError(t, err)
	ids := make([]int64, 0, len(due))
	for _, c := range due {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []int64{overdue.ID, fresh.ID}, ids,
		"thẻ mới phải vào hàng đợi dù due_at còn tương lai; thẻ tương lai đã ôn thì không")
}

func Test_find_deck_reports_existence_for_roadmap_port(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)

	info, err := svc.FindDeck(ctx, deck.ID)
	require.NoError(t, err)
	assert.True(t, info.Exists)
	assert.Equal(t, "zh", info.Lang)

	missing, err := svc.FindDeck(ctx, 999999)
	require.NoError(t, err, "deck không tồn tại là kết quả hợp lệ, KHÔNG phải lỗi hệ thống")
	assert.False(t, missing.Exists)
	assert.Empty(t, missing.Lang)
}

func Test_create_card_reuses_soft_deleted_row_instead_of_duplicating(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	first, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "x", Back: "b1"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteCard(ctx, first.ID))

	again, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "x", Back: "b2"})
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "phải hồi sinh tombstone, không tạo row thứ 2 cùng front")
	assert.Equal(t, first.GUID, again.GUID, "giữ guid để 2 máy hội tụ về cùng 1 thẻ")
	assert.Equal(t, "b2", again.Back)

	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM cards WHERE front = 'x'").Scan(&n).Error)
	assert.Equal(t, 1, n, "mọi lần xoá/tạo lại vẫn chỉ có 1 row cho front này")
}

func Test_duplicate_front_in_live_deck_returns_conflict(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	_, err = svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "dup", Back: "b"})
	require.NoError(t, err)

	_, err = svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "dup", Back: "b2"})
	require.Error(t, err)
	var appErr *srsapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 409, appErr.Status, "vi phạm ux_cards_deck_front phải là 409 chứ không phải 500")
}

func Test_card_not_found_returns_404(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	_, err := svc.CreateCard(ctx, 999999, srsapp.CardInput{Front: "f", Back: "b"})
	require.Error(t, err)
	var appErr *srsapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 404, appErr.Status)
	assert.Equal(t, "không tìm thấy deck", appErr.Message)
}

func Test_set_card_tone_persists_and_clears(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b", Pinyin: "ni3"})
	require.NoError(t, err)

	tone := "ní"
	updated, err := svc.SetCardTone(ctx, card.ID, &tone)
	require.NoError(t, err)
	require.NotNil(t, updated.Tone)
	assert.Equal(t, "ní", *updated.Tone)

	var stored *string
	require.NoError(t, db.Raw("SELECT tone FROM cards WHERE id = ?", card.ID).Scan(&stored).Error)
	require.NotNil(t, stored)
	assert.Equal(t, "ní", *stored)

	cleared, err := svc.SetCardTone(ctx, card.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.Tone)
	require.NoError(t, db.Raw("SELECT tone FROM cards WHERE id = ?", card.ID).Scan(&stored).Error)
	assert.Nil(t, stored, "xoá tone phải ghi SQL NULL, không phải chuỗi rỗng")
}

func Test_review_history_survives_card_soft_delete(t *testing.T) {
	db := newTestDB(t)
	svc, _ := newService(t, db, fixedNow)
	ctx := context.Background()

	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "f", Back: "b"})
	require.NoError(t, err)
	_, err = svc.RecordReview(ctx, srsapp.ReviewInput{CardID: card.ID, Grade: 3})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteCard(ctx, card.ID))

	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM reviews WHERE card_id = ?", card.ID).Scan(&n).Error)
	assert.Equal(t, 1, n,
		"reviews KHÔNG có cột deleted — lịch sử ôn là append-only, mất là mất vĩnh viễn")
}

func Test_repository_returns_sentinel_not_found(t *testing.T) {
	db := newTestDB(t)
	repo := srs.NewRepository(db)

	_, err := repo.DeckByID(context.Background(), 999999)
	assert.True(t, errors.Is(err, srsapp.ErrNotFound),
		"repository trả sentinel, use case mới bọc thành 404 tiếng Việt")
}
