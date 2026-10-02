package roadmapinfra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	app "langapp/internal/application/roadmap"
)

// LUẬT GHI — cùng với internal/infrastructure/srs/repository.go, viết lại đầy
// đủ ở đây vì 2 package không import nhau (2 bounded context độc lập):
//
//  1. CẤM `db.Model(&X{}).Update(...)` với struct RỗNG. UPDATE đó không có
//     khoá chính nên GORM không sinh `WHERE id = ?`; với
//     `AllowGlobalUpdate: false` (platform.OpenPostgres) nó trả
//     `ErrMissingWhereClause`, nhưng nếu ai đó bật `AllowGlobalUpdate: true`
//     thì UPDATE chạy trên toàn bảng và trigger `langapp_touch_updated_at` gõ
//     vào MỌI row → merge LWW phía peer chọn nhầm bản cũ. Bắt buộc
//     `db.Model(&x).Updates(...)` theo INSTANCE (x đã load, có ID).
//  2. MỌI đọc lại sau khi ghi phải đi qua `txOf` — đọc bằng `r.db` (pool) sẽ
//     thấy dữ liệu CŨ vì transaction chưa commit (P0 cổng Oracle M2).
//  3. KHÔNG query lồng trong vòng `rows.Next()`: conn giữ `rows` chưa trả về
//     pool, query con kẹp vĩnh viễn khi `MaxOpenConns = 1`. Mọi list ở đây
//     thu thập hết row vào slice trước, đóng rồi mới query tiếp.
//  4. Mọi list lọc `deleted = 0` thủ công (không `gorm.DeletedAt`).
//  5. Mọi ghi bọn trong UnitOfWork (mọi method ghi đều nhận `app.Tx`).
//
// ── CẢNH BÁO CHO M7 (merge LWW) ──────────────────────────────────────────────
// `Omit("updated_at")` KHÔNG chỉ bỏ giá trị ta truyền vào — nó loại cột
// `updated_at` khỏi MỆNH LỆNH `SET` kể cả khi `map` có key `updated_at` tường
// minh (probe trên SQL thật: `SET "title"=$1` chứ không phải
// `SET "title"=$1,"updated_at"=$2`). Nhờ vậy trigger `langapp_touch_updated_at`
// luôn chạm, và nó CỐ Ý bỏ qua khi `NEW.updated_at IS NOT DISTINCT FROM
// OLD.updated_at`.
//
// Hệ quả bắt buộc: **M7 KHÔNG được tái dùng các method `Update*` này cho
// merge peer.** Merge phải có method `Merge*` riêng KHÔNG Omit, ghi
// `updated_at` tường minh bằng mốc của peer, và có test assert mốc peer sống
// sót sau khi trigger chạy. Dùng nhầm `Update*` sẽ ghi đè mốc peer bằng
// `now()` của máy nhận → LWW chọn nhầm bản cũ là bản mới.

// unitOfWork hiện thực app.UnitOfWork bằng `gorm.DB.Transaction`.
type unitOfWork struct{ db *gorm.DB }

// NewUnitOfWork dựng UnitOfWork trên pool GORM đã có.
func NewUnitOfWork(db *gorm.DB) app.UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(app.Tx) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(txHandle{tx: tx})
	})
}

// txHandle là hiện thực của app.Tx (kiểu any) — mang *gorm.DB của transaction.
type txHandle struct{ tx *gorm.DB }

// errTxMismatch là lỗi trả về khi `app.Tx` không phải handle do package này tạo.
// Sự cố này chỉ xảy ra khi 2 context trộn handle — M3 (context `sync` cần ghi
// `roadmap_*` + `decks/cards` trong cùng 1 transaction) rất dễ lấy nhầm handle
// của context kia. Trước đây `txOf` IM LẶNG rơi về pool ⇒ ghi ra NGOÀI
// transaction, merge nửa vời không để lại dấu vết.
var errTxMismatch = errors.New("tx handle sai context — không phải do repository này tạo")

