package srs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo là repository trong bộ nhớ. Tầng application không được import
// tầng hạ tầng (luật DDD §2), nên test ở đây dùng test double; test repository
// thật nằm ở package infrastructure (Postgres thật).
type fakeRepo struct {
	decks   map[int64]Deck
	cards   map[int64]Card
	reviews []Review
	nextID  int64
	failOn  string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{decks: map[int64]Deck{}, cards: map[int64]Card{}, nextID: 1}
}

var errFake = errors.New("lỗi giả lập")

func (f *fakeRepo) id() int64 { f.nextID++; return f.nextID }

func (f *fakeRepo) CreateDeck(_ context.Context, _ Tx, d *Deck) error {
	if f.failOn == "CreateDeck" {
		return errFake
	}
	d.ID = f.id()
	f.decks[d.ID] = *d
	return nil
}

func (f *fakeRepo) ListDecks(context.Context) ([]Deck, error) {
	out := []Deck{}
	for _, d := range f.decks {
		if d.Deleted == 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRepo) DeckByID(_ context.Context, id int64) (Deck, error) {
	if d, ok := f.decks[id]; ok && d.Deleted == 0 {
		return d, nil
	}
	return Deck{}, ErrNotFound
}

// DecksByIDs mô phỏng `WHERE id IN (...)`: 1 lần gọi cho nhiều id, giống hệt
// gọi DeckByID mỗi id (dataloader `Stage.deck` của M4 dựa vào đúng điều đó).
func (f *fakeRepo) DecksByIDs(_ context.Context, ids []int64) ([]Deck, error) {
	out := []Deck{}
	for _, id := range ids {
		if d, ok := f.decks[id]; ok && d.Deleted == 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRepo) SoftDeleteDeck(_ context.Context, _ Tx, id int64) error {
	d := f.decks[id]
	d.Deleted = 1
	f.decks[id] = d
	for cid, c := range f.cards {
		if c.DeckID == id {
			c.Deleted = 1
			f.cards[cid] = c
		}
	}
	return nil
}

func (f *fakeRepo) CreateCard(_ context.Context, _ Tx, c *Card) error {
	if f.failOn == "CreateCard" {
		return errFake
	}
	c.ID = f.id()
	f.cards[c.ID] = *c
	return nil
}

func (f *fakeRepo) CardByID(_ context.Context, id int64) (Card, error) {
	if c, ok := f.cards[id]; ok && c.Deleted == 0 {
		return c, nil
	}
	return Card{}, ErrNotFound
}

func (f *fakeRepo) ListCards(_ context.Context, deckID int64) ([]Card, error) {
	out := []Card{}
	for _, c := range f.cards {
		if c.DeckID == deckID && c.Deleted == 0 {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRepo) UpdateCard(_ context.Context, _ Tx, c *Card) error {
	if f.failOn == "UpdateCard" {
		return errFake
	}
	f.cards[c.ID] = *c
	return nil
}

func (f *fakeRepo) SoftDeleteCard(_ context.Context, _ Tx, id int64) error {
	c := f.cards[id]
	c.Deleted = 1
	f.cards[id] = c
	return nil
}

func (f *fakeRepo) CreateReview(_ context.Context, _ Tx, r *Review) error {
	if f.failOn == "CreateReview" {
		return errFake
	}
	r.ID = f.id()
	f.reviews = append(f.reviews, *r)
	return nil
}

// fakeUow rollback bằng snapshot: 1 lần ghi lỗi phải không để lại thay đổi.
type fakeUow struct {
	repo      *fakeRepo
	opens     int
	rollbacks int
}

func (u *fakeUow) Do(_ context.Context, fn func(Tx) error) error {
	u.opens++
	snapshot := u.repo.clone()
	if err := fn(nil); err != nil {
		u.repo.restore(snapshot)
		u.rollbacks++
		return err
	}
	return nil
}

func (f *fakeRepo) clone() *fakeRepo {
	return &fakeRepo{
		decks: copyMap(f.decks), cards: copyMap(f.cards),
		reviews: append([]Review(nil), f.reviews...), nextID: f.nextID, failOn: f.failOn,
	}
}

func (f *fakeRepo) restore(s *fakeRepo) {
	f.decks, f.cards, f.reviews, f.nextID = s.decks, s.cards, s.reviews, s.nextID
}

func copyMap[V any](m map[int64]V) map[int64]V {
	out := make(map[int64]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

var fixedClock = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func newTestService() (*Service, *fakeRepo, *fakeUow) {
	repo := newFakeRepo()
	uow := &fakeUow{repo: repo}
	return NewService(repo, uow, func() time.Time { return fixedClock }), repo, uow
}

func seedDeckWithCard(t *testing.T, svc *Service) (Deck, Card) {
	t.Helper()
	ctx := context.Background()
	deck, err := svc.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, deck.ID, CardInput{Front: "你好", Back: "xin chào"})
	require.NoError(t, err)
	return deck, card
}

// ── Deck ────────────────────────────────────────────────────────────────────

func Test_create_deck_normalises_lang_variants(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	for _, c := range []struct{ in, want string }{
		{"zh", "zh"}, {"ZH", "zh"}, {"zh-CN", "zh"}, {"en_us", "en"}, {"", "zh"},
	} {
		got, err := svc.CreateDeck(ctx, "Deck", c.in)
		require.NoError(t, err, "lang %q", c.in)
		assert.Equal(t, c.want, got.Lang, "lang %q phải quy về zh|en", c.in)
	}
	_, err := svc.CreateDeck(ctx, "Deck", "fr")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Equal(t, "lang chỉ nhận zh hoặc en", appErr.Message)
}

func Test_create_deck_requires_name(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.CreateDeck(context.Background(), "   ", "zh")
	require.Error(t, err)
	assert.Equal(t, "thiếu tên deck", err.Error())
}

func Test_create_deck_generates_non_empty_guid(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		d, err := svc.CreateDeck(ctx, "Deck", "zh")
		require.NoError(t, err)
		assert.NotEmpty(t, d.GUID)
		assert.False(t, seen[d.GUID], "guid trùng lần %d", i)
		seen[d.GUID] = true
	}
	assert.Len(t, repo.decks, 50)
}

// ── Card ────────────────────────────────────────────────────────────────────

func Test_create_card_sets_new_state_and_plus_24h_due(t *testing.T) {
	svc, _, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)
	assert.Equal(t, "new", card.State)
	assert.Equal(t, fixedClock.Add(24*time.Hour).Format(time.RFC3339), card.DueAt,
		"thẻ mới đẩy hạn 24h nhưng vẫn vào hàng đợi nhờ state='new'")
	assert.NotEmpty(t, card.GUID)
}

func Test_create_card_rejects_blank_faces_and_unknown_deck(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)

	_, err = svc.CreateCard(ctx, deck.ID, CardInput{Front: "  ", Back: "b"})
	require.Error(t, err)
	assert.Equal(t, "thiếu mặt trước/sau của thẻ", err.Error())

	_, err = svc.CreateCard(ctx, 999, CardInput{Front: "f", Back: "b"})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
	assert.Equal(t, "không tìm thấy deck", appErr.Message)
}

func Test_update_card_cannot_blank_faces(t *testing.T) {
	svc, _, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)
	empty := "  "
	_, err := svc.UpdateCard(context.Background(), card.ID, CardPatch{Front: &empty})
	require.Error(t, err)
	assert.Equal(t, "mặt trước/sau không được rỗng", err.Error())
}

// ── Review ──────────────────────────────────────────────────────────────────

func Test_record_review_uses_domain_schedule(t *testing.T) {
	svc, _, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)

	res, err := svc.RecordReview(context.Background(), ReviewInput{CardID: card.ID, Grade: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Reps)
	assert.Equal(t, 1, res.IntervalDays, "reps=0 + grade 3 → chuỗi fallback 1-3-7-14-30")
	assert.True(t, res.Fallback)
	assert.Equal(t, fixedClock.Add(24*time.Hour).Format(time.RFC3339), res.DueAt)
}

func Test_record_review_grade_again_counts_lapse_and_shortens_interval(t *testing.T) {
	svc, repo, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)

	// reps>=3 để rơi vào FSRS-lite, nơi grade Again mới khác rõ rệt.
	repo.cards[card.ID] = Card{
		ID: card.ID, DeckID: card.DeckID, Front: "f", Back: "b", State: "review",
		Reps: 5, Stability: 10, Difficulty: 5, DueAt: fixedClock.Format(time.RFC3339),
	}
	res, err := svc.RecordReview(context.Background(), ReviewInput{CardID: card.ID, Grade: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, res.IntervalDays, "bấm Quên luôn hẹn lại 1 ngày")
	assert.Equal(t, 6, res.Reps)
	assert.Less(t, repo.cards[card.ID].Stability, 10.0, "stability phải giảm")
}

func Test_record_review_rejects_grade_outside_scale(t *testing.T) {
	svc, _, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)
	for _, g := range []int{0, 5, -1} {
		_, err := svc.RecordReview(context.Background(), ReviewInput{CardID: card.ID, Grade: g})
		require.Error(t, err, "grade %d", g)
		var appErr *Error
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, StatusBadRequest, appErr.Status)
		assert.Equal(t, "grade phải từ 1 đến 4 (1=Quên, 4=Dễ)", appErr.Message)
	}
}

