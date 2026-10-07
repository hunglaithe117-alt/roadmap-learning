package roadmapinfra

// Test nội bộ của package (không phải `roadmapinfra_test`) vì cần nắm được
// `txOf` để đọc lại TRONG transaction — mà đọc ngoài transaction thì thấy
// dữ liệu CŨ, tức là chính cái bug mà test này chặn.

import (
	"context"
	"testing"

	"gorm.io/gorm"

	app "langapp/internal/application/roadmap"
	"langapp/internal/platform/testdb"
)

// openTestDBForInternalTest dựng schema Postgres tạm cho test nội bộ.
//
// Dùng chung `platform/testdb` (không tự viết bản sao) vì 2 lý do:
//  1. `testdb.Open` nắm khoá advisory `MigrateLockKey` quanh DỰNG → MIGRATE →
//     DỌN. Bản sao tự viết sẽ chạy migration `CREATE EXTENSION pg_trgm`
//     song song với package khác → `duplicate key ... pg_extension_name_index`
//     (SQLSTATE 23505). Đây chính là lỗi đã xảy ra khi còn 2 bản sao.
//  2. Schema tạm trong DSN phải được dựng TRƯỚC khi pool mở kết nối; `testdb`
//     đã xử lý sẵn thứ tự này.
func openTestDBForInternalTest(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, context.Background())
}

// Test_readback_inside_transaction_sees_new_values là test HỒI QUY của P0.
//
// Nếu `UpdateStage` đọc lại bằng pool thay vì `txOf`, thì sau `UpdateStage` mà
// TRƯỚC commit, đọc lại cùng transaction phải thấy dữ liệu CŨ — chứng minh
// ngay bằng truy vấn trong `Do`, không cần đoán qua hành vi của test khác.
func Test_readback_inside_transaction_sees_new_values(t *testing.T) {
	db := openTestDBForInternalTest(t)
	ctx := context.Background()
	repo := NewRepository(db)
	uow := NewUnitOfWork(db)

	// Tạo 1 path + 1 stage ngoài transaction (đường đọc/ghi đơn lẻ).
	var pathID, stageID int64
	requireNoErr(t, uow.Do(ctx, func(tx app.Tx) error {
		p := app.Path{Slug: "p", Title: "P", Language: "zh", GUID: app.NewGUID(),
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := repo.CreatePath(ctx, tx, &p); err != nil {
			return err
		}
		pathID = p.ID
		s := app.Stage{PathID: p.ID, Slug: "g", Title: "Tiêu đề cũ", Position: 0,
			Status: app.StatusNotStarted, Terrain: "meadow", Direction: "up",
			CreatedAt: "2026-01-01T00:00:00Z", GUID: app.NewGUID(), UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := repo.CreateStage(ctx, tx, &s); err != nil {
			return err
		}
		stageID = s.ID
		return nil
	}))

	// Trong 1 transaction: sửa stage, rồi đọc lại CHÍNH trong transaction đó.
	var inTxTitle, inTxTerrain string
	var sawNewValue bool
	requireNoErr(t, uow.Do(ctx, func(tx app.Tx) error {
		st, err := repo.StageByID(ctx, stageID)
		if err != nil {
			return err
		}
		st.Title = "Tiêu đề MỚI"
		st.Terrain = "volcano"
		if err := repo.UpdateStage(ctx, tx, &st); err != nil {
			return err
		}

		// Giá trị mà repository TRẢ VỀ phải là giá trị vừa ghi.
		if st.Title != "Tiêu đề MỚI" || st.Terrain != "volcano" {
			t.Errorf("UpdateStage trả về giá trị cũ: title=%q terrain=%q", st.Title, st.Terrain)
		}

		// Đọc lại bằng chính connection của transaction, trước commit.
		tb, err := txOf(db, tx)
		if err != nil {
			return err
		}
		var row Stage
		if err := tb.WithContext(ctx).Where("id = ?", stageID).Take(&row).Error; err != nil {
			return err
		}
		inTxTitle, inTxTerrain = row.Title, row.Terrain
		sawNewValue = row.Title == "Tiêu đề MỚI" && row.Terrain == "volcano"
		return nil
	}))

	if !sawNewValue {
		t.Errorf("đọc lại TRONG transaction: got title=%q terrain=%q, want title=%q terrain=%q — "+
			"nghĩa là UpdateStage đã ghi ra ngoài transaction",
			inTxTitle, inTxTerrain, "Tiêu đề MỚI", "volcano")
	}
	_ = pathID

	// Sau commit thì pool cũng phải thấy giá trị mới (không phải vì test này,
	// mà để chắc chắn 2 phép đọc ở trên đang so sánh đúng dữ liệu).
	var committed string
	if err := db.Raw("SELECT title FROM roadmap_stages WHERE id = ?", stageID).
		Scan(&committed).Error; err != nil {
		t.Fatalf("đọc sau commit: %v", err)
	}
	if committed != "Tiêu đề MỚI" {
		t.Errorf("sau commit: got %q, want %q", committed, "Tiêu đề MỚI")
	}
}

// Test_tx_of_rejects_foreign_handle chốt F2: handle không do package này tạo
// phải ra lỗi, KHÔNG im lặng rơi về pool.
func Test_tx_of_rejects_foreign_handle(t *testing.T) {
	db := openTestDBForInternalTest(t)

	if _, err := txOf(db, nil); err != nil {
		t.Errorf("tx == nil là đường hợp lệ (gọi ngoài transaction), không được lỗi: %v", err)
	}
	if _, err := txOf(db, "handle của context khác"); err == nil {
		t.Error("tx handle sai kiểu phải trả lỗi, không được rơi về pool")
	}
	if _, err := txOf(db, struct{ ID int }{1}); err == nil {
		t.Error("tx handle sai kiểu phải trả lỗi, không được rơi về pool")
	}
	// Nhánh "handle đúng kiểu nhưng tx bên trong nil" không dựng được từ đây
	// sau khi gom về `txtx`: test nội bộ của package `txtx` chốt nhánh đó.

	// Handle hợp lệ thì qua, và trả về đúng *gorm.DB của transaction.
	if err := NewUnitOfWork(db).Do(context.Background(), func(tx app.Tx) error {
		got, err := txOf(db, tx)
		if err != nil {
			return err
		}
		if got == db {
			t.Error("txOf phải trả *gorm.DB của transaction, không phải của pool")
		}
		return nil
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("UnitOfWork.Do: %v", err)
	}
}