// txOf trả *gorm.DB đúng phạm vi:
//   - `tx == nil` → gọi ngoài transaction, dùng pool (hợp lệ cho unit test của
//     application khi `uow` không bind).
//   - handle hợp lệ → *gorm.DB của transaction.
//   - handle sai kiểu → LỖI, không rơi về pool.
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

// txDB là viết ngắn cho `txOf(...).WithContext(ctx)` ở các method ghi.
func txDB(root *gorm.DB, ctx context.Context, tx app.Tx) (*gorm.DB, error) {
	db, err := txOf(root, tx)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx), nil
}

// errNoRows chuẩn hoá lỗi "không tìm thấy" của GORM thành sentinel của
// application, để use case bọc thành 404 với message tiếng Việt.
func errNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return app.ErrNotFound
	}
	return err
}

// isNoRows báo lỗi có phải "query không trả dòng nào" không. Seed loader dùng
// nó để phân biệt "chưa có row" với lỗi hệ thống — `sql.ErrNoRows` khi quét
// bằng `Row().Scan()`.
func isNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows)
}

// Repository hiện thực app.Repository.
type Repository struct {
	db *gorm.DB
}

// NewRepository dựng repository trên pool GORM.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Repository = (*Repository)(nil)

// ── Path ────────────────────────────────────────────────────────────────────

// CreatePath insert 1 path. `guid` BẮT BUỘC khác rỗng: `ux_roadmap_paths_guid`
// UNIQUE trên `NOT NULL DEFAULT ”` — 2 path cùng guid rỗng là đụng nhau.
func (r *Repository) CreatePath(ctx context.Context, tx app.Tx, p *app.Path) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := pathFromApp(*p)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*p = pathToApp(row)
	return nil
}

// ListPaths trả mọi path còn sống, sắp theo (language, id) — giữ đúng thứ tự
// v1 trả về để UI không nhảy dòng sau mỗi lần thêm path.
func (r *Repository) ListPaths(ctx context.Context) ([]app.Path, error) {
	var rows []Path
	err := r.db.WithContext(ctx).Where("deleted = 0").Order("language, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap paths: %w", err)
	}
	out := make([]app.Path, 0, len(rows))
	for _, row := range rows {
		out = append(out, pathToApp(row))
	}
	return out, nil
}

// PathBySlug trả 1 path theo slug, app.ErrNotFound nếu đã xoá mềm.
func (r *Repository) PathBySlug(ctx context.Context, slug string) (app.Path, error) {
	var row Path
	err := r.db.WithContext(ctx).Where("slug = ? AND deleted = 0", slug).Take(&row).Error
	if err != nil {
		return app.Path{}, errNoRows(err)
	}
	return pathToApp(row), nil
}

// PathByID trả 1 path theo id.
func (r *Repository) PathByID(ctx context.Context, id int64) (app.Path, error) {
	var row Path
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Path{}, errNoRows(err)
	}
	return pathToApp(row), nil
}

// UpdatePath ghi các cột sửa được. `slug` KHÔNG nằm trong danh sách: slug là
// natural key của seed, đổi nó sẽ khiến seed tạo path trùng ở lần boot sau.
func (r *Repository) UpdatePath(ctx context.Context, tx app.Tx, p *app.Path) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := pathFromApp(*p)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"language":   row.Language,
			"title":      row.Title,
			"overview":   row.Overview,
			"is_builtin": row.IsBuiltin,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2): đọc bằng pool sẽ trả row cũ.
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*p = pathToApp(row)
	return nil
}

