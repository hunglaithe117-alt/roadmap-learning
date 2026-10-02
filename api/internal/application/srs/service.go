package srs

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/srs"
)

// Service là use case của context srs. 1 struct cho cả deck/card/review vì
// các use case dùng chung repository + uow + clock; tách 3 struct chỉ tăng file
// mà không tách được trách nhiệm nào.
type Service struct {
	repo  Repository
	uow   UnitOfWork
	nowFn NowFunc
}

// NewService dựng service. nowFn nil → UTC thật.
func NewService(repo Repository, uow UnitOfWork, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, nowFn: nowFn}
}

func (s *Service) timestamp() string { return s.nowFn().UTC().Format(time.RFC3339) }

// ── Deck ────────────────────────────────────────────────────────────────────

// CreateDeck tạo bộ thẻ. `lang` rỗng mặc định "zh" (giữ hành vi v1);
// `NormalizeLang` map các biến thể hợp lệ của cùng 1 ngôn ngữ ("zh-CN",
// "EN_US") về zh/en thay vì 400 — còn giá trị không nhận ra thì 400.
func (s *Service) CreateDeck(ctx context.Context, name, lang string) (Deck, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return Deck{}, newError(StatusBadRequest, "thiếu tên deck")
	}
	if strings.TrimSpace(lang) == "" {
		lang = "zh"
	}
	l, ok := domain.NormalizeLang(lang)
	if !ok {
		return Deck{}, newError(StatusBadRequest, "lang chỉ nhận zh hoặc en")
	}
	now := s.timestamp()
	var created Deck
	err := s.inTx(ctx, func(tx Tx) error {
		created = Deck{Name: n, Lang: l, CreatedAt: now, GUID: NewGUID(), UpdatedAt: now}
		if err := s.repo.CreateDeck(ctx, tx, &created); err != nil {
			return fmt.Errorf("tạo deck: %w", err)
		}
		return nil
	})
	return created, err
}

// ListDecks trả mọi deck (deleted = 0).
func (s *Service) ListDecks(ctx context.Context) ([]Deck, error) {
	out, err := s.repo.ListDecks(ctx)
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách deck: %w", err)
	}
	return out, nil
}

// DeleteDeck xoá mềm deck và toàn bộ thẻ của nó (tombstone để sync lan truyền).
func (s *Service) DeleteDeck(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id deck không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.DeckByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy deck")
		}
		if err := s.repo.SoftDeleteDeck(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá deck: %w", err)
		}
		return nil
	})
}

// ── Card ────────────────────────────────────────────────────────────────────

// CardInput là input CreateCard.
type CardInput struct {
	Front  string
	Back   string
	Pinyin string
}

// CreateCard thêm thẻ vào deck. `due_at` khởi tạo = now + 24h — nhưng thẻ vẫn
// vào hàng đợi ôn ngay vì DueFilter luôn kéo `state = 'new'` (xem domain).
func (s *Service) CreateCard(ctx context.Context, deckID int64, in CardInput) (Card, error) {
	if deckID <= 0 {
		return Card{}, newError(StatusBadRequest, "id deck không hợp lệ")
	}
	front := strings.TrimSpace(in.Front)
	back := strings.TrimSpace(in.Back)
	if front == "" || back == "" {
		return Card{}, newError(StatusBadRequest, "thiếu mặt trước/sau của thẻ")
	}
	now := s.nowFn().UTC()
	var created Card
	err := s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.DeckByID(ctx, deckID); err != nil {
			return wrapNotFound(err, "không tìm thấy deck")
		}
		ts := now.Format(time.RFC3339)
		created = Card{
			DeckID: deckID, Front: front, Back: back, Pinyin: in.Pinyin,
			DueAt: now.Add(24 * time.Hour).Format(time.RFC3339),
			// state = 'new' là điều kiện để DueFilter kéo thẻ chưa từng ôn vào
			// hàng đợi dù due_at còn ở tương lai.
			State:     domain.StateNew,
			CreatedAt: ts, GUID: NewGUID(), UpdatedAt: ts,
		}
		if err := s.repo.CreateCard(ctx, tx, &created); err != nil {
			if isUniqueViolation(err) {
				return newError(StatusConflict, "thẻ đã tồn tại trong deck")
			}
			return fmt.Errorf("tạo thẻ: %w", err)
		}
		return nil
	})
	return created, err
}

