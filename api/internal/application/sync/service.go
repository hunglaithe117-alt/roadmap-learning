package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "langapp/internal/domain/sync"
	"langapp/internal/typednil"
)

// Error là lỗi nghiệp vụ mang HTTP status sẵn. Transport (M4) chỉ cần
// `errors.As(err, &appErr)` rồi `appErr.Status`.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Status OK của HTTP, khai báo tại chỗ để application không import net/http.
//
// `StatusNotImplemented` (501) dùng cho "tính năng chưa cấu hình" — cùng ngữ
// nghĩa với `/api/backup` và `/api/restore` (xem `transport/http/backup.go`).
// KHÔNG dùng 503 cho nhánh đó: 503 nghĩa là "tạm thời không phục vụ được, thử
// lại sau", còn thiếu cấu hình thì **thử lại bao nhiêu lần cũng y hệt** — user
// bị bảo "thử lại" sẽ thử lại vô ích.
const (
	StatusOK         = 200
	StatusBadRequest = 400
	StatusNotFound   = 404
	// StatusConflict dành cho merge conflict THẬT (LWW chọn khác giữa local
	// và peer) — hiện chưa có call site nào trả nó vì app chưa có peer, nhưng
	// hằng vẫn khai ở đây để khi M7 bật peer thì đúng mã sẵn có, và để
	// "chưa cấu hình peer" KHÔNG lấn sang mã này (xem `Sync`).
	StatusConflict            = 409
	StatusInternalServerError = 500
	StatusNotImplemented      = 501
)

// ErrNotFound là sentinel cho "không tìm thấy".
var ErrNotFound = errors.New("không tìm thấy")

func newError(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// Service là use case merge của context sync.
type Service struct {
	repo   Repository
	uow    UnitOfWork
	loader SnapshotLoader
	now    NowFunc
}

// NewService dựng service. nowFn nil → UTC thật.
func NewService(repo Repository, uow UnitOfWork, loader SnapshotLoader, nowFn NowFunc) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	return &Service{repo: repo, uow: uow, loader: loader, now: nowFn}
}

// Sync nạp snapshot peer rồi merge vào DB local.
//
// 1 lần gọi = 1 transaction duy nhất cho MỌI bảng. Đây là điểm M3 khác v1
// rõ nhất: v1 chỉ merge `decks`/`cards`/`reviews`/`notes` và bỏ sót toàn bộ
// `roadmap_*` (lỗ hổng đã ghi ở plan M7), nên 2 máy lệch roadmap không bao
// giờ hội tụ.
func (s *Service) Sync(ctx context.Context) (MergeResult, error) {
	// Cổng này là HÀNH ĐỘNG duy nhất chặn "chưa có nguồn snapshot peer" — vì
	// vậy nó phải soi CẢ con trỏ nil, không chỉ interface nil.
	//
	// Trước B2, chỗ gọi truyền `syncinfra.NewSchemaLoader(nil)`: con trỏ nil
	// trong interface ⇒ `s.loader == nil` FALSE ⇒ code đi tiếp vào `Load` với
	// `peer` nil ⇒ panic. Giờ `Wire` truyền `nil` thật, nhưng một ai đó có thể
	// lại truyền con trỏ nil ở lần sau — và khi đó chỉ cần 1 dòng ở đây để lỗi
	// vẫn là lỗi nghiệp vụ thay vì sập server.
	//
	// ── VÌ SAO 501, KHÔNG PHẢI 409 ──────────────────────────────────────
	//
	// 409 Conflict nghĩa là "yêu cầu mâu thuẫn với trạng thái hiện tại" — đúng
	// cho merge thật sự gặp xung đột dữ liệu. Ở đây KHÔNG có mâu thuẫn nào: app
	// v1 đơn giản là **chưa có** nguồn snapshot peer, và `mutation.sync` bị
	// từ chối trước khi đọc/ghi bất cứ thứ gì. Trả 409 khiến client hiểu
	// "dữ liệu của tôi xung đột với peer" — một kết luận sai mà user không
	// cách nào tự kiểm chứng, vì app không có peer để đối chiếu.
	//
	// 501 Not Implemented là mã đúng nghĩa và **nhất quán với `/api/backup`**
	// (`transport/http/backup.go` cũng trả 501 kèm đúng message này cho cùng
	// một tình huống "chưa bật"). 503 thì không hợp: 503 là "tạm thời, thử lại
	// sau", còn thiếu cấu hình thì thử lại vô ích.
	//
	// 500 cũng không được: `toUserError` (`transport/graphql/errors.go`) che
	// message của lỗi 500 thành `"lỗi hệ thống"` ⇒ sửa xong panic thì user
	// vẫn **không đọc được** vì sao. `Test_Sync_never_reports_conflict_for_missing_peer`
	// khoá đúng ranh giới này.
	if typednil.Is(s.loader) {
		return MergeResult{}, newError(StatusNotImplemented, "chưa cấu hình nguồn snapshot peer")
	}
	snap, err := s.loader.Load(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("nạp snapshot peer: %w", err)
	}
	return s.Merge(ctx, snap)
}

