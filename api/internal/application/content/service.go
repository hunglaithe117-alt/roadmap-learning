package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/content"
	"langapp/internal/typednil"
)

// Service orchestrates content use cases: dictionaries, seeding, tone drills, and THIEU checklists.
type Service struct {
	repo  Repository
	uow   UnitOfWork
	decks DeckWriter
	tone  CardToneWriter
	cards CardReader
	data  StaticContent
	now   NowFunc
}

// StaticContent provides bundled static academic content.
type StaticContent interface {
	HskSeedByLevel(level string) []StaticHskEntry
	EnDict() []StaticENEntry
	PVO() []StaticSeedCard
	PVOT82() []StaticSeedCard
	TMRND() []StaticSeedCard
	TMRNDT82() []StaticSeedCard
	StrokeIndex(level string) []StaticStrokeIndex
	LookupStroke(level, hanzi string) (StaticStrokeInfo, bool)
	ReaderArticles(level, id string) []StaticReaderArticle
}

// StaticHskEntry represents a static HSK seed dictionary entry.
type StaticHskEntry struct {
	Hanzi  string
	Pinyin string
	Nghia  string
	Level  string
	Tone   string
}

// StaticENEntry represents a static English dictionary entry.
type StaticENEntry struct {
	Lang    string
	Term    string
	Reading string
	Gloss   string
}

// StaticSeedCard represents a static English seed card.
type StaticSeedCard struct {
	Front  string
	Back   string
	IPA    string
	Stress string
}

// StaticStrokeIndex represents lightweight stroke metadata for a level.
type StaticStrokeIndex struct {
	Hanzi       string
	StrokeCount int
}

// StaticStrokeInfo represents detailed character stroke steps.
type StaticStrokeInfo struct {
	Hanzi       string
	PinyinMarks string
	Level       string
	StrokeCount int
	Strokes     []StaticStrokeStep
}

// StaticStrokeStep represents an individual stroke drawing step.
type StaticStrokeStep struct {
	Order int
	Code  string
	Name  string
}

// StaticReaderArticle represents a graded reader article.
type StaticReaderArticle struct {
	ID     string
	Level  string
	Lang   string
	Title  string
	Text   string
	Source string
}

// NewService constructs a content service. Defaults to Clock if nowFn is nil.
func NewService(repo Repository, uow UnitOfWork, decks DeckWriter, tone CardToneWriter,
	cards CardReader, data StaticContent, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, decks: decks, tone: tone, cards: cards,
		data: data, now: nowFn}
}


// DefaultSearchLimit is the default result limit for dictionary queries.
const DefaultSearchLimit = 10

// SearchDict searches the Chinese dictionary by hanzi, pinyin, or translation.
func (s *Service) SearchDict(ctx context.Context, q string, limit int) ([]ZHEntry, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return nil, newError(StatusBadRequest, "thiếu q")
	}
	return s.repo.SearchZH(ctx, query, clampLimit(limit, DefaultSearchLimit, 50))
}

// SearchEnglish searches the English dictionary by headword or meaning.
func (s *Service) SearchEnglish(ctx context.Context, q string, limit int) ([]ENEntry, error) {
	query := strings.TrimSpace(q)
	if query == "" {
		return nil, newError(StatusBadRequest, "thiếu q")
	}
	return s.repo.SearchEN(ctx, query, clampLimit(limit, DefaultSearchLimit, 50))
}

// LookupStress looks up word stress from the dictionary or falls back to suffix rules.
func (s *Service) LookupStress(ctx context.Context, word string) (domain.StressResult, error) {
	key := strings.TrimSpace(word)
	if key == "" {
		return domain.StressResult{}, newError(StatusBadRequest, "thiếu word")
	}
	e, found, err := s.repo.LookupEN(ctx, key)
	if err != nil {
		return domain.StressResult{}, fmt.Errorf("tra en_dict: %w", err)
	}
	return domain.LookupStress(key, domain.EnglishEntry{
		Term: e.Term, Reading: e.Reading,
	}, found), nil
}

