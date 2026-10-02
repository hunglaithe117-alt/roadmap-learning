package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/content"
)

// Service là use case của context content. 1 struct cho cả dict / seed / tone
// / THIEU vì chúng dùng chung repository + uow + clock; tách 4 struct chỉ tăng
// file mà không tách được trách nhiệm nào.
type Service struct {
	repo Repository
	uow  UnitOfWork
	// decks là cầu nối sang bounded context srs (tạo deck/thẻ seed, ghi tone).
	decks DeckWriter
	tone  CardToneWriter
	cards CardReader
	// data là nguồn nội dung bundle sẵn (HSK, en_dict, PVO/TMRND, bút thuận,
	// bài đọc). Tách khỏi Repository vì đây là dữ liệu tĩnh, không phải DB.
	data StaticContent
	now  NowFunc
}

// StaticContent là dữ liệu học thuật bundle trong code (không nằm trong DB).
// Khai ở đây để application không import tầng hạ tầng; dữ liệu thật nằm ở
// tầng hạ tầng, package `contentinfra` (port từ api/*.go v1).
type StaticContent interface {
	// HskSeedByLevel trả seed từ vựng của 1 level HSK (nil = chưa có seed).
	HskSeedByLevel(level string) []StaticHskEntry
	// EnDict là từ điển tiếng Anh tích hợp.
	EnDict() []StaticENEntry
	// PVO / TMRND là 2 nhóm thẻ seed tiếng Anh.
	PVO() []StaticSeedCard
	PVOT82() []StaticSeedCard
	TMRND() []StaticSeedCard
	TMRNDT82() []StaticSeedCard
	// StrokeIndex là index nhẹ 1 level; LookupStroke trả chi tiết từng chữ.
	StrokeIndex(level string) []StaticStrokeIndex
	LookupStroke(level, hanzi string) (StaticStrokeInfo, bool)
	// ReaderArticles lọc theo level + id (rỗng = không lọc).
	ReaderArticles(level, id string) []StaticReaderArticle
}

// Static* là DTO của dữ liệu tĩnh. Khai cục bộ ở application (không dùng
// struct của infrastructure) để hướng phụ thuộc vẫn 1 chiều.
type StaticHskEntry struct {
	Hanzi  string
	Pinyin string
	Nghia  string
	Level  string
	Tone   string
}

type StaticENEntry struct {
	Lang    string
	Term    string
	Reading string
	Gloss   string
}

type StaticSeedCard struct {
	Front  string
	Back   string
	IPA    string
	Stress string
}

type StaticStrokeIndex struct {
	Hanzi       string
	StrokeCount int
}

type StaticStrokeInfo struct {
	Hanzi       string
	PinyinMarks string
	Level       string
	StrokeCount int
	Strokes     []StaticStrokeStep
}

type StaticStrokeStep struct {
	Order int
	Code  string
	Name  string
}

type StaticReaderArticle struct {
	ID     string
	Level  string
	Lang   string
	Title  string
	Text   string
	Source string
}

// NewService dựng service. nowFn nil → UTC thật. `data` nil thì các use case
// đọc dữ liệu tĩnh trả lỗi tường minh thay vì panic.
func NewService(repo Repository, uow UnitOfWork, decks DeckWriter, tone CardToneWriter,
	cards CardReader, data StaticContent, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, decks: decks, tone: tone, cards: cards,
		data: data, now: nowFn}
}

// ── Tra từ điển ─────────────────────────────────────────────────────────────

// DefaultSearchLimit là số kết quả mặc định — khớp `DEFAULT` của hàm SQL
// `dict_search`/`en_dict_search` (M1 ghi rõ dict_search cũ từng default 20 là
// lệch với LIMIT của caller v1).
const DefaultSearchLimit = 10

