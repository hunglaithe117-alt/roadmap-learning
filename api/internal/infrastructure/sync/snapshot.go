package syncinfra

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
	domain "langapp/internal/domain/sync"
)

// guidByID dựng bản đồ id→guid cho 1 bảng. Dùng khi đọc row con: các bảng
// merge lưu FK dạng `*_id` số, mà mối liên kết để merge là GUID.
func (r *Repository) guidByID(ctx context.Context, tx app.Tx, table string) (map[int64]string, error) {
	var rows []struct {
		ID   int64
		GUID string
	}
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := db.Table(table).Select("id, guid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid của %s: %w", table, err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.GUID
	}
	return out, nil
}

// resolve tra id số LOCAL của 1 row cha theo GUID, ĐỌC TRONG ĐÚNG `db` đang
// ghi (transaction hoặc pool).
//
// Dùng `db` làm tham số chứ không dùng `r.read()`: khi merge vừa insert 1 path
// mới, con stage của nó phải FK trỏ tới path đó — đọc qua connection khác sẽ
// không thấy row chưa commit.
//
// Chỉ SELECT, không INSERT: mọi row cha đã được merge xong TRƯỚC khi merge bảng
// con (thứ tự bắt buộc vì FK), nên guid không resolve được nghĩa là snapshot
// hỏng — báo lỗi để rollback cả merge thay vì tạo row cha mồ côi.
func (r *Repository) resolve(ctx context.Context, db *gorm.DB, table, guid string) (int64, error) {
	if guid == "" {
		return 0, fmt.Errorf("thiếu guid của cha %s", table)
	}
	var id int64
	if err := db.Raw(`SELECT id FROM `+table+` WHERE guid = ?`, guid).Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("không tìm thấy cha %s guid=%s: %w", table, guid, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("không tìm thấy cha %s guid=%s", table, guid)
	}
	return id, nil
}

// ── Snapshot loader ─────────────────────────────────────────────────────────

// SchemaLoader hiện thực `app.SnapshotLoader` bằng cách đọc trực tiếp 1
// schema Postgres (snapshot máy peer).
//
// M3 dùng nó theo 2 hướng:
//   - Test 2 máy: tạo schema thứ 2 trong cùng database, điền dữ liệu "máy
//     kia", rồi merge vào schema local. Đây là hình thức ATTACH tương
//     đương của Postgres (SQLite có `ATTACH`, Postgres không — M1 §3.8 đã
//     ghi bắt buộc thiết kế lại cơ chế này).
//   - M7: sau `pg_restore` vào schema tạm, trỏ `peer` vào đó — interface không
//     đổi.
//
// Mọi bảng trong `app.SkipTables` KHÔNG được đọc: dict/en_dict là dữ liệu
// tĩnh 2 máy đã giống nhau, còn `schema_migrations`/`goose_db_version` là
// version của chính DB peer và lấy sang sẽ làm máy local tưởng đã upgrade.
type SchemaLoader struct {
	peer *gorm.DB
}

// NewSchemaLoader dựng loader đọc snapshot từ pool trỏ schema peer.
func NewSchemaLoader(peer *gorm.DB) *SchemaLoader { return &SchemaLoader{peer: peer} }

var _ app.SnapshotLoader = (*SchemaLoader)(nil)

// fkField chỉ field nào của `domain.Row` nhận GUID của cha.
type fkField int

const (
	// fkParent → Row.ParentGUID (cha theo cây roadmap).
	fkParent fkField = iota
	// fkDeck → Row.DeckGUID (cha là `decks`).
	fkDeck
	// fkCard → Row.CardGUID (cha là `cards`).
	fkCard
)

// fkRef là 1 cột FK của bảng con, cùng bảng cha và field nhận GUID.
type fkRef struct {
	// column là cột FK trong bảng con, VD "deck_id".
	column string
	// parent là bảng chứa GUID cần tra.
	parent domain.Table
	// field là chỗ đặt GUID sau khi tra.
	field fkField
}