// SoftDeletePath đánh dấu xoá path (tombstone cho sync) và xoá mềm luôn toàn bộ
// nhánh con — cascade nằm ở repository vì nó là các câu UPDATE theo khoá ngoại,
// đặt ở use case sẽ khiến tầng application phải biết cấu trúc bảng.
func (r *Repository) SoftDeletePath(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Path{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	return r.softDeleteTreeOfPath(ctx, db, id)
}

// softDeleteTreeOfPath xoá mềm 3 tầng dưới path, theo thứ tự từ sâu lên để
// subquery luôn thấy row chưa xoá.
func (r *Repository) softDeleteTreeOfPath(ctx context.Context, db *gorm.DB, pathID int64) error {
	stmts := []struct {
		table  string
		column string
		query  string
	}{
		{"roadmap_resources", "topic_id",
			"UPDATE roadmap_resources SET deleted = 1 WHERE deleted = 0 AND topic_id IN " +
				"(SELECT t.id FROM roadmap_topics t JOIN roadmap_stages s ON s.id = t.stage_id WHERE s.path_id = ?)"},
		{"roadmap_topics", "stage_id",
			"UPDATE roadmap_topics SET deleted = 1 WHERE deleted = 0 AND stage_id IN " +
				"(SELECT id FROM roadmap_stages WHERE path_id = ?)"},
		{"roadmap_milestones", "stage_id",
			"UPDATE roadmap_milestones SET deleted = 1 WHERE deleted = 0 AND stage_id IN " +
				"(SELECT id FROM roadmap_stages WHERE path_id = ?)"},
	}
	for _, s := range stmts {
		if err := db.Exec(s.query, pathID).Error; err != nil {
			return fmt.Errorf("soft delete %s: %w", s.table, err)
		}
	}
	return nil
}

// ── Stage ───────────────────────────────────────────────────────────────────

// CreateStage insert 1 stage.
func (r *Repository) CreateStage(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*s = stageToApp(row)
	return nil
}

// ListStages trả stage của path, sắp theo (position, id).
func (r *Repository) ListStages(ctx context.Context, pathID int64) ([]app.Stage, error) {
	var rows []Stage
	err := r.db.WithContext(ctx).Where("path_id = ? AND deleted = 0", pathID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap stages: %w", err)
	}
	out := make([]app.Stage, 0, len(rows))
	for _, row := range rows {
		out = append(out, stageToApp(row))
	}
	return out, nil
}

// StageByID trả 1 stage.
func (r *Repository) StageByID(ctx context.Context, id int64) (app.Stage, error) {
	var row Stage
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Stage{}, errNoRows(err)
	}
	return stageToApp(row), nil
}

// UpdateStage ghi các cột sửa được.
func (r *Repository) UpdateStage(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"slug":           row.Slug,
			"title":          row.Title,
			"goal":           row.Goal,
			"position":       row.Position,
			"duration_weeks": row.DurationWeeks,
			"deck_id":        row.DeckID,
			"terrain":        row.Terrain,
			"direction":      row.Direction,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2) để lấy `updated_at` mà trigger vừa chạm.
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*s = stageToApp(row)
	return nil
}