// SearchDict tra từ điển Trung (hanzi/pinyin/nghia).
//
// Hạn chế CẦN GHI RÕ cho UI (đã document ở STACK-V2-PLAN §4.2): `to_tsvector`
// với config `simple` coi cả chuỗi Hán là MỘT token, nên tra Hán đa ký tự đi
// qua đường `ILIKE` chứ không qua `@@`. Tra 1 ký tự Hán vẫn ra kết quả
// (đường ILIKE), nhưng KHÔNG phải do Postgres mạnh hơn FTS5 — app v1 với
// SQLite FTS5 cũng trả `[]` cho cả `MATCH '你好'` lẫn `MATCH '你'`.
func (s *Service) SearchDict(ctx context.Context, q string, limit int) ([]ZHEntry, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return nil, newError(StatusBadRequest, "thiếu q")
	}
	return s.repo.SearchZH(ctx, query, clampLimit(limit, DefaultSearchLimit, 50))
}

// SearchEnglish tra từ điển Anh (headword/IPA/nghĩa).
func (s *Service) SearchEnglish(ctx context.Context, q string, limit int) ([]ENEntry, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return nil, newError(StatusBadRequest, "thiếu q")
	}
	return s.repo.SearchEN(ctx, query, clampLimit(limit, DefaultSearchLimit, 50))
}

// LookupStress tra trọng âm 1 từ: ưu tiên tra từ điển, không có thì rơi về bộ
// quy tắc hậu tố (luôn gắn cờ `exception` để UI nói rõ đây là đoán).
func (s *Service) LookupStress(ctx context.Context, word string) (domain.StressResult, error) {
	key := strings.TrimSpace(word)
	if key == "" {
		return domain.StressResult{}, newError(StatusBadRequest, "thiếu word")
	}
	e, found, err := s.repo.LookupEN(ctx, key)
	if err != nil {
		return domain.StressResult{}, fmt.Errorf("tra en_dict: %w", err)
	}
	// `lower(term) = lower(?)` trả về term gốc trong DB (giữ hoa/thường như lúc
	// seed); domain dùng key đã hạ chữ thường để tra bảng ngoại lệ.
	return domain.LookupStress(key, domain.EnglishEntry{
		Term: e.Term, Reading: e.Reading,
	}, found), nil
}

// UpsertDictEntry thêm/sửa 1 mục từ điển. Idempotent theo chữ Hán: chữ đã có
// thì KHÔNG ghi đè nội dung cũ (người dùng sửa nghĩa thì import lại không được
// xoá) — hành vi port từ ZhImportHandler v1.
func (s *Service) UpsertDictEntry(ctx context.Context, e ZHEntry) (ZHEntry, bool, error) {
	hanzi := strings.TrimSpace(e.Hanzi)
	if hanzi == "" {
		return ZHEntry{}, false, newError(StatusBadRequest, "thiếu chữ Hán")
	}
	if len([]rune(hanzi)) > 64 {
		return ZHEntry{}, false, newError(StatusBadRequest, "chữ Hán quá dài")
	}
	var inserted bool
	err := s.inTx(ctx, func(tx Tx) error {
		known, err := s.repo.DictHanziSet(ctx, tx)
		if err != nil {
			return fmt.Errorf("đọc từ điển: %w", err)
		}
		if known[hanzi] {
			return nil
		}
		ok, err := s.repo.InsertDict(ctx, tx, &ZHEntry{
			Hanzi: hanzi, Pinyin: strings.TrimSpace(e.Pinyin), Nghia: strings.TrimSpace(e.Nghia),
		})
		if err != nil {
			return fmt.Errorf("lưu từ điển: %w", err)
		}
		inserted = ok
		return nil
	})
	if err != nil {
		return ZHEntry{}, false, err
	}
	return ZHEntry{Hanzi: hanzi, Pinyin: e.Pinyin, Nghia: e.Nghia}, inserted, nil
}

// ── Nét chữ ─────────────────────────────────────────────────────────────────