// ListCards trả thẻ của deck theo id tăng dần.
func (s *Service) ListCards(ctx context.Context, deckID int64) ([]Card, error) {
	if deckID <= 0 {
		return nil, newError(StatusBadRequest, "id deck không hợp lệ")
	}
	if _, err := s.repo.DeckByID(ctx, deckID); err != nil {
		return nil, wrapNotFound(err, "không tìm thấy deck")
	}
	out, err := s.repo.ListCards(ctx, deckID)
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách thẻ: %w", err)
	}
	return out, nil
}

// CardPatch là input UpdateCard.
type CardPatch struct {
	Front  *string
	Back   *string
	Pinyin *string
}

// UpdateCard sửa mặt trước/sau/pinyin. Không cho sửa `due_at` / `reps` — đó là
// output của engine ôn, sửa tay sẽ phá lịch.
func (s *Service) UpdateCard(ctx context.Context, id int64, patch CardPatch) (Card, error) {
	if id <= 0 {
		return Card{}, newError(StatusBadRequest, "id thẻ không hợp lệ")
	}
	if patch.Front == nil && patch.Back == nil && patch.Pinyin == nil {
		return Card{}, newError(StatusBadRequest, "không có gì để cập nhật")
	}
	var updated Card
	err := s.inTx(ctx, func(tx Tx) error {
		c, err := s.repo.CardByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy thẻ")
		}
		if patch.Front != nil {
			c.Front = strings.TrimSpace(*patch.Front)
		}
		if patch.Back != nil {
			c.Back = strings.TrimSpace(*patch.Back)
		}
		if patch.Pinyin != nil {
			c.Pinyin = *patch.Pinyin
		}
		if c.Front == "" || c.Back == "" {
			return newError(StatusBadRequest, "mặt trước/sau không được rỗng")
		}
		if err := s.repo.UpdateCard(ctx, tx, &c); err != nil {
			if isUniqueViolation(err) {
				return newError(StatusConflict, "thẻ đã tồn tại trong deck")
			}
			return fmt.Errorf("cập nhật thẻ: %w", err)
		}
		updated = c
		return nil
	})
	return updated, err
}

// DeleteCard xoá mềm thẻ (tombstone để sync).
func (s *Service) DeleteCard(ctx context.Context, id int64) error {
	if id <= 0 {
		return newError(StatusBadRequest, "id thẻ không hợp lệ")
	}
	return s.inTx(ctx, func(tx Tx) error {
		if _, err := s.repo.CardByID(ctx, id); err != nil {
			return wrapNotFound(err, "không tìm thấy thẻ")
		}
		if err := s.repo.SoftDeleteCard(ctx, tx, id); err != nil {
			return fmt.Errorf("xoá thẻ: %w", err)
		}
		return nil
	})
}

// DueCards trả thẻ đến hạn của deck: quá hạn (due_at <= now) HOẶC chưa từng
// ôn (`state = 'new'`). Không có cờ include_new — thẻ mới có due_at +24h nên
// lọc chỉ theo due_at sẽ giấu chúng khỏi hàng đợi.
func (s *Service) DueCards(ctx context.Context, deckID int64) ([]Card, error) {
	if deckID <= 0 {
		return nil, newError(StatusBadRequest, "id deck không hợp lệ")
	}
	all, err := s.repo.ListCards(ctx, deckID)
	if err != nil {
		return nil, fmt.Errorf("đọc thẻ đến hạn: %w", err)
	}
	cards := make([]domain.Card, 0, len(all))
	for _, c := range all {
		cards = append(cards, cardToDomain(c))
	}
	// Apply lọc + sắp (due_at, id) — quy tắc hàng đợi nằm ở domain, không
	// viết lại ở đây (v1 đã viết tay 1 lần và lệch khi thêm state 'new').
	due := domain.NewDueFilter(s.nowFn(), deckID).Apply(cards)
	out := make([]Card, 0, len(due))
	for _, c := range due {
		out = append(out, cardFromDomain(c))
	}
	return out, nil
}

// ── Review ──────────────────────────────────────────────────────────────────