// snapshotTable mô tả 1 bảng trong snapshot: cột đọc + các FK cần resolve.
type snapshotTable struct {
	table domain.Table
	// fks là các cột FK đọc thêm (KHÔNG nằm trong `Values` vì đó là liên
	// kết, không phải payload so sánh LWW).
	//
	// MỘT BẢNG CÓ THỂ CÓ NHIỀU FK: `roadmap_stages` vừa trỏ `path_id` (cha
	// trong cây roadmap) vừa trỏ `deck_id` (amendment A1 — bấm stage nhảy thẳng
	// `/review`). Bỏ sót `deck_id` ở đây khiến merge ghi `deck_id = NULL`,
	// **âm thầm chết feature A1 sau sync đầu tiên** — đó chính là finding B1
	// của cổng Oracle M3.
	fks []fkRef
}

// snapshotTables là thứ tự đọc snapshot. THỨ TỰ KHÔNG QUAN TRỌNG (mọi row
// được đọc trước rồi mới merge) nhưng để cha trước con cho dễ đọc.
var snapshotTables = []snapshotTable{
	{domain.TableDecks, nil},
	{domain.TableCards, []fkRef{{"deck_id", domain.TableDecks, fkDeck}}},
	{domain.TableRoadmapPaths, nil},
	{domain.TableRoadmapStages, []fkRef{
		{"path_id", domain.TableRoadmapPaths, fkParent},
		{"deck_id", domain.TableDecks, fkDeck},
	}},
	{domain.TableRoadmapMilestones, []fkRef{{"stage_id", domain.TableRoadmapStages, fkParent}}},
	{domain.TableRoadmapTopics, []fkRef{{"stage_id", domain.TableRoadmapStages, fkParent}}},
	{domain.TableRoadmapResources, []fkRef{{"topic_id", domain.TableRoadmapTopics, fkParent}}},
	{domain.TableRoadmapBookmarks, nil},
	{domain.TableReviews, []fkRef{{"card_id", domain.TableCards, fkCard}}},
	{domain.TableNotes, []fkRef{{"card_id", domain.TableCards, fkCard}}},
}

// snapshotColumns là cột đọc của từng bảng. Danh sách cố định (không `SELECT
// *`) để 2 máy lệch version không lọt qua nhau — `CheckVersion` chặn trước,
// nhưng đọc theo danh sách khiến lỗi thiếu cột lộ ra ngay.
var snapshotColumns = map[domain.Table][]string{
	domain.TableDecks: {"guid", "name", "lang", "created_at", "updated_at", "deleted"},
	domain.TableCards: {"guid", "front", "back", "pinyin", "due_at", "stability",
		"difficulty", "reps", "lapses", "state", "created_at", "tone", "ipa", "stress",
		"audio_url", "updated_at", "deleted"},
	domain.TableRoadmapPaths: {"guid", "slug", "language", "title", "overview", "is_builtin",
		"created_at", "updated_at", "deleted"},
	// `deck_id` KHÔNG nằm ở đây mà nằm trong `snapshotTables[].fks` (xem
	// `fkRef`): đó là FK cần resolve thành GUID, không phải payload LWW. Bỏ nó
	// khỏi `fks` là mất feature A1 (bấm stage nhảy `/review`) — xem B1.
	domain.TableRoadmapStages: {"guid", "slug", "title", "goal", "position", "duration_weeks",
		"status", "status_note", "created_at", "updated_at", "deleted", "completed_at",
		"terrain", "direction"},
	domain.TableRoadmapMilestones: {"guid", "text", "position", "created_at", "updated_at", "deleted"},
	domain.TableRoadmapTopics: {"guid", "title", "why", "activities", "position", "status",
		"status_note", "created_at", "updated_at", "deleted", "completed_at", "is_optional",
		"map_x", "map_y"},
	domain.TableRoadmapResources: {"guid", "title", "url", "kind", "note", "position",
		"created_at", "updated_at", "deleted"},
	domain.TableRoadmapBookmarks: {"guid", "title", "url", "note", "tags", "status",
		"created_at", "updated_at", "deleted"},
	// `reviews`/`notes` KHÔNG có updated_at/deleted: append-only, merge union
	// theo guid chứ không LWW (thêm tombstone sẽ làm mất lịch sử ôn).
	domain.TableReviews: {"guid", "grade", "reviewed_at", "next_due_at"},
	domain.TableNotes:   {"guid", "text", "created_at"},
}

