// Package syncinfra là hiện thực GORM của bounded context sync. Đây là nơi
// DUY NHẤT trong context này được phép import gorm.io (STACK-V2-PLAN §2).
//
// Điểm khác biệt so với repository của mọi context khác: Ở ĐÂY mọi method ghi
// đều set `updated_at` TƯỜNG MINH, KHÔNG dùng `Omit("updated_at")`.
//
// Lý do (oracle đã đo, ghi lại để không ai "tối ưu" ngược lại): `Omit` của
// GORM loại cột khỏi tập SET **kể cả khi map có key tường minh**. Dùng nó ở
// merge sẽ khiến trigger `langapp_touch_updated_at` ghi đè mốc LWW từ peer
// bằng giờ máy local → 2 máy so sai vĩnh viễn và một bên ghi đè bên kia mãi.
package syncinfra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/sync"
	domain "langapp/internal/domain/sync"
)

// unitOfWork hiện thực app.UnitOfWork bằng `gorm.DB.Transaction`.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork dựng UnitOfWork trên pool GORM đã có.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(txHandle{tx: tx})
	})
}

type txHandle struct{ tx *gorm.DB }

// errTxMismatch là lỗi trả về khi `app.Tx` không phải handle do package này tạo.
var errTxMismatch = errors.New("tx handle sai context — không phải do repository này tạo")

// txOf trả *gorm.DB đúng phạm vi:
//   - `tx == nil` → gọi ngoài transaction, dùng pool (hợp lệ cho unit test của
//     application khi `uow` không bind).
//   - handle hợp lệ → *gorm.DB của transaction.
//   - handle sai kiểu → LỖI, KHÔNG rơi về pool.
//
// Vì sao phải lỗi chứ không rơi về pool (finding H4 của cổng Oracle M3): im
// lặng rơi về pool biến 1 lỗi lập trình thành **toàn bộ merge ghi ra NGOÀI
// transaction** — không rollback được, và test vẫn xanh vì dữ liệu "vẫn tới
// nơi". Với `sync` còn tệ hơn: transaction đã `SetLastSyncAt` thì rollback,
// nhưng bảng đã merge thì đã commit.
func txOf(root *gorm.DB, tx app.Tx) (*gorm.DB, error) {
	if tx == nil {
		return root, nil
	}
	h, ok := tx.(txHandle)
	if !ok || h.tx == nil {
		return nil, errTxMismatch
	}
	return h.tx, nil
}

// Repository hiện thực app.Repository cho CẢ 2 đầu: local (ghi) và peer (đọc
// snapshot).
//
// M3 dựng 2 schema trong cùng 1 database: `local` trỏ schema đang merge vào,
// `peer` trỏ schema chứa snapshot máy kia. M7 sẽ thay `peer` bằng pool
// chỉ-đọc trỏ schema tạm sau `pg_restore` — interface `application/sync` không
// đổi.
type Repository struct {
	local *gorm.DB
	peer  *gorm.DB
}

// NewRepository dựng repository trên pool GORM local.
func NewRepository(db *gorm.DB) *Repository { return &Repository{local: db} }

// NewPeerRepository dựng repository 2 đầu: `local` để ghi, `peer` chỉ đọc.
func NewPeerRepository(local, peer *gorm.DB) *Repository {
	return &Repository{local: local, peer: peer}
}

var _ app.Repository = (*Repository)(nil)

// write trả *gorm.DB đúng phạm vi ghi: transaction nếu có, ngược lại pool.
func (r *Repository) write(tx app.Tx) (*gorm.DB, error) { return txOf(r.local, tx) }

// read trả pool đọc local cho các method đọc KHÔNG phụ thuộc dữ liệu vừa
// ghi (`SchemaVersion`, `LastSyncAt`, `CountConflicts`, `ListConflicts`).
//
// DANH SÁCH NÀY ĐÓNG, thêm 1 method vào đây là bug chờ xảy ra: mọi thứ gọi từ
// trong `uow.Do` phải đi qua `at(tx)`. Kiểm tra: thêm `tx` vào chữ ký → an toàn;
// thêm vào danh sách này → đọc ngoài transaction, KHÔNG thấy row vừa ghi.
func (r *Repository) read() *gorm.DB { return r.local }

// at trả phạm vi đọc theo `tx`: có transaction thì đọc trong đó (BẮT BUỘC với
// mọi `*Rows` — merge đọc lại chính dữ liệu vừa insert, đọc ngoài transaction
// sẽ không thấy row chưa commit), không thì đọc pool.
func (r *Repository) at(tx app.Tx) (*gorm.DB, error) { return r.write(tx) }