// ReviewResult là kết quả chấm điểm 1 thẻ — trả về cho client để hiển thị
// "lần sau ôn sau 3 ngày" mà không phải tự tính lại.
type ReviewResult struct {
	CardID       int64   `json:"card_id"`
	DueAt        string  `json:"due_at"`
	IntervalDays int     `json:"interval_days"`
	Stability    float64 `json:"stability"`
	Difficulty   float64 `json:"difficulty"`
	Reps         int     `json:"reps"`
	Fallback     bool    `json:"fallback"`
}

// ReviewInput là input RecordReview.
type ReviewInput struct {
	CardID int64
	Grade  int
}

// RecordReview chấm điểm 1 thẻ: tính lịch bằng domain/srs.ScheduleNext rồi ghi
// UPDATE card + INSERT reviews trong CÙNG 1 transaction. Crash giữa chừng để
// lại thẻ đã lên lịch mà không có lịch sử — sync merge sẽ tính sai toàn bộ
// reps sau đó.
func (s *Service) RecordReview(ctx context.Context, in ReviewInput) (ReviewResult, error) {
	if in.CardID <= 0 {
		return ReviewResult{}, newError(StatusBadRequest, "thiếu card_id")
	}
	grade := domain.Grade(in.Grade)
	if !grade.Valid() {
		return ReviewResult{}, newError(StatusBadRequest, "grade phải từ 1 đến 4 (1=Quên, 4=Dễ)")
	}
	var out ReviewResult
	err := s.inTx(ctx, func(tx Tx) error {
		c, err := s.repo.CardByID(ctx, in.CardID)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy thẻ")
		}
		now := s.nowFn().UTC()
		res := domain.ScheduleNext(c.Reps, c.Stability, c.Difficulty, grade, now)
		lapses := c.Lapses
		if res.Lapse {
			lapses++
		}
		c.DueAt = res.DueAt.Format(time.RFC3339)
		c.Stability = res.Stability
		c.Difficulty = res.Difficulty
		c.Reps++
		c.Lapses = lapses
		c.State = domain.StateReview
		if err := s.repo.UpdateCard(ctx, tx, &c); err != nil {
			return fmt.Errorf("lưu lịch ôn của thẻ: %w", err)
		}
		// guid BẮT BUỘC khác rỗng: `ux_reviews_guid` UNIQUE trên
		// `NOT NULL DEFAULT ''` — 2 review cùng guid rỗng là đụng nhau.
		if err := s.repo.CreateReview(ctx, tx, &Review{
			CardID:     c.ID,
			Grade:      in.Grade,
			ReviewedAt: now.Format(time.RFC3339),
			NextDueAt:  c.DueAt,
			GUID:       NewGUID(),
		}); err != nil {
			return fmt.Errorf("lưu lịch sử ôn: %w", err)
		}
		out = ReviewResult{
			CardID: c.ID, DueAt: c.DueAt, IntervalDays: res.IntervalDays,
			Stability: res.Stability, Difficulty: res.Difficulty,
			Reps: c.Reps, Fallback: res.FallbackUsed,
		}
		return nil
	})
	return out, err
}

// ── Tone ────────────────────────────────────────────────────────────────────

// SetCardTone ghi số thanh đã chấm vào thẻ (nội dung "ni3" → "ní" do content
// context tính, xem internal/domain/content GradeTonePair).
//
// Không có bước tự chấm ở đây: chấm thanh là business rule của context
// `content` (bảng thang điệu), srs chỉ giữ kết quả. Tone nil = xoá.
func (s *Service) SetCardTone(ctx context.Context, id int64, tone *string) (Card, error) {
	if id <= 0 {
		return Card{}, newError(StatusBadRequest, "id thẻ không hợp lệ")
	}
	if tone != nil {
		t := strings.TrimSpace(*tone)
		if t == "" {
			tone = nil
		} else {
			tone = &t
		}
	}
	var updated Card
	err := s.inTx(ctx, func(tx Tx) error {
		c, err := s.repo.CardByID(ctx, id)
		if err != nil {
			return wrapNotFound(err, "không tìm thấy thẻ")
		}
		c.Tone = tone
		if err := s.repo.UpdateCard(ctx, tx, &c); err != nil {
			return fmt.Errorf("lưu thanh điệu của thẻ: %w", err)
		}
		updated = c
		return nil
	})
	return updated, err
}