// UpdateStageStatus ghi status + status_note + completed_at trong 1 UPDATE rồi
// làm mới `s` bằng row đọc lại TRONG CÙNG transaction.
//
// `completed_at` là *string: nil ghi SQL NULL, KHÔNG ghi "" — "" sẽ lọt vào
// `WHERE completed_at IS NOT NULL` và phá `progress?since=` (F11 M1).
//
// Không set updated_at tường minh → trigger chạm (status là thay đổi cần lan
// sang máy peer). Caller sở hữu `s` và đã đọc nó trong cùng tx, nên nhận con
// trỏ là đủ — không cần `id`/`status`/`note`/`completedAt` làm tham số riêng.
func (r *Repository) UpdateStageStatus(ctx context.Context, tx app.Tx, s *app.Stage) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := stageFromApp(*s)
	if err := db.Model(&row).Where("deleted = 0").Omit("updated_at").
		Updates(map[string]any{
			"status":       row.Status,
			"status_note":  row.StatusNote,
			"completed_at": row.CompletedAt,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*s = stageToApp(row)
	return nil
}

// SoftDeleteStage đánh dấu xoá stage và toàn bộ nhánh con (milestone, topic,
// resource) — cascade nằm ở repository vì đó là các câu UPDATE theo khoá
// ngoại, tầng application không nên biết cấu trúc bảng.
func (r *Repository) SoftDeleteStage(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Stage{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	// Thứ tự sâu → nông: subquery của tầng sau phải thấy row chưa xoá.
	if err := db.Exec(
		"UPDATE roadmap_resources SET deleted = 1 WHERE deleted = 0 AND topic_id IN "+
			"(SELECT id FROM roadmap_topics WHERE stage_id = ?)", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_resources: %w", err)
	}
	if err := db.Exec(
		"UPDATE roadmap_topics SET deleted = 1 WHERE deleted = 0 AND stage_id = ?", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_topics: %w", err)
	}
	if err := db.Exec(
		"UPDATE roadmap_milestones SET deleted = 1 WHERE deleted = 0 AND stage_id = ?", id).Error; err != nil {
		return fmt.Errorf("soft delete roadmap_milestones: %w", err)
	}
	return nil
}

// MaxStagePosition trả position lớn nhất trong path, -1 nếu chưa có stage —
// để use case tính position kế tiếp (max+1) khi client không gửi.
func (r *Repository) MaxStagePosition(ctx context.Context, pathID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Stage{}).
		Where("path_id = ? AND deleted = 0", pathID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max stage position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Topic ───────────────────────────────────────────────────────────────────

// CreateTopic insert 1 topic.
func (r *Repository) CreateTopic(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*t = topicToApp(row)
	return nil
}

// ListTopics trả topic của stage, sắp theo (position, id).
func (r *Repository) ListTopics(ctx context.Context, stageID int64) ([]app.Topic, error) {
	var rows []Topic
	err := r.db.WithContext(ctx).Where("stage_id = ? AND deleted = 0", stageID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap topics: %w", err)
	}
	out := make([]app.Topic, 0, len(rows))
	for _, row := range rows {
		out = append(out, topicToApp(row))
	}
	return out, nil
}

// TopicByID trả 1 topic.
func (r *Repository) TopicByID(ctx context.Context, id int64) (app.Topic, error) {
	var row Topic
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Topic{}, errNoRows(err)
	}
	return topicToApp(row), nil
}

// UpdateTopic ghi các cột sửa được.
func (r *Repository) UpdateTopic(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":       row.Title,
			"why":         row.Why,
			"activities":  row.Activities,
			"position":    row.Position,
			"is_optional": row.IsOptional,
			// map_x/map_y là *float64: nil ghi SQL NULL thật, nên `ClearMap`
			// của application xoá được toạ độ (xem service.go).
			"map_x": row.MapX,
			"map_y": row.MapY,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2) — cũng xác nhận map_x/map_y đã về NULL khi
	// ClearMap, thay vì GORM bỏ sót nil khỏi SET.
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*t = topicToApp(row)
	return nil
}

// UpdateTopicStatus ghi status + status_note + completed_at rồi làm mới `t`
// bằng row đọc lại TRONG CÙNG transaction (xem UpdateStageStatus).
func (r *Repository) UpdateTopicStatus(ctx context.Context, tx app.Tx, t *app.Topic) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := topicFromApp(*t)
	if err := db.Model(&row).Where("deleted = 0").Omit("updated_at").
		Updates(map[string]any{
			"status":       row.Status,
			"status_note":  row.StatusNote,
			"completed_at": row.CompletedAt,
		}).Error; err != nil {
		return err
	}
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*t = topicToApp(row)
	return nil
}

