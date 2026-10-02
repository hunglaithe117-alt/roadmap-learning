package sync

import (
	"context"
	"errors"
	"slices"
	"time"

	domain "langapp/internal/domain/sync"
)

var errFake = errors.New("lỗi giả lập")

var fixedNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fakeRepo mô phỏng DB local bằng bộ nhớ: mỗi bảng là `map[guid]Row` cộng
// `idSeq` giả lập sequence Postgres.
//
// Test merge THẬT (2 schema Postgres thật) nằm ở
// package `syncinfra` — ở đây chỉ kiểm tra phối hợp giữa use case và
// `domain/sync.Decide` (thứ tự bảng, rollback 1 transaction, dedupe).
type fakeRepo struct {
	schemaV, peerV int
	lastSync       string
	idSeq          int64
	failOn         string

	// Mỗi bảng = map[guid]{id, payload}. `payload` là DTO có kiểu của bảng đó
	// (lưu `any` để 1 kiểu phục vụ cả 8 bảng LWW thay vì 8 cặp field riêng —
	// con số 8 ở ĐÂY là "số bảng có struct riêng", KHÔNG phải "số bảng merge
	// cập nhật map `local`" — cái đó là 5, xem comment ở `mergeCards`.)
	decks, cards   map[string]stored
	paths, stages  map[string]stored
	ms, topics     map[string]stored
	res, bm        map[string]stored
	reviews, notes map[string]stored

	reviewSeq []string
	conflicts []domain.Conflict

	// skipInsertGUID là các guid mà lần INSERT tương ứng sẽ bị
	// `ON CONFLICT DO NOTHING` bỏ qua ⇒ `RETURNING id` không trả dòng ⇒
	// `id = 0`. Test mục 2 dùng nó để kích hoạt nhánh "insert bị skip" — trước
	// đó nhánh này KHÔNG test được vì fake luôn insert thành công, nên bug im
	// lặng (đếm `merged` + không log conflict) sống dai suốt M3.
	skipInsertGUID []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		schemaV: 4, peerV: 4,
		decks: map[string]stored{}, cards: map[string]stored{},
		paths: map[string]stored{}, stages: map[string]stored{},
		ms: map[string]stored{}, topics: map[string]stored{},
		res: map[string]stored{}, bm: map[string]stored{},
		reviews: map[string]stored{}, notes: map[string]stored{},
	}
}

// fakeUOW mô phỏng transaction: chụp trạng thái trước, callback trả lỗi ⇒ khôi
// phục lại nguyên trạng. Test "rollback khi lỗi giữa chừng" dựa vào đây.
type fakeUOW struct {
	repo *fakeRepo
	inTx int
}

func (u *fakeUOW) Do(_ context.Context, fn func(tx Tx) error) error {
	u.inTx++
	before := u.repo.clone()
	if err := fn(u.repo); err != nil {
		u.repo.restore(before)
		return err
	}
	return nil
}

func (f *fakeRepo) clone() *fakeRepo {
	c := newFakeRepo()
	c.schemaV, c.peerV, c.lastSync, c.idSeq = f.schemaV, f.peerV, f.lastSync, f.idSeq
	for _, pair := range [][2]map[string]stored{
		{c.decks, f.decks}, {c.cards, f.cards}, {c.paths, f.paths},
		{c.stages, f.stages}, {c.ms, f.ms}, {c.topics, f.topics},
		{c.res, f.res}, {c.bm, f.bm}, {c.reviews, f.reviews}, {c.notes, f.notes},
	} {
		copyMapStored(pair[0], pair[1])
	}
	c.reviewSeq = append([]string(nil), f.reviewSeq...)
	return c
}

func (f *fakeRepo) restore(c *fakeRepo) {
	f.decks, f.cards = c.decks, c.cards
	f.paths, f.stages, f.ms, f.topics = c.paths, c.stages, c.ms, c.topics
	f.res, f.bm, f.reviews, f.notes = c.res, c.bm, c.reviews, c.notes
	f.reviewSeq = c.reviewSeq
	f.conflicts = nil
	f.lastSync = c.lastSync
	f.idSeq = c.idSeq
}

// stored là 1 dòng trong fake DB: id số (sequence giả lập) + payload có kiểu.
type stored struct {
	id      int64
	payload any
}

func copyMapStored(dst, src map[string]stored) {
	for k, v := range src {
		dst[k] = v
	}
}

func (f *fakeRepo) nextID() int64 { f.idSeq++; return f.idSeq }

// ── Repository ──────────────────────────────────────────────────────────────