// Load đọc toàn bộ snapshot peer thành `domain.PeerSnapshot`.
//
// Chặn `peer` nil ở ĐÂY, không chỉ ở `application/sync.Service.Sync`, vì đây mới
// là chỗ bắt đúng hình thức của B2: `NewSchemaLoader(nil)` trả về một con trỏ
// **KHÔNG nil** bọc `peer` nil. Ở đó `s.loader == nil` ở tầng application là
// FALSE (xem `stack-v2-typednil.md` B2) nên tầng trên không chặn được, còn ở
// đây `l.peer == nil` mới nói thật — và chặn được thì chỉ còn 1 dòng.
//
// Không chặn bằng `typednil` ở đây: `l.peer` là `*gorm.DB` — CON TRỎ thật, đã
// là kiểu cụ thể, nên so sánh `== nil` của Go là đúng. Bẫy typed-nil chỉ xảy ra
// khi con trỏ bị GIẤU trong interface.
func (l *SchemaLoader) Load(ctx context.Context) (domain.PeerSnapshot, error) {
	if l == nil || l.peer == nil {
		return domain.PeerSnapshot{}, errors.New("chưa cấu hình nguồn snapshot peer: SchemaLoader không có pool đọc schema peer")
	}
	// Bản đồ id→guid của cha, dựng TRƯỚC khi đọc bảng con (cần resolve FK).
	guidByTable := map[domain.Table]map[int64]string{}
	for _, spec := range snapshotTables {
		for _, fk := range spec.fks {
			if _, done := guidByTable[fk.parent]; done {
				continue
			}
			m, err := l.guidByID(ctx, fk.parent)
			if err != nil {
				return domain.PeerSnapshot{}, err
			}
			guidByTable[fk.parent] = m
		}
	}

	out := domain.PeerSnapshot{SchemaVersion: l.schemaVersion(ctx)}
	for _, spec := range snapshotTables {
		rows, err := l.readRows(ctx, spec, guidByTable)
		if err != nil {
			return domain.PeerSnapshot{}, err
		}
		out.Rows = append(out.Rows, rows...)
	}
	for _, r := range out.Rows {
		if r.UpdatedAt > out.MaxUpdatedAt {
			out.MaxUpdatedAt = r.UpdatedAt
		}
	}
	return out, nil
}

func (l *SchemaLoader) schemaVersion(ctx context.Context) int {
	var v int
	if err := l.peer.WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0
	}
	return v
}

func (l *SchemaLoader) guidByID(ctx context.Context, table domain.Table) (map[int64]string, error) {
	var rows []struct {
		ID   int64
		GUID string
	}
	if err := l.peer.WithContext(ctx).Table(string(table)).Select("id, guid").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc guid %s của snapshot: %w", table, err)
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.GUID
	}
	return out, nil
}