// UpsertDictEntry inserts a Chinese dictionary entry idempotently without overwriting existing terms.
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

// Strokes returns lightweight stroke index or detailed stroke steps for a character.
func (s *Service) Strokes(ctx context.Context, level, hanzi string) ([]StaticStrokeIndex, StaticStrokeInfo, error) {
	if typednil.Is(s.data) {
		return nil, StaticStrokeInfo{}, errors.New("chưa cấu hình nguồn dữ liệu nét chữ")
	}
	lvl := strings.TrimSpace(level)
	if lvl == "" {
		lvl = "HSK1"
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

// ReaderArticles returns graded reader articles filtered by level and ID.
func (s *Service) ReaderArticles(ctx context.Context, level, id string) ([]StaticReaderArticle, error) {
	if typednil.Is(s.data) {
		return nil, errors.New("chưa cấu hình nguồn dữ liệu bài đọc")
	}
	return s.data.ReaderArticles(strings.TrimSpace(level), strings.TrimSpace(id)), nil
}

// Chunk splits a sentence into content and function words.
func (s *Service) Chunk(ctx context.Context, sentence string) ([]domain.Chunk, error) {
	if strings.TrimSpace(sentence) == "" {
		return nil, newError(StatusBadRequest, "câu rỗng")
	}
	return domain.SplitChunks(sentence), nil
}

// GradeTonePair compares answered tones with expected pattern and grades the drill.
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

// SetTone records validated tone patterns to card metadata via SRS port.
func (s *Service) SetTone(ctx context.Context, cardID int64, tone string) (string, error) {
	if cardID <= 0 {
		return "", newError(StatusBadRequest, "id thẻ không hợp lệ")
	}
	if typednil.Is(s.tone) {
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

// ImportResult reports HSK import counts.
type ImportResult struct {
	DeckID     int64
	Deck       string
	Level      string
	CardsAdded int
	CardsTotal int
	DictAdded  int
}

// ImportHSK imports vocabulary entries for an HSK level into a deck and dictionary table.
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
	if typednil.Is(s.data) {
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

// wrapSeedErr translates seed domain errors to application errors.
func wrapSeedErr(err error) error {
	if errors.Is(err, ErrLangMismatch) {
		return newError(StatusBadRequest, "deck đã tồn tại nhưng không phải tiếng Trung")
	}
	if errors.Is(err, ErrNotFound) {
		return newError(StatusNotFound, "không tìm thấy deck")
	}
	return err
}

// EnglishSeedResult reports English seed statistics.
type EnglishSeedResult struct {
	EnDict     int
	PVOAdded   int
	TMRNDAdded int
}

// SeedEnglish seeds the integrated English dictionary and PVO/TMRND decks.
func (s *Service) SeedEnglish(ctx context.Context) (EnglishSeedResult, error) {
	if typednil.Is(s.data) {
		return EnglishSeedResult{}, errors.New("chưa cấu hình nguồn seed tiếng Anh")
	}
	now := s.now().UTC()
	ts := now.Format(time.RFC3339)
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	var out EnglishSeedResult

	err := s.inTx(ctx, func(tx Tx) error {
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

// THIEUPrefix is the reserved notes prefix for THIEU self-assessment checklists.
const THIEUPrefix = "THIEU|"

// ThieuInput holds parameters for AppendThieu.
type ThieuInput struct {
	Session string
	Scores  map[string]int
	Note    string
}

// ThieuSession represents a recorded checklist evaluation session.
type ThieuSession struct {
	ID        int64
	Session   string
	Scores    map[string]int
	Average   float64
	Note      string
	CreatedAt string
}

// ListThieuAxes returns all 8 checklist evaluation axes.
func (s *Service) ListThieuAxes() []domain.THIEUAxis {
	return domain.THIEUAxes
}

// AppendThieu appends a checklist evaluation note.
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

// ListThieuLimit is the maximum number of checklist sessions returned.
const ListThieuLimit = 60

// ListThieu returns checklist evaluation history, latest first.
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