// conflictSink gom xung đột trong lúc merge và ghi log DB ngay từng dòng —
// log phải nằm TRONG transaction, nếu ghi sau commit thì merge rollback xong sẽ
// còn log của lần merge đã hủy, khiến UI báo nhầm.
type conflictSink struct {
	repo Repository
	tx   Tx
	all  []domain.Conflict
}

func (c *conflictSink) add(conflict *domain.Conflict) {
	if conflict == nil {
		return
	}
	// Ghi log bỏ qua lỗi: mất 1 dòng log conflict KHÔNG được làm hỏng merge
	// (dữ liệu đã hội tụ là mục tiêu chính, log chỉ để user xem lại).
	_ = c.repo.LogConflict(context.Background(), c.tx, *conflict)
	c.all = append(c.all, *conflict)
}

// addSkipped ghi log cho INSERT bị `ON CONFLICT DO NOTHING` bỏ qua.
//
// Vì sao phải có: `Upsert*` của cây `roadmap_*` dùng `ON CONFLICT DO NOTHING`
// (bắt buộc — Postgres **hủy cả transaction** khi 1 câu lệnh vi phạm UNIQUE,
// xem `insertDeck`). Khi bị skip, `RETURNING id` không trả dòng nên id = 0.
// Trước đây code vẫn `merged.X++` và im lặng: báo cáo nói
// `merged={RoadmapPaths:1}, conflicts=0` trong khi DB **không hề đổi** ⇒ user
// tin là đã sync xong. Đây là loại bug im lặng, nên ghi log thay vì đếm.
func (c *conflictSink) addSkipped(table domain.Table, guid, key, now string) {
	c.add(&domain.Conflict{
		Table: table, GUID: guid, Winner: domain.WinnerLocal,
		Detail: fmt.Sprintf("insert-skipped-duplicate key=%q — peer có row này nhưng "+
			"local đã có row trùng UNIQUE dưới guid khác; dữ liệu peer KHÔNG được ghi",
			key),
		At: now,
	})
}

// SyncStatus là trạng thái merge trả về endpoint `/api/sync/status`.
type SyncStatus struct {
	Enabled       bool
	Strategy      string
	LastSyncAt    string
	ConflictCount int
}

// MergeStrategy là nhãn chiến lược merge — trả về client để UI hiện.
const MergeStrategy = "last-write-win"

// Status trả cờ + last_sync_at + số conflict. `enabled` luôn true: merge luôn
// sẵn sàng, không có cờ tắt/mở (đã bỏ ở v1).
func (s *Service) Status(ctx context.Context) (SyncStatus, error) {
	last, err := s.repo.LastSyncAt(ctx)
	if err != nil {
		return SyncStatus{}, fmt.Errorf("đọc last_sync_at: %w", err)
	}
	n, err := s.repo.CountConflicts(ctx)
	if err != nil {
		return SyncStatus{}, fmt.Errorf("đếm conflict: %w", err)
	}
	return SyncStatus{
		Enabled: true, Strategy: MergeStrategy, LastSyncAt: last, ConflictCount: n,
	}, nil
}

// Conflicts trả log xung đột, mới nhất trước. UI chỉ XEM — không có nút
// resolve tay, vì merge đã tự chọn người thắng và sửa tay sẽ phá hội tụ.
func (s *Service) Conflicts(ctx context.Context, limit int) ([]domain.Conflict, error) {
	if limit <= 0 {
		limit = 200
	}
	out, err := s.repo.ListConflicts(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("đọc log xung đột: %w", err)
	}
	return out, nil
}

// timestampNow là mốc UTC dùng cho mọi ghi trong 1 lần merge.
func (s *Service) timestampNow() string { return s.now().UTC().Format(time.RFC3339) }