// SoftDeleteTopic đánh dấu xoá topic và resource của nó.
func (r *Repository) SoftDeleteTopic(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	if err := db.Model(&Topic{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error; err != nil {
		return err
	}
	return errNoRows(db.Model(&Resource{}).Where("topic_id = ? AND deleted = 0", id).
		UpdateColumn("deleted", 1).Error)
}

// MaxTopicPosition trả position lớn nhất trong stage, -1 nếu chưa có topic.
func (r *Repository) MaxTopicPosition(ctx context.Context, stageID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Topic{}).
		Where("stage_id = ? AND deleted = 0", stageID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max topic position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Resource ────────────────────────────────────────────────────────────────

// CreateResource insert 1 tài liệu.
func (r *Repository) CreateResource(ctx context.Context, tx app.Tx, rs *app.Resource) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := resFromApp(*rs)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*rs = resToApp(row)
	return nil
}

// ListResources trả tài liệu của topic, sắp theo (position, id).
func (r *Repository) ListResources(ctx context.Context, topicID int64) ([]app.Resource, error) {
	var rows []Resource
	err := r.db.WithContext(ctx).Where("topic_id = ? AND deleted = 0", topicID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap resources: %w", err)
	}
	out := make([]app.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, resToApp(row))
	}
	return out, nil
}

// ResourceByID trả 1 tài liệu.
func (r *Repository) ResourceByID(ctx context.Context, id int64) (app.Resource, error) {
	var row Resource
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Resource{}, errNoRows(err)
	}
	return resToApp(row), nil
}

// UpdateResource ghi các cột sửa được.
func (r *Repository) UpdateResource(ctx context.Context, tx app.Tx, rs *app.Resource) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := resFromApp(*rs)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":    row.Title,
			"url":      row.URL,
			"kind":     row.Kind,
			"note":     row.Note,
			"position": row.Position,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2) — cũng xác nhận `url` đã về NULL khi
	// `ClearURL`, vì GORM bỏ qua nil trong map.
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*rs = resToApp(row)
	return nil
}

// SoftDeleteResource đánh dấu xoá tài liệu.
func (r *Repository) SoftDeleteResource(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&Resource{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}

// MaxResourcePosition trả position lớn nhất trong topic, -1 nếu chưa có.
func (r *Repository) MaxResourcePosition(ctx context.Context, topicID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Resource{}).
		Where("topic_id = ? AND deleted = 0", topicID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max resource position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Milestone ───────────────────────────────────────────────────────────────

// CreateMilestone insert 1 mốc nhỏ.
func (r *Repository) CreateMilestone(ctx context.Context, tx app.Tx, m *app.Milestone) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := msFromApp(*m)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*m = msToApp(row)
	return nil
}

// ListMilestones trả mốc của stage, sắp theo (position, id).
func (r *Repository) ListMilestones(ctx context.Context, stageID int64) ([]app.Milestone, error) {
	var rows []Milestone
	err := r.db.WithContext(ctx).Where("stage_id = ? AND deleted = 0", stageID).
		Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list roadmap milestones: %w", err)
	}
	out := make([]app.Milestone, 0, len(rows))
	for _, row := range rows {
		out = append(out, msToApp(row))
	}
	return out, nil
}

// MilestoneByID trả 1 mốc.
func (r *Repository) MilestoneByID(ctx context.Context, id int64) (app.Milestone, error) {
	var row Milestone
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error
	if err != nil {
		return app.Milestone{}, errNoRows(err)
	}
	return msToApp(row), nil
}

// UpdateMilestone ghi text + position.
func (r *Repository) UpdateMilestone(ctx context.Context, tx app.Tx, m *app.Milestone) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := msFromApp(*m)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"text":     row.Text,
			"position": row.Position,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2).
	if err := db.Where("id = ? AND deleted = 0", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*m = msToApp(row)
	return nil
}

// SoftDeleteMilestone đánh dấu xoá mốc.
func (r *Repository) SoftDeleteMilestone(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&Milestone{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}

// MaxMilestonePosition trả position lớn nhất trong stage, -1 nếu chưa có.
func (r *Repository) MaxMilestonePosition(ctx context.Context, stageID int64) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&Milestone{}).
		Where("stage_id = ? AND deleted = 0", stageID).
		Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max milestone position: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// ── Bookmark ────────────────────────────────────────────────────────────────