// GetStrokes trả index nhẹ của 1 level, hoặc chi tiết 1 chữ khi `hanzi` khác
// rỗng. Chưa có dữ liệu chữ đó (hoặc thuộc level khác) → 404.
func (s *Service) GetStrokes(ctx context.Context, level, hanzi string) ([]StaticStrokeIndex, StaticStrokeInfo, error) {
	if s.data == nil {
		return nil, StaticStrokeInfo{}, errors.New("chưa cấu hình nguồn dữ liệu nét chữ")
	}
	lvl := strings.TrimSpace(level)
	if lvl == "" {
		lvl = "HSK1" // mặc định port từ ZhStrokesHandler v1
	}
	h := strings.TrimSpace(hanzi)
	if h == "" {
		return s.data.StrokeIndex(lvl), StaticStrokeInfo{}, nil
	}
	info, ok := s.data.LookupStroke(lvl, h)
	if !ok {
		return nil, StaticStrokeInfo{}, newError(StatusNotFound, "chưa có dữ liệu nét cho chữ này")
	}
	return nil, info, nil
}

// ── Bài đọc ─────────────────────────────────────────────────────────────────

// GetReaderArticles trả bài đọc lọc theo level và id (rỗng = không lọc).
func (s *Service) GetReaderArticles(ctx context.Context, level, id string) ([]StaticReaderArticle, error) {
	if s.data == nil {
		return nil, errors.New("chưa cấu hình nguồn dữ liệu bài đọc")
	}
	return s.data.ReaderArticles(strings.TrimSpace(level), strings.TrimSpace(id)), nil
}

// ── Chunking ────────────────────────────────────────────────────────────────

// Chunk tách câu thành từ nội dung / từ chức năng — quy tắc nằm ở
// domain/content.SplitChunks, tầng này chỉ validate input.
func (s *Service) Chunk(ctx context.Context, sentence string) ([]domain.Chunk, error) {
	if strings.TrimSpace(sentence) == "" {
		return nil, newError(StatusBadRequest, "câu rỗng")
	}
	return domain.SplitChunks(sentence), nil
}

// ── Thanh điệu ──────────────────────────────────────────────────────────────

// GradeTonePair chấm đáp án drill so với mẫu. Toàn bộ quy tắc nằm ở
// domain/content.GradeTonePair (bảng thang điệu) — tầng này chỉ dịch lỗi
// thuần sang 400.
func (s *Service) GradeTonePair(expected, answered string) (domain.ToneGradeResult, error) {
	if strings.TrimSpace(expected) == "" {
		return domain.ToneGradeResult{}, newError(StatusBadRequest, "thiếu thanh mẫu")
	}
	res, err := domain.GradeTonePair(expected, answered)
	if err != nil {
		return domain.ToneGradeResult{}, wrapTone(err)
	}
	return res, nil
}

// SetTone ghi kết quả chấm thanh vào thẻ qua port sang `srs`.
//
// Bước CHẤM thuộc context này (bảng thang điệu), bước GHI thuộc `srs` (bảng
// `cards`) — đó là lý do `srs.Service.SetCardTone` ở M2 chỉ nhận tone đã chấm.
func (s *Service) SetTone(ctx context.Context, cardID int64, tone string) (string, error) {
	if cardID <= 0 {
		return "", newError(StatusBadRequest, "id thẻ không hợp lệ")
	}
	if s.tone == nil {
		return "", errors.New("chưa cấu hình cổng ghi thanh điệu")
	}
	t := strings.TrimSpace(tone)
	if !domain.ValidTonePattern(t) {
		return "", newError(StatusBadRequest,
			"thanh điệu chỉ gồm 1-5, cách nhau bằng dấu cách hoặc gạch ngang (VD: 3 3)")
	}
	var out string
	err := s.inTx(ctx, func(tx Tx) error {
		got, err := s.tone.SetCardTone(ctx, tx, cardID, &t)
		if err != nil {
			return err
		}
		out = got
		return nil
	})
	if err != nil {
		return "", wrapNotFound(err, "không tìm thấy thẻ")
	}
	return out, nil
}

// ── Import seed HSK ──────────────────────────────────────────────────────────

// ImportInput là input ImportHSK.
type ImportInput struct {
	// Level mặc định "HSK1" (giữ hành vi v1).
	Level string
	// Deck rỗng = trùng Level. Deck KHÁC level bị từ chối 400: key
	// (deck_id, front) là khoá hợp nhất `ux_cards_deck_front`, đổi tên deck cho
	// cùng 1 level sẽ nhân đôi mọi thẻ khi merge (quyết định T9.1 của v1).
	Deck string
}