func Test_record_review_writes_distinct_non_empty_guids(t *testing.T) {
	svc, repo, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.RecordReview(ctx, ReviewInput{CardID: card.ID, Grade: 3})
		require.NoError(t, err)
	}
	require.Len(t, repo.reviews, 3)
	seen := map[string]bool{}
	for _, r := range repo.reviews {
		assert.NotEmpty(t, r.GUID, "guid rỗng sẽ đụng ux_reviews_guid ngay lần 2")
		assert.False(t, seen[r.GUID], "guid trùng")
		seen[r.GUID] = true
		assert.Equal(t, fixedClock.Format(time.RFC3339), r.ReviewedAt)
	}
}

func Test_record_review_rolls_back_card_when_review_insert_fails(t *testing.T) {
	svc, repo, uow := newTestService()
	_, card := seedDeckWithCard(t, svc)
	repo.failOn = "CreateReview"

	_, err := svc.RecordReview(context.Background(), ReviewInput{CardID: card.ID, Grade: 3})
	require.Error(t, err)
	assert.Equal(t, 0, repo.cards[card.ID].Reps,
		"UPDATE card phải bị rollback cùng INSERT reviews — nếu không, thẻ lên lịch mà không có lịch sử")
	assert.Positive(t, uow.rollbacks)
}