// CreateBookmark insert 1 bookmark. `guid` BẮT BUỘC khác rỗng:
// `ux_roadmap_bookmarks_guid` UNIQUE trên `NOT NULL DEFAULT ”` (xem
// `CreatePath` — cùng lý do).
func (r *Repository) CreateBookmark(ctx context.Context, tx app.Tx, b *app.Bookmark) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := bmFromApp(*b)
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	*b = bmToApp(row)
	return nil
}

// ListBookmarks trả bookmark còn sống, sắp theo `id` tăng dần.
//
// Lọc tag dùng `position(',' || ? || ',' in ',' || tags || ',') > 0` — dấu
// phẩy biên ở CẢ HAI vế là điều kiện bắt buộc: `tags LIKE '%a%'` sẽ khớp
// bookmark mang tag `ab`, và người dùng lọc tag "a" thấy kết quả của "abc"
// là hỏng. `position` (không phải `LIKE`) vì còn tránh được chuyện `%`/`_`
// trong tên tag bị hiểu là wildcard.
//
// Cột `tags` ghi bởi `domain.EncodeTags` KHÔNG có khoảng trắng quanh dấu
// phẩy, nên so khớp ở đây là so chính xác từng phần tử. Bản thuần của điều
// kiện này là `domain.TagsContain` — test viền `a` vs `ab` chạy trên cả hai.
func (r *Repository) ListBookmarks(ctx context.Context, filter app.BookmarkFilter) ([]app.Bookmark, error) {
	q := r.db.WithContext(ctx).Model(&bookmarkRow{}).Where("deleted = 0")
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Tag != "" {
		q = q.Where("position(',' || ? || ',' in ',' || tags || ',') > 0", filter.Tag)
	}
	var rows []bookmarkRow
	if err := q.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list roadmap bookmarks: %w", err)
	}
	out := make([]app.Bookmark, 0, len(rows))
	for _, row := range rows {
		out = append(out, bmToApp(row))
	}
	return out, nil
}

// BookmarkByID trả 1 bookmark, `app.ErrNotFound` nếu đã xoá mềm.
func (r *Repository) BookmarkByID(ctx context.Context, id int64) (app.Bookmark, error) {
	var row bookmarkRow
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted = 0", id).Take(&row).Error; err != nil {
		return app.Bookmark{}, errNoRows(err)
	}
	return bmToApp(row), nil
}

// UpdateBookmark ghi các cột sửa được + `status`. `status` nằm chung đường ghi
// với `SetBookmarkStatus` vì bookmark không có `completed_at` cần luật riêng —
// khác stage/topic, 2 hàm đó phải set/clear mốc.
func (r *Repository) UpdateBookmark(ctx context.Context, tx app.Tx, b *app.Bookmark) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	row := bmFromApp(*b)
	if err := db.Model(&row).Omit("updated_at").
		Updates(map[string]any{
			"title":  row.Title,
			"url":    row.URL,
			"note":   row.Note,
			"tags":   row.Tags,
			"status": row.Status,
		}).Error; err != nil {
		return err
	}
	// Đọc lại TRONG tx (luật 2): đọc bằng pool sẽ trả row cũ.
	if err := db.Where("id = ?", row.ID).Take(&row).Error; err != nil {
		return errNoRows(err)
	}
	*b = bmToApp(row)
	return nil
}

// SoftDeleteBookmark đánh dấu xoá (tombstone cho sync).
func (r *Repository) SoftDeleteBookmark(ctx context.Context, tx app.Tx, id int64) error {
	db, err := txDB(r.db, ctx, tx)
	if err != nil {
		return err
	}
	return db.Model(&bookmarkRow{ID: id}).Where("deleted = 0").
		UpdateColumn("deleted", 1).Error
}