// ImportResult là báo cáo import, giữ nguyên 5 trường mà endpoint v1 trả.
type ImportResult struct {
	DeckID     int64
	Deck       string
	Level      string
	CardsAdded int
	CardsTotal int
	DictAdded  int
}

// ImportHSK nạp seed từ vựng 1 level HSK vào 1 deck + bảng dict.
//
// IDEMPOTENT theo 2 khoá: deck theo tên, thẻ theo (deck, front), từ điển theo
// chữ Hán. Chạy 2 lần lần thứ 2 phải báo `cards_added = 0` và `dict_added = 0`
// mà tổng số thẻ không đổi.
//
// Toàn bộ nằm trong 1 transaction: crash giữa chừng để lại deck rỗng rồi lần
// import sau tưởng đã xong (COUNT check `ux_cards_deck_front` lúc đó vẫn
// chạy) — đúng lý do M2 đặt `UnitOfWork` làm ranh giới bắt buộc.
func (s *Service) ImportHSK(ctx context.Context, in ImportInput) (ImportResult, error) {
	level := strings.TrimSpace(in.Level)
	if level == "" {
		level = "HSK1"
	}
	deck := strings.TrimSpace(in.Deck)
	if deck == "" {
		deck = level
	}
	if deck != level {
		return ImportResult{}, newError(StatusBadRequest,
			"deck HSK phải trùng level (dùng deck mặc định %s)", level)
	}
	if s.data == nil {
		return ImportResult{}, errors.New("chưa cấu hình nguồn seed HSK")
	}
	entries := s.data.HskSeedByLevel(level)
	if entries == nil {
		return ImportResult{}, newError(StatusBadRequest,
			"chưa có seed cho level %s (có HSK1-HSK4)", level)
	}
	now := s.now().UTC()
	ts := now.Format(time.RFC3339)
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	deckGUID := SeedDeckGUID(deck, "zh")

	var out ImportResult
	err := s.inTx(ctx, func(tx Tx) error {
		ref, err := s.decks.EnsureSeedDeck(ctx, tx, deck, "zh", deckGUID, ts)
		if err != nil {
			return err
		}
		known, err := s.repo.DictHanziSet(ctx, tx)
		if err != nil {
			return fmt.Errorf("đọc từ điển: %w", err)
		}
		for _, e := range entries {
			tone := e.Tone
			added, err := s.decks.UpsertSeedCard(ctx, tx, SeedCard{
				DeckID: ref.ID, Front: e.Hanzi, Back: e.Nghia, Pinyin: e.Pinyin,
				Tone: &tone, DueAt: due, GUID: SeedCardGUID(deck, "zh", e.Hanzi),
				Now: ts, TouchUpdated: true,
			})
			if err != nil {
				return err
			}
			if added {
				out.CardsAdded++
			}
			if !known[e.Hanzi] {
				if _, err := s.repo.InsertDict(ctx, tx, &ZHEntry{
					Hanzi: e.Hanzi, Pinyin: e.Pinyin, Nghia: e.Nghia,
				}); err != nil {
					return fmt.Errorf("nạp từ điển HSK: %w", err)
				}
				known[e.Hanzi] = true
				out.DictAdded++
			}
		}
		total, err := s.decks.CountDeckCards(ctx, tx, ref.ID)
		if err != nil {
			return err
		}
		out = ImportResult{
			DeckID: ref.ID, Deck: deck, Level: level,
			CardsAdded: out.CardsAdded, CardsTotal: total, DictAdded: out.DictAdded,
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, wrapSeedErr(err)
	}
	return out, nil
}

// wrapSeedErr dịch sentinel của use case seed thành *Error có status đúng:
// sai ngôn ngữ deck là 400 (input sai), còn lỗi kỹ thuật đi nguyên vẹn.
func wrapSeedErr(err error) error {
	if errors.Is(err, ErrLangMismatch) {
		return newError(StatusBadRequest, "deck đã tồn tại nhưng không phải tiếng Trung")
	}
	if errors.Is(err, ErrNotFound) {
		return newError(StatusNotFound, "không tìm thấy deck")
	}
	return err
}

// ── Seed tiếng Anh ──────────────────────────────────────────────────────────

// EnglishSeedResult là báo cáo seed tiếng Anh (3 số, giữ nguyên v1).
type EnglishSeedResult struct {
	EnDict     int
	PVOAdded   int
	TMRNDAdded int
}

// SeedEnglish nạp en_dict tích hợp + 2 deck PVO/TMRND. Idempotent toàn bộ:
// lần 2 không thêm gì.
func (s *Service) SeedEnglish(ctx context.Context) (EnglishSeedResult, error) {
	if s.data == nil {
		return EnglishSeedResult{}, errors.New("chưa cấu hình nguồn seed tiếng Anh")
	}
	now := s.now().UTC()
	ts := now.Format(time.RFC3339)
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	var out EnglishSeedResult

	err := s.inTx(ctx, func(tx Tx) error {
		// en_dict chỉ seed khi bảng TRỐNG: nếu đã có dòng nào thì user đã sửa
		// hoặc peer đã merge vào — nạp thêm chỉ tạo entry trùng.
		dictCount, err := s.repo.CountEN(ctx)
		if err != nil {
			return err
		}
		if dictCount == 0 {
			for _, e := range s.data.EnDict() {
				if _, err := s.repo.InsertEN(ctx, tx, &ENEntry{
					Lang: e.Lang, Term: e.Term, Reading: e.Reading, Gloss: e.Gloss,
				}); err != nil {
					return fmt.Errorf("nạp en_dict: %w", err)
				}
			}
			dictCount = len(s.data.EnDict())
		}
		out.EnDict = dictCount

		pvo, err := s.seedDeck(ctx, tx, "PVO", "en", append(s.data.PVO(), s.data.PVOT82()...), ts, due)
		if err != nil {
			return err
		}
		out.PVOAdded = pvo
		tmrnd, err := s.seedDeck(ctx, tx, "TMRND", "en",
			append(s.data.TMRND(), s.data.TMRNDT82()...), ts, due)
		if err != nil {
			return err
		}
		out.TMRNDAdded = tmrnd
		return nil
	})
	if err != nil {
		return EnglishSeedResult{}, wrapSeedErr(err)
	}
	return out, nil
}

// seedDeck nạp 1 nhóm thẻ seed vào deck tên `name`, trả số thẻ MỚI thêm.
func (s *Service) seedDeck(ctx context.Context, tx Tx, name, lang string,
	cards []StaticSeedCard, ts, due string) (int, error) {
	ref, err := s.decks.EnsureSeedDeck(ctx, tx, name, lang, SeedDeckGUID(name, lang), ts)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, c := range cards {
		ipa, stress := c.IPA, c.Stress
		ok, err := s.decks.UpsertSeedCard(ctx, tx, SeedCard{
			DeckID: ref.ID, Front: c.Front, Back: c.Back,
			IPA: &ipa, Stress: &stress, DueAt: due,
			GUID: SeedCardGUID(name, lang, c.Front), Now: ts, TouchUpdated: true,
		})
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

// ── THIEU ───────────────────────────────────────────────────────────────────

// THIEUPrefix là prefix reserved của context content trong `notes.text`.
// `srs` từ chối note user dùng prefix này (api/decks.go v1), và `practice`
// dùng 2 prefix riêng (SHADOW|/ERR|) — không trùng nhau.
const THIEUPrefix = "THIEU|"

// ThieuInput là input AppendThieu.
type ThieuInput struct {
	// Session rỗng = ngày UTC hôm nay (YYYY-MM-DD), giữ hành vi v1.
	Session string
	// Scores là điểm 8 trục A-H, thang 1-5. Thiếu trục nào cũng 400 — bản
	// chấm 4/8 trục không có nghĩa với checklist "đánh giá buổi luyện".
	Scores map[string]int
	Note   string
}

// ThieuSession là 1 buổi chấm checklist đã đọc lại từ note.
type ThieuSession struct {
	ID        int64
	Session   string
	Scores    map[string]int
	Average   float64
	Note      string
	CreatedAt string
}

// ListThieuAxes trả 8 trục A-H kèm rubric tiếng Việt.
func (s *Service) ListThieuAxes() []domain.THIEUAxis {
	return domain.THIEUAxes
}

// AppendThieu lưu 1 buổi chấm checklist vào `notes` (card_id NULL + prefix
// THIEU|). Không thêm bảng/cột: contract v4 giữ nguyên, và nhờ đó note THIEU
// merge được cùng mọi note khác (union theo guid).
func (s *Service) AppendThieu(ctx context.Context, in ThieuInput) (ThieuSession, error) {
	if len(in.Scores) != len(domain.THIEUAxes) {
		return ThieuSession{}, newError(StatusBadRequest, "thiếu điểm: cần đủ 8 trục A-H")
	}
	for _, ax := range domain.THIEUAxes {
		v, ok := in.Scores[ax.Code]
		if !ok || v < 1 || v > 5 {
			return ThieuSession{}, newError(StatusBadRequest, "điểm trục %s phải từ 1 đến 5", ax.Code)
		}
	}
	session := strings.TrimSpace(in.Session)
	if session == "" {
		session = s.now().UTC().Format("2006-01-02")
	}
	ts := s.timestamp()
	payload, err := json.Marshal(map[string]any{
		"session": session, "scores": in.Scores, "note": in.Note,
	})
	if err != nil {
		return ThieuSession{}, fmt.Errorf("mã hoá checklist: %w", err)
	}
	var id int64
	err = s.inTx(ctx, func(tx Tx) error {
		n := Note{Text: THIEUPrefix + string(payload), CreatedAt: ts, GUID: NewGUID()}
		if err := s.repo.InsertNote(ctx, tx, &n); err != nil {
			return fmt.Errorf("lưu checklist: %w", err)
		}
		id = n.ID
		return nil
	})
	if err != nil {
		return ThieuSession{}, err
	}
	return ThieuSession{
		ID: id, Session: session, Scores: in.Scores,
		Average: domain.THIEUAvg(in.Scores), Note: in.Note, CreatedAt: ts,
	}, nil
}

// ListThieuLimit là số buổi chấm tối đa trả về — giữ nguyên LIMIT 60 của v1.
const ListThieuLimit = 60

// ListThieu trả lịch sử chấm checklist, mới nhất trước. Note hỏng (JSON
// không parse được) bị bỏ qua thay vì làm hỏng cả danh sách.
func (s *Service) ListThieu(ctx context.Context) ([]ThieuSession, error) {
	notes, err := s.repo.ListNotesByPrefix(ctx, THIEUPrefix, ListThieuLimit)
	if err != nil {
		return nil, fmt.Errorf("đọc lịch sử checklist: %w", err)
	}
	out := make([]ThieuSession, 0, len(notes))
	for _, n := range notes {
		session, ok := parseThieuNote(n)
		if !ok {
			continue
		}
		out = append(out, session)
	}
	return out, nil
}

// parseThieuNote giải 1 note THIEU| về ThieuSession. ok=false nếu payload hỏng.
func parseThieuNote(n Note) (ThieuSession, bool) {
	var payload struct {
		Session string         `json:"session"`
		Scores  map[string]int `json:"scores"`
		Note    string         `json:"note"`
	}
	body := strings.TrimPrefix(n.Text, THIEUPrefix)
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ThieuSession{}, false
	}
	if payload.Scores == nil {
		payload.Scores = map[string]int{}
	}
	return ThieuSession{
		ID: n.ID, Session: payload.Session, Scores: payload.Scores,
		Average: domain.THIEUAvg(payload.Scores), Note: payload.Note,
		CreatedAt: n.CreatedAt,
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

// clampLimit giữ limit trong [lo, hi]; <= 0 dùng mặc định.
func clampLimit(limit, def, hi int) int {
	if limit <= 0 {
		return def
	}
	if limit > hi {
		return hi
	}
	return limit
}