func Test_record_review_missing_card_returns_404(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.RecordReview(context.Background(), ReviewInput{CardID: 999, Grade: 3})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
	assert.Equal(t, "không tìm thấy thẻ", appErr.Message)
}

// ── DueCards ────────────────────────────────────────────────────────────────

func Test_due_cards_include_new_state_even_when_due_in_future(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)

	// Thẻ mới: due_at = +24h (tương lai) nhưng state='new' → phải vào hàng đợi.
	fresh, err := svc.CreateCard(ctx, deck.ID, CardInput{Front: "fresh", Back: "b"})
	require.NoError(t, err)
	// Thẻ đã ôn, hẹn 3 ngày nữa → không đến hạn.
	future, err := svc.CreateCard(ctx, deck.ID, CardInput{Front: "future", Back: "b"})
	require.NoError(t, err)
	repo.cards[future.ID] = Card{ID: future.ID, DeckID: deck.ID, Front: "future", Back: "b",
		State: "review", DueAt: fixedClock.Add(72 * time.Hour).Format(time.RFC3339)}

	due, err := svc.DueCards(ctx, deck.ID)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, fresh.ID, due[0].ID,
		"lọc chỉ theo due_at sẽ giấu thẻ mới khỏi hàng đợi — đó là lý do v1 dùng OR state='new'")
}