func (f *fakeRepo) SchemaVersion(context.Context) (int, error)     { return f.schemaV, nil }
func (f *fakeRepo) PeerSchemaVersion(context.Context) (int, error) { return f.peerV, nil }
func (f *fakeRepo) LastSyncAt(context.Context) (string, error)     { return f.lastSync, nil }
func (f *fakeRepo) SetLastSyncAt(_ context.Context, _ Tx, now string) error {
	f.lastSync = now
	return nil
}

func (f *fakeRepo) LogConflict(_ context.Context, _ Tx, c domain.Conflict) error {
	f.conflicts = append(f.conflicts, c)
	return nil
}
func (f *fakeRepo) ListConflicts(context.Context, int) ([]domain.Conflict, error) {
	return f.conflicts, nil
}
func (f *fakeRepo) CountConflicts(context.Context) (int, error) { return len(f.conflicts), nil }

func (f *fakeRepo) DeckRows(context.Context, Tx) (map[string]DeckRow, error) {
	out := map[string]DeckRow{}
	for guid, cell := range f.decks {
		d := cell.payload.(DeckRow)
		d.ID = cell.id
		out[guid] = d
	}
	return out, nil
}

func (f *fakeRepo) UpsertDeck(_ context.Context, _ Tx, d DeckRow, found bool) (int64, error) {
	if f.failOn == "decks" {
		return 0, errFake
	}
	// `found = true`: caller đã tự resolve id local, giữ nguyên `d.ID`.
	if found {
		f.decks[d.GUID] = stored{id: d.ID, payload: d}
		return d.ID, nil
	}
	d.ID = f.nextID()
	f.decks[d.GUID] = stored{id: d.ID, payload: d}
	return d.ID, nil
}

func (f *fakeRepo) CardRows(context.Context, Tx) (map[string]CardRow, error) {
	out := map[string]CardRow{}
	for guid, cell := range f.cards {
		c := cell.payload.(CardRow)
		c.ID = cell.id
		out[guid] = c
	}
	return out, nil
}

func (f *fakeRepo) TombstoneCard(_ context.Context, _ Tx, _ int64, front string) (int64, string, bool, error) {
	for guid, cell := range f.cards {
		row := cell.payload.(CardRow)
		if row.Front == front && row.Deleted == 1 {
			return cell.id, guid, true, nil
		}
	}
	return 0, "", false, nil
}

// LiveCardGUIDByFront mô phỏng `WHERE deck_id = ? AND front = ? AND deleted = 0
// ORDER BY id LIMIT 1` — phải quét thật thay vì luôn trả "không thấy", nếu không
// các nhánh merge dựa vào nó (trùng front 2 máy, tombstone của peer sập vào
// thẻ sống) sẽ không bao giờ chạy trong test.
func (f *fakeRepo) LiveCardGUIDByFront(_ context.Context, _ Tx, deckID int64, front string) (int64, string, bool, error) {
	bestID := int64(0)
	bestGUID := ""
	for guid, cell := range f.cards {
		row := cell.payload.(CardRow)
		if row.Front != front || row.Deleted != 0 {
			continue
		}
		if row.DeckID == nil || *row.DeckID != deckID {
			continue
		}
		if bestGUID == "" || cell.id < bestID {
			bestID, bestGUID = cell.id, guid
		}
	}
	if bestGUID == "" {
		return 0, "", false, nil
	}
	return bestID, bestGUID, true, nil
}

func (f *fakeRepo) UpsertCard(_ context.Context, _ Tx, c CardRow, found bool) (int64, bool, error) {
	if f.failOn == "cards" {
		return 0, false, errFake
	}
	// `found = true`: caller đã tự resolve id local (merge dùng đúng id đó khi
	// hồi sinh tombstone), nên giữ nguyên `c.ID` thay vì tra lại theo guid.
	//
	// XOÁ khoá cũ mang cùng `id`: ở đường hồi sinh tombstone, câu SQL thật là
	// `UPDATE cards SET guid = ? WHERE id = ?` — đổi khoá chứ không thêm row.
	// Fake phải mô phỏng đúng, nếu không nó tự sinh 1 thẻ ma và mọi assert
	// "không nhân đôi" ở test đều vô nghĩa.
	if found {
		for guid, cell := range f.cards {
			if cell.id == c.ID && guid != c.GUID {
				delete(f.cards, guid)
			}
		}
		f.cards[c.GUID] = stored{id: c.ID, payload: c}
		return c.ID, false, nil
	}
	c.ID = f.nextID()
	f.cards[c.GUID] = stored{id: c.ID, payload: c}
	return c.ID, false, nil
}

