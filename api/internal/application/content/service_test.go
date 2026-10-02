package content

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/content"
)

// Tầng application KHÔNG được import tầng hạ tầng (luật DDD §2 — grep quét
// cả `_test.go`), nên test ở đây dùng test double trong bộ nhớ. Test
// repository thật (Postgres thật) nằm ở package infrastructure.

var errFake = errors.New("lỗi giả lập")

type fakeRepo struct {
	dict    []ZHEntry
	enDict  []ENEntry
	notes   []Note
	nextID  int64
	failOn  string
	dictSet map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{nextID: 1, dictSet: map[string]bool{}}
}

func (f *fakeRepo) id() int64 { f.nextID++; return f.nextID }

func (f *fakeRepo) SearchZH(_ context.Context, q string, limit int) ([]ZHEntry, error) {
	if f.failOn == "SearchZH" {
		return nil, errFake
	}
	// Mô phỏng đủ: khử wildcard + ILIKE, giống hàm SQL `dict_search`.
	qc := strings.NewReplacer("%", "", "_", "", "\\", "").Replace(q)
	out := []ZHEntry{}
	for _, e := range f.dict {
		if strings.Contains(e.Hanzi, qc) || strings.Contains(e.Pinyin, qc) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hanzi < out[j].Hanzi })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) SearchEN(_ context.Context, q string, limit int) ([]ENEntry, error) {
	if f.failOn == "SearchEN" {
		return nil, errFake
	}
	out := []ENEntry{}
	lc := strings.ToLower(q)
	for _, e := range f.enDict {
		if strings.Contains(strings.ToLower(e.Term), lc) {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) LookupEN(_ context.Context, term string) (ENEntry, bool, error) {
	for _, e := range f.enDict {
		if strings.EqualFold(e.Term, term) {
			return e, true, nil
		}
	}
	return ENEntry{}, false, nil
}

func (f *fakeRepo) DictHanziSet(context.Context, Tx) (map[string]bool, error) {
	out := map[string]bool{}
	for k, v := range f.dictSet {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRepo) InsertDict(_ context.Context, _ Tx, e *ZHEntry) (bool, error) {
	if f.failOn == "InsertDict" {
		return false, errFake
	}
	if f.dictSet[e.Hanzi] {
		return false, nil
	}
	f.dict = append(f.dict, *e)
	f.dictSet[e.Hanzi] = true
	return true, nil
}

func (f *fakeRepo) CountDict(context.Context) (int, error) { return len(f.dict), nil }

func (f *fakeRepo) InsertEN(_ context.Context, _ Tx, e *ENEntry) (bool, error) {
	if f.failOn == "InsertEN" {
		return false, errFake
	}
	f.enDict = append(f.enDict, *e)
	return true, nil
}

func (f *fakeRepo) CountEN(context.Context) (int, error) { return len(f.enDict), nil }

func (f *fakeRepo) InsertNote(_ context.Context, _ Tx, n *Note) error {
	if f.failOn == "InsertNote" {
		return errFake
	}
	n.ID = f.id()
	f.notes = append(f.notes, *n)
	return nil
}

func (f *fakeRepo) ListNotesByPrefix(_ context.Context, prefix string, limit int) ([]Note, error) {
	out := []Note{}
	for i := len(f.notes) - 1; i >= 0 && len(out) < limit; i-- {
		if strings.HasPrefix(f.notes[i].Text, prefix) {
			out = append(out, f.notes[i])
		}
	}
	return out, nil
}

// fakeDecks mô phỏng `srs` cho import/seed.
type fakeDecks struct {
	decks  map[string]DeckRef
	cards  map[string]SeedCard
	failOn string
	nextID int64
}

func newFakeDecks() *fakeDecks {
	return &fakeDecks{decks: map[string]DeckRef{}, cards: map[string]SeedCard{}, nextID: 100}
}

func (f *fakeDecks) EnsureSeedDeck(_ context.Context, _ Tx, name, lang, guid, now string) (DeckRef, error) {
	if f.failOn == "EnsureSeedDeck" {
		return DeckRef{}, errFake
	}
	if d, ok := f.decks[name]; ok {
		if d.Lang != lang {
			return DeckRef{}, ErrLangMismatch
		}
		return d, nil
	}
	d := DeckRef{ID: f.nextID, Name: name, Lang: lang}
	f.nextID++
	f.decks[name] = d
	return d, nil
}

func (f *fakeDecks) UpsertSeedCard(_ context.Context, _ Tx, c SeedCard) (bool, error) {
	if f.failOn == "UpsertSeedCard" {
		return false, errFake
	}
	key := strconv.FormatInt(c.DeckID, 10) + "|" + c.Front
	if _, ok := f.cards[key]; ok {
		return false, nil // đã có → idempotent, KHÔNG ghi đè nội dung user
	}
	f.cards[key] = c
	return true, nil
}

func (f *fakeDecks) CountDeckCards(_ context.Context, _ Tx, deckID int64) (int, error) {
	prefix := strconv.FormatInt(deckID, 10) + "|"
	n := 0
	for k := range f.cards {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n, nil
}

type fakeTone struct {
	lastTone *string
	failOn   string
}

func (f *fakeTone) SetCardTone(_ context.Context, _ Tx, _ int64, tone *string) (string, error) {
	if f.failOn == "SetCardTone" {
		return "", errFake
	}
	f.lastTone = tone
	if tone == nil {
		return "", nil
	}
	return *tone, nil
}

type fakeCards struct{ exists bool }

func (f fakeCards) CardExists(context.Context, int64) (bool, error) { return f.exists, nil }

func newService(repo Repository, decks DeckWriter, tone CardToneWriter, now time.Time) *Service {
	return NewService(repo, nil, decks, tone, fakeCards{exists: true}, fakeStatic(), func() time.Time { return now })
}

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fakeStatic là dữ liệu tĩnh nhỏ — test không cần 1043 từ HSK thật, chỉ cần
// chứng minh use case gọi đúng và xử lý idempotent đúng.

func fakeStatic() StaticContent { return fakeStaticData{} }

// fakeStaticData là dữ liệu tĩnh rút gọn cho test.
type fakeStaticData struct{}

func (fakeStaticData) HskSeedByLevel(level string) []StaticHskEntry {
	switch level {
	case "HSK1":
		return []StaticHskEntry{
			{Hanzi: "你", Pinyin: "ni3", Nghia: "bạn", Level: "HSK1", Tone: "3"},
			{Hanzi: "好", Pinyin: "hao3", Nghia: "tốt", Level: "HSK1", Tone: "3"},
		}
	case "HSK2":
		return []StaticHskEntry{{Hanzi: "千", Pinyin: "qian1", Nghia: "nghìn", Level: "HSK2", Tone: "1"}}
	default:
		return nil
	}
}

func (fakeStaticData) EnDict() []StaticENEntry {
	return []StaticENEntry{{Lang: "en", Term: "progress", Reading: "/ˈprəʊɡres/", Gloss: "tiến bộ"}}
}
func (fakeStaticData) PVO() []StaticSeedCard {
	return []StaticSeedCard{{Front: "make progress", Back: "đạt tiến bộ", IPA: "/meɪk ˈprəʊɡres/", Stress: "make-PRO-gress"}}
}
func (fakeStaticData) PVOT82() []StaticSeedCard {
	return []StaticSeedCard{{Front: "take notes", Back: "ghi chép", IPA: "/teɪk nəʊts/", Stress: "take-NOTES"}}
}
func (fakeStaticData) TMRND() []StaticSeedCard {
	return []StaticSeedCard{{Front: "I want to make progress every day.", Back: "Tôi muốn tiến bộ."}}
}
func (fakeStaticData) TMRNDT82() []StaticSeedCard { return nil }

func (fakeStaticData) StrokeIndex(level string) []StaticStrokeIndex {
	if level == "HSK1" {
		return []StaticStrokeIndex{{Hanzi: "人", StrokeCount: 2}}
	}
	return []StaticStrokeIndex{}
}

func (fakeStaticData) LookupStroke(level, hanzi string) (StaticStrokeInfo, bool) {
	if level == "HSK1" && hanzi == "人" {
		return StaticStrokeInfo{
			Hanzi: "人", PinyinMarks: "rén", Level: "HSK1", StrokeCount: 2,
			Strokes: []StaticStrokeStep{{Order: 1, Code: "p", Name: "phẩy"}},
		}, true
	}
	return StaticStrokeInfo{}, false
}

func (fakeStaticData) ReaderArticles(level, id string) []StaticReaderArticle {
	all := []StaticReaderArticle{
		{ID: "hsk1-1", Level: "HSK1", Lang: "zh", Title: "我的一天"},
		{ID: "a1-1", Level: "A1", Lang: "en", Title: "My Morning"},
	}
	out := []StaticReaderArticle{}
	for _, a := range all {
		if level != "" && a.Level != level {
			continue
		}
		if id != "" && a.ID != id {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ── Test ────────────────────────────────────────────────────────────────────

func Test_search_dict_rejects_empty_query(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	_, err := svc.SearchDict(context.Background(), "   ", 10)
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_search_dict_matches_hanzi_and_pinyin(t *testing.T) {
	repo := newFakeRepo()
	repo.dict = []ZHEntry{{Hanzi: "你好", Pinyin: "ni3 hao3", Nghia: "xin chào"}}
	svc := newService(repo, newFakeDecks(), &fakeTone{}, fixedNow)

	// 1 ký tự Hán: đường ILIKE lưới vớ (FTS `to_tsvector('simple')` gộp cả
	// chuỗi Hán làm 1 token nên @@ không khớp — xem STACK-V2-PLAN §4.2).
	got, err := svc.SearchDict(context.Background(), "你", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "你好", got[0].Hanzi)

	got, err = svc.SearchDict(context.Background(), "ni3", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func Test_lookup_stress_prefers_dict_then_rule(t *testing.T) {
	repo := newFakeRepo()
	repo.enDict = []ENEntry{{Lang: "en", Term: "candid", Reading: "/ˈkændɪd/"}}
	svc := newService(repo, newFakeDecks(), &fakeTone{}, fixedNow)

	// Có trong dict → source=dict.
	// Key đưa vào là CHỮ THƯỜNG ("candid" còn "CANDID") — đó là lý do tra
	// chính xác phải dùng `lower(term) = lower(?)`.
	got, err := svc.LookupStress(context.Background(), "CANDID")
	require.NoError(t, err)
	assert.Equal(t, domain.SourceDict, got.Source)
	assert.Equal(t, "candid", got.Term, "trả về term gốc trong DB, không phải input")
	assert.False(t, got.Exception, "headword thường không phải ngoại lệ")

	// Không có trong dict → rơi về quy tắc, exception=true (UI phải gắn nhãn).
	got, err = svc.LookupStress(context.Background(), "elephant")
	require.NoError(t, err)
	assert.Equal(t, domain.SourceRule, got.Source)
	assert.True(t, got.Exception, "đoán bằng quy tắc phải luôn gắn cờ ngoại lệ")
}

func Test_upsert_dict_entry_does_not_overwrite_existing(t *testing.T) {
	repo := newFakeRepo()
	repo.dict = []ZHEntry{{Hanzi: "好", Pinyin: "hao3", Nghia: "tốt/khỏe"}}
	repo.dictSet["好"] = true
	svc := newService(repo, newFakeDecks(), &fakeTone{}, fixedNow)

	_, inserted, err := svc.UpsertDictEntry(context.Background(),
		ZHEntry{Hanzi: "好", Pinyin: "hao9", Nghia: "nghĩa mới của user"})
	require.NoError(t, err)
	assert.False(t, inserted, "chữ đã có thì không ghi đè")
	require.Len(t, repo.dict, 1)
	assert.Equal(t, "tốt/khỏe", repo.dict[0].Nghia, "nội dung cũ phải sống")
}

func Test_import_hsk_twice_adds_nothing_the_second_time(t *testing.T) {
	repo, decks := newFakeRepo(), newFakeDecks()
	svc := newService(repo, decks, &fakeTone{}, fixedNow)

	first, err := svc.ImportHSK(context.Background(), ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	assert.Equal(t, 2, first.CardsAdded)
	assert.Equal(t, 2, first.DictAdded)
	assert.Equal(t, 2, first.CardsTotal)
	assert.Equal(t, int64(100), first.DeckID)

	second, err := svc.ImportHSK(context.Background(), ImportInput{Level: "HSK1"})
	require.NoError(t, err)
	assert.Equal(t, 0, second.CardsAdded, "import lần 2 phải idempotent")
	assert.Equal(t, 0, second.DictAdded)
	assert.Equal(t, 2, second.CardsTotal, "tổng số thẻ không đổi")
	assert.Len(t, repo.dict, 2)
}

func Test_import_hsk_rejects_custom_deck_name(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	_, err := svc.ImportHSK(context.Background(), ImportInput{Level: "HSK1", Deck: "Từ của tôi"})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status,
		"deck khác level sẽ nhân đôi mọi thẻ khi merge (khoá (deck_id, front))")
}

func Test_import_hsk_reports_lang_mismatch(t *testing.T) {
	decks := newFakeDecks()
	decks.decks["HSK1"] = DeckRef{ID: 7, Name: "HSK1", Lang: "en"}
	svc := newService(newFakeRepo(), decks, &fakeTone{}, fixedNow)

	_, err := svc.ImportHSK(context.Background(), ImportInput{Level: "HSK1"})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_import_hsk_rejects_unknown_level(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	_, err := svc.ImportHSK(context.Background(), ImportInput{Level: "HSK9"})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_seed_english_twice_adds_nothing_the_second_time(t *testing.T) {
	repo, decks := newFakeRepo(), newFakeDecks()
	svc := newService(repo, decks, &fakeTone{}, fixedNow)

	first, err := svc.SeedEnglish(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, first.EnDict)
	assert.Equal(t, 2, first.PVOAdded, "1 PVO gốc + 1 PVO T8.2")
	assert.Equal(t, 1, first.TMRNDAdded)

	second, err := svc.SeedEnglish(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, second.PVOAdded, "seed lần 2 phải idempotent")
	assert.Equal(t, 0, second.TMRNDAdded)
	assert.Equal(t, 1, second.EnDict, "en_dict đã có thì không nạp thêm")
	assert.Len(t, repo.enDict, 1)
}

func Test_set_tone_validates_pattern_before_writing(t *testing.T) {
	tone := &fakeTone{}
	svc := newService(newFakeRepo(), newFakeDecks(), tone, fixedNow)

	_, err := svc.SetTone(context.Background(), 1, "3 x")
	require.Error(t, err, "pattern không hợp lệ phải bị chặn TRƯỚC khi ghi")
	assert.Nil(t, tone.lastTone, "không được ghi rác xuống DB")

	got, err := svc.SetTone(context.Background(), 1, " 3 3 ")
	require.NoError(t, err)
	assert.Equal(t, "3 3", got)
	require.NotNil(t, tone.lastTone)
	assert.Equal(t, "3 3", *tone.lastTone)
}

func Test_grade_tone_pair_maps_to_srs_grade(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	res, err := svc.GradeTonePair("3 3", "3 3")
	require.NoError(t, err)
	assert.Equal(t, 4, res.Grade, "trùng hết -> Dễ (4)")
	assert.True(t, res.Exact)

	res, err = svc.GradeTonePair("3 3", "3 4")
	require.NoError(t, err)
	assert.Equal(t, 3, res.Grade, "trùng 1/2 -> Được (3)")

	res, err = svc.GradeTonePair("3 3", "1 1")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Grade, "không trùng âm nào -> Quên (1)")
}

func Test_grade_tone_pair_rejects_syllable_count_mismatch(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	_, err := svc.GradeTonePair("3 3", "3")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}

func Test_append_thieu_requires_all_eight_axes(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, newFakeDecks(), &fakeTone{}, fixedNow)

	_, err := svc.AppendThieu(context.Background(), ThieuInput{Scores: map[string]int{"A": 3}})
	require.Error(t, err, "thiếu trục phải bị chặn — bản chấm 4/8 trục không có nghĩa")

	scores := map[string]int{"A": 3, "B": 4, "C": 5, "D": 2, "E": 3, "F": 4, "G": 5, "H": 2}
	_, err = svc.AppendThieu(context.Background(), ThieuInput{Scores: map[string]int{"A": 9}})
	require.Error(t, err, "điểm ngoài thang 1-5 phải bị chặn")

	got, err := svc.AppendThieu(context.Background(), ThieuInput{Scores: scores, Note: "buổi sáng"})
	require.NoError(t, err)
	assert.Equal(t, 28.0/8, got.Average)
	assert.Equal(t, fixedNow.UTC().Format("2006-01-02"), got.Session,
		"session rỗng = ngày UTC hôm nay")
}

func Test_list_thieu_skips_corrupted_notes(t *testing.T) {
	repo := newFakeRepo()
	repo.notes = []Note{
		{ID: 1, Text: THIEUPrefix + `{"session":"2026-09-01","scores":{"A":5},"note":""}`},
		{ID: 2, Text: THIEUPrefix + "KHÔNG PHẢI JSON"},
		{ID: 3, Text: `SHADOW|{"loops":3}`},
	}
	svc := newService(repo, newFakeDecks(), &fakeTone{}, fixedNow)

	got, err := svc.ListThieu(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1, "note hỏng + note sai prefix phải bị lọc, không làm hỏng cả danh sách")
	assert.Equal(t, "2026-09-01", got[0].Session)
}

func Test_chunk_uses_domain_split(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	got, err := svc.Chunk(context.Background(), "I want to make progress")
	require.NoError(t, err)
	require.Len(t, got, 5)
	assert.Equal(t, domain.ChunkFunction, got[0].Kind, "'I' là từ chức năng")
	assert.Equal(t, domain.ChunkContent, got[4].Kind, "'progress' là từ nội dung")

	_, err = svc.Chunk(context.Background(), "   ")
	require.Error(t, err)
}

func Test_get_strokes_returns_index_or_detail(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	idx, detail, err := svc.GetStrokes(context.Background(), "HSK1", "")
	require.NoError(t, err)
	assert.Empty(t, detail)
	require.Len(t, idx, 1)

	idx, detail, err = svc.GetStrokes(context.Background(), "HSK1", "人")
	require.NoError(t, err)
	assert.Empty(t, idx)
	assert.Equal(t, 2, detail.StrokeCount)
	assert.Equal(t, "rén", detail.PinyinMarks)

	// Chữ có dữ liệu nhưng KHÁC level → 404 (client chỉ hỏi chữ của level
	// đang xem).
	_, _, err = svc.GetStrokes(context.Background(), "HSK2", "人")
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusNotFound, appErr.Status)
}

func Test_get_reader_articles_filters_by_level_and_id(t *testing.T) {
	svc := newService(newFakeRepo(), newFakeDecks(), &fakeTone{}, fixedNow)

	got, err := svc.GetReaderArticles(context.Background(), "HSK1", "")
	require.NoError(t, err)
	require.Len(t, got, 1)

	got, err = svc.GetReaderArticles(context.Background(), "", "a1-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "My Morning", got[0].Title)
}

func Test_seed_guid_is_stable_across_calls(t *testing.T) {
	// 2 máy cùng import HSK1 phải cho cùng guid, nếu không merge sẽ nhân đôi
	// mọi thẻ (đây là lý do M1 giữ `ux_cards_deck_front` + `guid` UNIQUE).
	assert.Equal(t, SeedDeckGUID("HSK1", "zh"), SeedDeckGUID("HSK1", "zh"))
	assert.Equal(t, SeedCardGUID("HSK1", "zh", "你"), SeedCardGUID("HSK1", "zh", "你"))
	assert.NotEqual(t, SeedCardGUID("HSK1", "zh", "你"), SeedCardGUID("HSK1", "zh", "好"))
	assert.NotEqual(t, SeedDeckGUID("HSK1", "zh"), SeedDeckGUID("HSK2", "zh"))
}

func Test_new_guid_never_repeats(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		g := NewGUID()
		require.False(t, seen[g], "guid rỗng/lặp sẽ đụng UNIQUE và làm mất row khi merge")
		seen[g] = true
	}
}