// ── DeckReader cho context roadmap ──────────────────────────────────────────

// DeckInfo là thông tin deck mà context roadmap cần (port ở
// application/roadmap/ports.go — 2 bounded context khai báo port độc lập để
// không import chéo, chỉ gặp nhau ở wiring của infrastructure).
type DeckInfo struct {
	Exists bool
	Name   string
	Lang   string
}

// FindDeck là hiện thực `roadmap.DeckReader`. Nằm ở package srs vì đây là
// thông tin của bounded context srs — roadmap đọc qua interface, không đọc
// bảng `decks` trực tiếp.
//
// Trả cả `Name` vì M6 hiện nút "vào /review" cần tên deck thật ("Ôn HSK1"),
// không phải id.
// FindDecks là batch của FindDeck: 1 lệnh `id IN (...)` cho NHIỀU deck.
//
// Dataloader `Stage.deck` của cây roadmap (M4) cần đúng hình dạng này: gọi
// `FindDeck` mỗi stage là N statement, và số statement đó tăng theo số stage.
func (s *Service) FindDecks(ctx context.Context, ids []int64) (map[int64]DeckInfo, error) {
	out := make(map[int64]DeckInfo, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.repo.DecksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, d := range rows {
		out[d.ID] = DeckInfo{Exists: true, Name: d.Name, Lang: d.Lang}
	}
	return out, nil
}

func (s *Service) FindDeck(ctx context.Context, id int64) (DeckInfo, error) {
	if id <= 0 {
		return DeckInfo{}, nil
	}
	d, err := s.repo.DeckByID(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return DeckInfo{}, nil // không có là kết quả hợp lệ, không phải lỗi
		}
		return DeckInfo{}, fmt.Errorf("đọc deck %d: %w", id, err)
	}
	return DeckInfo{Exists: true, Name: d.Name, Lang: d.Lang}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func cardToDomain(c Card) domain.Card {
	return domain.Card{
		ID: c.ID, DeckID: c.DeckID, Front: c.Front, Back: c.Back,
		Pinyin: c.Pinyin, DueAt: parseTS(c.DueAt), Stability: c.Stability,
		Difficulty: c.Difficulty, Reps: c.Reps, Lapses: c.Lapses,
		State: c.State, CreatedAt: parseTS(c.CreatedAt),
		Tone: c.Tone, IPA: c.IPA, Stress: c.Stress, AudioURL: c.AudioURL,
		GUID: c.GUID, UpdatedAt: parseTS(c.UpdatedAt), Deleted: c.Deleted,
	}
}

func cardFromDomain(c domain.Card) Card {
	return Card{
		ID: c.ID, DeckID: c.DeckID, Front: c.Front, Back: c.Back,
		Pinyin: c.Pinyin, DueAt: formatTS(c.DueAt), Stability: c.Stability,
		Difficulty: c.Difficulty, Reps: c.Reps, Lapses: c.Lapses,
		State: c.State, CreatedAt: formatTS(c.CreatedAt),
		Tone: c.Tone, IPA: c.IPA, Stress: c.Stress, AudioURL: c.AudioURL,
		GUID: c.GUID, UpdatedAt: formatTS(c.UpdatedAt), Deleted: c.Deleted,
	}
}

// parseTS đọc cột TEXT RFC3339 thành time.Time. Dữ liệu hỏng trả zero time
// (UTC) thay vì panic — 1 row hỏng không được làm sập cả hàng đợi ôn.
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// formatTS là chiều ngược của parseTS. Zero time ghi thành "" (cột NOT NULL
// nhưng row mới chưa có mốc) — không ghi chuỗi "0001-01-01..." vì sẽ lọt vào
// mọi bộ lọc theo mốc thời gian.
func formatTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// inTx chạy fn trong UnitOfWork; uow nil (test chỉ cần validate) → chạy thẳng.
func (s *Service) inTx(ctx context.Context, fn func(tx Tx) error) error {
	if s.uow == nil {
		return fn(nil)
	}
	return s.uow.Do(ctx, fn)
}