func (f *fakeRepo) PathRows(context.Context, Tx) (map[string]PathRow, error) {
	return rowsAs[PathRow](f.paths), nil
}
func (f *fakeRepo) UpsertPath(_ context.Context, _ Tx, p PathRow, found bool) (int64, bool, error) {
	return f.upsertAny("paths", p.GUID, p, found, f.paths)
}

func (f *fakeRepo) StageRows(context.Context, Tx) (map[string]StageRow, error) {
	return rowsAs[StageRow](f.stages), nil
}
func (f *fakeRepo) UpsertStage(_ context.Context, _ Tx, s StageRow, found bool) (int64, bool, error) {
	return f.upsertAny("stages", s.GUID, s, found, f.stages)
}

func (f *fakeRepo) MilestoneRows(context.Context, Tx) (map[string]MilestoneRow, error) {
	return rowsAs[MilestoneRow](f.ms), nil
}
func (f *fakeRepo) UpsertMilestone(_ context.Context, _ Tx, m MilestoneRow, found bool) (int64, bool, error) {
	return f.upsertAny("milestones", m.GUID, m, found, f.ms)
}

func (f *fakeRepo) TopicRows(context.Context, Tx) (map[string]TopicRow, error) {
	return rowsAs[TopicRow](f.topics), nil
}
func (f *fakeRepo) UpsertTopic(_ context.Context, _ Tx, tp TopicRow, found bool) (int64, bool, error) {
	return f.upsertAny("topics", tp.GUID, tp, found, f.topics)
}

func (f *fakeRepo) ResourceRows(context.Context, Tx) (map[string]ResourceRow, error) {
	return rowsAs[ResourceRow](f.res), nil
}
func (f *fakeRepo) UpsertResource(_ context.Context, _ Tx, r ResourceRow, found bool) (int64, bool, error) {
	return f.upsertAny("resources", r.GUID, r, found, f.res)
}

func (f *fakeRepo) BookmarkRows(context.Context, Tx) (map[string]BookmarkRow, error) {
	return rowsAs[BookmarkRow](f.bm), nil
}
func (f *fakeRepo) UpsertBookmark(_ context.Context, _ Tx, b BookmarkRow, found bool) (int64, bool, error) {
	return f.upsertAny("bookmarks", b.GUID, b, found, f.bm)
}

func (f *fakeRepo) ReviewGUIDs(context.Context, Tx) (map[string]bool, error) {
	out := map[string]bool{}
	for guid := range f.reviews {
		out[guid] = true
	}
	return out, nil
}

func (f *fakeRepo) AppendReview(_ context.Context, _ Tx, r ReviewRow) (bool, error) {
	if f.failOn == "reviews" {
		return false, errFake
	}
	if _, ok := f.reviews[r.GUID]; ok {
		return false, nil
	}
	f.reviews[r.GUID] = stored{payload: r}
	f.reviewSeq = append(f.reviewSeq, r.GUID)
	return true, nil
}

func (f *fakeRepo) ReviewsOfCard(_ context.Context, _ Tx, cardID int64) ([]ReplayReview, error) {
	out := []ReplayReview{}
	for _, guid := range f.reviewSeq {
		r := f.reviews[guid].payload.(ReviewRow)
		if r.CardID != nil && *r.CardID == cardID {
			out = append(out, ReplayReview{Grade: r.Grade, ReviewedAt: r.ReviewedAt})
		}
	}
	return out, nil
}

func (f *fakeRepo) ReplayCard(_ context.Context, _ Tx, cardID int64, res ReplayResult, now string) error {
	if f.failOn == "replay" {
		return errFake
	}
	for guid, cell := range f.cards {
		row := cell.payload.(CardRow)
		if row.ID != cardID {
			continue
		}
		row.Reps, row.Lapses = res.Reps, res.Lapses
		row.Stability, row.Difficulty = res.Stability, res.Difficulty
		row.DueAt, row.State, row.UpdatedAt = res.DueAt, "review", now
		f.cards[guid] = stored{id: cell.id, payload: row}
		return nil
	}
	return nil
}

func (f *fakeRepo) NoteGUIDs(context.Context, Tx) (map[string]bool, error) {
	out := map[string]bool{}
	for guid := range f.notes {
		out[guid] = true
	}
	return out, nil
}

func (f *fakeRepo) AppendNote(_ context.Context, _ Tx, n NoteRow) (bool, error) {
	if f.failOn == "notes" {
		return false, errFake
	}
	if _, ok := f.notes[n.GUID]; ok {
		return false, nil
	}
	f.notes[n.GUID] = stored{payload: n}
	return true, nil
}