// txCtx là `r.at(tx)` + `WithContext(ctx)` — viết ngắn cho mọi method nhận cả
// hai. Giữ 1 chỗ để không bao giờ quên xử lý lỗi của `txOf`.
func (r *Repository) txCtx(ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := r.at(tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// errNoRows chuẩn hoá lỗi "không tìm thấy" của GORM.
func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

// ── Version + meta ──────────────────────────────────────────────────────────

// SchemaVersion đọc `MAX(version)` của `schema_migrations` (bảng app v1 đọc;
// M7 chuyển sang `goose_db_version`).
//
// Bảng rỗng trả 0 CHỨ không lỗi: `CheckVersion` sẽ từ chối merge, và thông
// báo "version 0" hữu ích hơn lỗi SQL. Cùng hành vi v1.
func (r *Repository) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := r.read().WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0, fmt.Errorf("đọc schema version: %w", err)
	}
	return v, nil
}

// PeerSchemaVersion đọc version của snapshot.
//
// Chặn `peer` nil vì `Wire` dựng `NewPeerRepository(db, nil)` — tức `peer` là
// nil trong app thật. Không chặn thì `r.peer.WithContext(...)` deref
// `*gorm.DB` nil.
//
// Cùng lớp lỗi với B2 (`stack-v2-typednil.md`): ở đó bẫy là con trỏ nil GIẤU
// trong interface; ở đây con trỏ là kiểu cụ thể `*gorm.DB` nên `== nil` của Go
// là đúng — nhưng đường tới panic thì y hệt, và người đọc `Wire` một mình không
// dễ thấy chỗ này sẽ nổ.
//
// Hiện chưa reachable qua HTTP: `Service.Sync` dừng ở cổng loader trước khi tới
// `Merge`. Nhưng `Merge` là hàm public của `Service` và test gọi trực tiếp, nên
// "chưa reachable" ở đây là đặc điểm của wiring HIỆN TẠI chứ không phải bảo
// đảm của hàm.
func (r *Repository) PeerSchemaVersion(ctx context.Context) (int, error) {
	if r == nil || r.peer == nil {
		return 0, errors.New("chưa cấu hình nguồn snapshot peer: repository không có pool đọc schema peer")
	}
	var v int
	if err := r.peer.WithContext(ctx).Raw("SELECT MAX(version) FROM schema_migrations").Scan(&v).Error; err != nil {
		return 0, fmt.Errorf("đọc schema version peer: %w", err)
	}
	return v, nil
}

// LastSyncAt đọc `sync_meta.last_sync_at`, "" khi chưa sync lần nào.
func (r *Repository) LastSyncAt(ctx context.Context) (string, error) {
	var v string
	err := r.read().WithContext(ctx).Raw(
		"SELECT v FROM sync_meta WHERE k = 'last_sync_at'").Scan(&v).Error
	if err != nil {
		// Chưa có dòng nào = chưa sync lần nào, KHÔNG phải lỗi.
		return "", nil
	}
	return v, nil
}

// SetLastSyncAt ghi mốc sync (upsert).
func (r *Repository) SetLastSyncAt(ctx context.Context, tx app.Tx, now string) error {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO sync_meta (k, v) VALUES ('last_sync_at', ?)
		 ON CONFLICT (k) DO UPDATE SET v = excluded.v`, now).Error
}

// ── Nhật ký xung đột ────────────────────────────────────────────────────────

// LogConflict ghi 1 dòng `sync_conflicts`.
func (r *Repository) LogConflict(ctx context.Context, tx app.Tx, c domain.Conflict) error {
	db, err := r.txCtx(ctx, tx)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO sync_conflicts (table_name, guid, winner, detail, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		string(c.Table), c.GUID, c.Winner, c.Detail, c.At).Error
}

// ListConflicts đọc log xung đột, mới nhất trước.
func (r *Repository) ListConflicts(ctx context.Context, limit int) ([]domain.Conflict, error) {
	var rows []struct {
		Table, GUID, Winner, Detail, CreatedAt string
	}
	if err := r.read().WithContext(ctx).Raw(`
		SELECT table_name, guid, winner, detail, created_at
		FROM sync_conflicts ORDER BY id DESC LIMIT ?`, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("đọc log xung đột: %w", err)
	}
	out := make([]domain.Conflict, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Conflict{
			Table: domain.Table(row.Table), GUID: row.GUID, Winner: row.Winner,
			Detail: row.Detail, At: row.CreatedAt,
		})
	}
	return out, nil
}

// CountConflicts đếm tổng số dòng log.
func (r *Repository) CountConflicts(ctx context.Context) (int, error) {
	var n int64
	if err := r.read().WithContext(ctx).Raw("SELECT COUNT(*) FROM sync_conflicts").Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("đếm conflict: %w", err)
	}
	return int(n), nil
}