func (l *SchemaLoader) readRows(ctx context.Context, spec snapshotTable, guidByTable map[domain.Table]map[int64]string) ([]domain.Row, error) {
	cols := snapshotColumns[spec.table]
	if cols == nil {
		return nil, fmt.Errorf("bảng %s không có danh sách cột trong snapshot", spec.table)
	}
	selectList := "id, guid"
	if len(cols) > 0 {
		selectList += ", " + joinCols(cols)
	}
	// Đọc thêm từng cột FK (không nằm trong `Values` vì đó là liên kết, không
	// phải payload so sánh LWW).
	for _, fk := range spec.fks {
		selectList += ", " + fk.column
	}
	var raw []map[string]any
	if err := l.peer.WithContext(ctx).Raw("SELECT " + selectList + " FROM " + string(spec.table)).
		Scan(&raw).Error; err != nil {
		return nil, fmt.Errorf("đọc %s của snapshot: %w", spec.table, err)
	}
	out := make([]domain.Row, 0, len(raw))
	for _, m := range raw {
		guid := anyToString(m["guid"])
		if guid == "" {
			// Không có guid thì merge không định danh được (2 row sẽ thành 1).
			// Bỏ thay vì sinh guid ngẫu nhiên — sinh ở máy peer làm mỗi lần
			// load ra 1 guid khác nhau và không bao giờ hội tụ.
			continue
		}
		values := make(map[string]string, len(cols))
		for _, c := range cols {
			// Cột NULL bị BỎ QUA, không ghi thành chuỗi rỗng.
			//
			// Lý do: `domain.Row.Values` là map, và `differs()` so cả `len()` lẫn
			// từng giá trị. Nếu NULL → "" thì một bản có, một bản không
			// (`completed_at` NULL ở local vs `""` từ peer) luôn khác nhau ⇒ luật
			// "2 bản giống hệt → keep" KHÔNG BAO GIỜ chạy, và tầng hạ tầng ghi
			// `completed_at = ''` thay vì NULL ⇒ `idx_roadmap_stages_completed`
			// (`WHERE completed_at IS NOT NULL`) đếm nhầm node chưa xong.
			//
			// Bỏ khoá (thay vì ghi "") cũng giữ đúng dữ liệu `ipa=''` của thẻ
			// TMRND: `''` là giá trị thật, khác NULL, và vẫn còn trong map.
			if m[c] == nil {
				continue
			}
			values[c] = anyToString(m[c])
		}
		r := domain.Row{
			Table:     spec.table,
			GUID:      guid,
			UpdatedAt: anyToString(m["updated_at"]),
			CreatedAt: anyToString(m["created_at"]),
			Deleted:   anyToInt(m["deleted"]),
			Values:    values,
		}
		for _, fk := range spec.fks {
			// FK NULL (VD stage chưa gắn deck) → GUID rỗng. Đó là giá trị HỢP
			// LỆ "cha không có" và khác hẳn với "row không tồn tại" — tầng
			// application dùng `*string` để phân biệt 2 tình huống khi ghi.
			guid := guidByTable[fk.parent][anyToInt64(m[fk.column])]
			switch fk.field {
			case fkDeck:
				r.DeckGUID = guid
			case fkCard:
				r.CardGUID = guid
			default:
				r.ParentGUID = guid
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func joinCols(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += c
	}
	return out
}

// anyToString chuẩn hóa giá trị driver trả về về chuỗi.
//
// PHẢI xử lý `int16`/`int32`/`int64`: pgx trả `INTEGER` (cột `deleted`, `grade`,
// `position`…) là `int32`, mà chỉ xử lý `int64` sẽ biến mọi giá trị số thành
// chuỗi rỗng — `deleted` thành "" và tombstone LWW biến mất âm thầm.
func anyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float32:
		return formatFloat(float64(t))
	case float64:
		return formatFloat(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return ""
	}
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	case []byte:
		if s := string(t); s == "t" || s == "true" {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func anyToInt64(v any) int64 { return int64(anyToInt(v)) }

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }

// formatFloat dùng 'g' -1 để giữ tròn tối đa: cột `map_x`/`map_y` là REAL và
// `stability` là DOUBLE PRECISION, so khớp LWW so sánh CHUỖI — làm tròn quá
// chỗ sẽ báo "khác nhau" giữa 2 máy dù dữ liệu giống nhau.
func formatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