// upsertAny là hàm ghi chung cho 6 bảng roadmap: `found` = update (giữ id) vs
// insert (id mới).
func (f *fakeRepo) upsertAny(table, guid string, payload any, found bool, cells map[string]stored) (int64, bool, error) {
	if f.failOn == table {
		return 0, false, errFake
	}
	if found {
		payload = withID(payload, cells[guid].id)
		cells[guid] = stored{id: cells[guid].id, payload: payload}
		return cells[guid].id, false, nil
	}
	if slices.Contains(f.skipInsertGUID, guid) {
		return 0, true, nil
	}
	id := f.nextID()
	cells[guid] = stored{id: id, payload: withID(payload, id)}
	return id, false, nil
}

// rowsAs đổi `map[guid]stored` sang `map[guid]T` có gán id — 1 hàm cho 8
// bảng thay vì 8 vòng lặp gần như giống hệt.
func rowsAs[T any](cells map[string]stored) map[string]T {
	out := make(map[string]T, len(cells))
	for guid, cell := range cells {
		row := cell.payload.(T)
		out[guid] = withIDOf(row, cell.id)
	}
	return out
}

func withID(payload any, id int64) any {
	switch r := payload.(type) {
	case PathRow:
		r.ID = id
		return r
	case StageRow:
		r.ID = id
		return r
	case MilestoneRow:
		r.ID = id
		return r
	case TopicRow:
		r.ID = id
		return r
	case ResourceRow:
		r.ID = id
		return r
	case BookmarkRow:
		r.ID = id
		return r
	default:
		return payload
	}
}

func withIDOf[T any](row T, id int64) T {
	if p, ok := any(row).(PathRow); ok {
		p.ID = id
		return any(p).(T)
	}
	if s, ok := any(row).(StageRow); ok {
		s.ID = id
		return any(s).(T)
	}
	if m, ok := any(row).(MilestoneRow); ok {
		m.ID = id
		return any(m).(T)
	}
	if tp, ok := any(row).(TopicRow); ok {
		tp.ID = id
		return any(tp).(T)
	}
	if r, ok := any(row).(ResourceRow); ok {
		r.ID = id
		return any(r).(T)
	}
	if b, ok := any(row).(BookmarkRow); ok {
		b.ID = id
		return any(b).(T)
	}
	return row
}

// seedLocalCard đưa 1 card vào DB local (dữ liệu có sẵn trước merge).
func seedLocalCard(f *fakeRepo, guid, deckGUID, front string, deleted int, updated string) {
	cardID := f.nextID()
	deckID := f.decks[deckGUID].id
	f.cards[guid] = stored{id: cardID, payload: CardRow{
		ID: cardID, GUID: guid, DeckGUID: deckGUID, DeckID: &deckID,
		Front: front, Back: "b-" + front, Pinyin: "p", DueAt: updated,
		State: "new", CreatedAt: updated, UpdatedAt: updated, Deleted: deleted,
	}}
}

var _ Repository = (*fakeRepo)(nil)

// ── Loader ──────────────────────────────────────────────────────────────────

type fakeLoader struct{ snap domain.PeerSnapshot }

func (l fakeLoader) Load(context.Context) (domain.PeerSnapshot, error) { return l.snap, nil }

func newService(repo *fakeRepo, snap domain.PeerSnapshot) (*Service, *fakeUOW) {
	uow := &fakeUOW{repo: repo}
	return NewService(repo, uow, fakeLoader{snap: snap},
		func() time.Time { return fixedNow }), uow
}

// ── Builders ────────────────────────────────────────────────────────────────

// snapRow dựng 1 `domain/sync.Row` gọn cho test. `parentGUID` đi vào đúng
// trường tuỳ bảng: cards → DeckGUID, reviews/notes → CardGUID, cây roadmap →
// ParentGUID.
func snapRow(table domain.Table, guid, parentGUID, updated string, deleted int, values map[string]string) domain.Row {
	r := domain.Row{
		Table: table, GUID: guid, ParentGUID: parentGUID,
		CreatedAt: updated, UpdatedAt: updated, Deleted: deleted, Values: values,
	}
	switch table {
	case domain.TableCards:
		r.DeckGUID = parentGUID
		r.ParentGUID = ""
	case domain.TableReviews, domain.TableNotes:
		r.CardGUID = parentGUID
		r.ParentGUID = ""
	}
	return r
}