func Test_due_cards_sort_overdue_before_new(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	deck, err := svc.CreateDeck(ctx, "D", "zh")
	require.NoError(t, err)

	fresh, err := svc.CreateCard(ctx, deck.ID, CardInput{Front: "fresh", Back: "b"})
	require.NoError(t, err)
	overdue, err := svc.CreateCard(ctx, deck.ID, CardInput{Front: "overdue", Back: "b"})
	require.NoError(t, err)
	repo.cards[overdue.ID] = Card{ID: overdue.ID, DeckID: deck.ID, Front: "overdue", Back: "b",
		State: "review", DueAt: fixedClock.Add(-48 * time.Hour).Format(time.RFC3339)}

	due, err := svc.DueCards(ctx, deck.ID)
	require.NoError(t, err)
	require.Len(t, due, 2)
	assert.Equal(t, overdue.ID, due[0].ID, "thẻ quá hạn lâu nhất lên trước")
	assert.Equal(t, fresh.ID, due[1].ID)
}

func Test_due_cards_excludes_soft_deleted(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	_, card := seedDeckWithCard(t, svc)
	require.NoError(t, svc.DeleteCard(ctx, card.ID))
	due, err := svc.DueCards(ctx, card.DeckID)
	require.NoError(t, err)
	assert.Empty(t, due)
}

// ── Tone ────────────────────────────────────────────────────────────────────

func Test_set_card_tone_normalises_blank_to_null(t *testing.T) {
	svc, _, _ := newTestService()
	_, card := seedDeckWithCard(t, svc)
	ctx := context.Background()

	blank := "   "
	got, err := svc.SetCardTone(ctx, card.ID, &blank)
	require.NoError(t, err)
	assert.Nil(t, got.Tone, "tone rỗng = NULL, không phải chuỗi rỗng")

	tone := "  ní  "
	got, err = svc.SetCardTone(ctx, card.ID, &tone)
	require.NoError(t, err)
	require.NotNil(t, got.Tone)
	assert.Equal(t, "ní", *got.Tone, "tone được trim")
}

func Test_set_card_tone_missing_card_returns_404(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.SetCardTone(context.Background(), 999, nil)
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
}

// ── DeckReader port cho roadmap ────────────────────────────────────────────

func Test_find_deck_reports_missing_as_result_not_error(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	deck, _ := seedDeckWithCard(t, svc)

	info, err := svc.FindDeck(ctx, deck.ID)
	require.NoError(t, err)
	assert.True(t, info.Exists)
	assert.Equal(t, "zh", info.Lang)

	missing, err := svc.FindDeck(ctx, 999)
	require.NoError(t, err, "deck không có là kết quả hợp lệ, không phải lỗi hệ thống")
	assert.False(t, missing.Exists)

	zero, err := svc.FindDeck(ctx, 0)
	require.NoError(t, err)
	assert.False(t, zero.Exists, "id <= 0 là câu hỏi vô nghĩa, trả về không có")
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func Test_parse_ts_tolerates_dirty_data_without_panicking(t *testing.T) {
	assert.True(t, parseTS("").IsZero(), "chuỗi rỗng = zero time, không phải lỗi")
	assert.True(t, parseTS("không-parse-được").IsZero(),
		"1 row hỏng không được làm sập cả hàng đợi ôn")
	loc := time.FixedZone("UTC+7", 7*3600)
	got := parseTS("2026-09-28T10:00:00+07:00")
	assert.True(t, got.Equal(time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)), "mọi mốc chuẩn hoá UTC, got %s (%v)", got, loc)
}

func Test_format_ts_zero_becomes_empty_string(t *testing.T) {
	assert.Equal(t, "", formatTS(time.Time{}),
		"zero time ghi thành \"\" chứ không \"0001-01-01...\" để khỏi lọt vào bộ lọc theo mốc")
	assert.Equal(t, "2026-09-28T03:00:00Z", formatTS(time.Date(2026, 9, 28, 10, 0, 0, 0, time.FixedZone("UTC+7", 7*3600))))
}

func Test_new_guid_never_repeats(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		g := NewGUID()
		assert.NotEmpty(t, g)
		assert.False(t, seen[g], "guid trùng lần %d", i)
		seen[g] = true
	}
}
