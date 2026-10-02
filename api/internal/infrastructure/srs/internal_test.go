package srsinfra

// Test nội bộ của package (không phải `srsinfra_test`) vì cần nắm được
// `txHandle` để đọc lại TRONG transaction — mà đọc ngoài transaction thì thấy
// dữ liệu CŨ, tức là chính cái bug P0 mà test này chặn.

import (
	"context"
	"testing"

	"gorm.io/gorm"

	app "langapp/internal/application/srs"
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

// Test_update_card_readback_inside_transaction_sees_new_values là test HỒI QUY
// của P0 cho `srs`. Trước đây `UpdateCard` chỉ ghi mà không đọc lại, nên
// `SetCardTone` trả về struct cũ và không ai assert giá trị trả về.
func Test_update_card_readback_inside_transaction_sees_new_values(t *testing.T) {
	db := openTestDBForInternalTest(t)
	ctx := context.Background()
	repo := NewRepository(db)
	uow := NewUnitOfWork(db)

	var cardID int64
	if err := uow.Do(ctx, func(tx app.Tx) error {
		d := app.Deck{Name: "D", Lang: "zh", GUID: app.NewGUID(),
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := repo.CreateDeck(ctx, tx, &d); err != nil {
			return err
		}
		c := app.Card{DeckID: d.ID, Front: "f", Back: "b", State: "new",
			DueAt: "2026-01-02T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z",
			GUID: app.NewGUID(), UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := repo.CreateCard(ctx, tx, &c); err != nil {
			return err
		}
		cardID = c.ID
		return nil
	}); err != nil {
		t.Fatalf("UnitOfWork.Do (setup): %v", err)
	}

	var inTxTone string
	if err := uow.Do(ctx, func(tx app.Tx) error {
		c, err := repo.CardByID(ctx, cardID)
		if err != nil {
			return err
		}
		tone := "ní"
		c.Tone = &tone
		if err := repo.UpdateCard(ctx, tx, &c); err != nil {
			return err
		}
		if c.Tone == nil || *c.Tone != "ní" {
			t.Errorf("UpdateCard trả về giá trị cũ: tone=%v", c.Tone)
		}
		tb, err := txOf(db, tx)
		if err != nil {
			return err
		}
		var row Card
		if err := tb.WithContext(ctx).Where("id = ?", cardID).Take(&row).Error; err != nil {
			return err
		}
		if row.Tone == nil {
			t.Error("đọc lại TRONG transaction phải thấy tone mới, got NULL")
		} else {
			inTxTone = *row.Tone
		}
		return nil
	}); err != nil {
		t.Fatalf("UnitOfWork.Do: %v", err)
	}
	if inTxTone != "ní" {
		t.Errorf("trong transaction phải thấy %q, got %q", "ní", inTxTone)
	}
}

// Test_tx_of_rejects_foreign_handle chốt F2 cho context srs: handle do context
// khác tạo phải ra lỗi, KHÔNG im lặng rơi về pool (M3 `sync` sẽ trộn handle
// `roadmap` với handle `srs` trong cùng 1 transaction).
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
	if _, err := txOf(db, txHandle{}); err == nil {
		t.Error("txHandle rỗng (tx = nil bên trong) phải trả lỗi")
	}
}

// Test_wrong_tx_handle_writes_nothing là F2 ở mức DB: truyền handle tùy ý cho
// mọi method ghi thì không được có bất kỳ thay đổi nào lọt ra ngoài.
func Test_wrong_tx_handle_writes_nothing(t *testing.T) {
	db := openTestDBForInternalTest(t)
	ctx := context.Background()
	repo := NewRepository(db)
	bad := "foreign-handle"

	d := app.Deck{Name: "D", Lang: "zh", GUID: app.NewGUID(), CreatedAt: "c", UpdatedAt: "c"}
	if err := repo.CreateDeck(ctx, bad, &d); err == nil {
		t.Error("CreateDeck với handle sai phải trả lỗi")
	}
	if err := repo.SoftDeleteDeck(ctx, bad, 1); err == nil {
		t.Error("SoftDeleteDeck với handle sai phải trả lỗi")
	}
	c := app.Card{DeckID: 1, Front: "f", Back: "b", State: "new", DueAt: "d", CreatedAt: "c", GUID: "g", UpdatedAt: "c"}
	if err := repo.CreateCard(ctx, bad, &c); err == nil {
		t.Error("CreateCard với handle sai phải trả lỗi")
	}
	if err := repo.UpdateCard(ctx, bad, &c); err == nil {
		t.Error("UpdateCard với handle sai phải trả lỗi")
	}
	if err := repo.SoftDeleteCard(ctx, bad, 1); err == nil {
		t.Error("SoftDeleteCard với handle sai phải trả lỗi")
	}
	rv := app.Review{CardID: 1, Grade: 3, ReviewedAt: "r", NextDueAt: "n", GUID: "g"}
	if err := repo.CreateReview(ctx, bad, &rv); err == nil {
		t.Error("CreateReview với handle sai phải trả lỗi")
	}

	var n int
	if err := db.Raw("SELECT COUNT(*) FROM decks").Scan(&n).Error; err != nil {
		t.Fatalf("đếm row: %v", err)
	}
	if n != 0 {
		t.Errorf("handle sai KHÔNG được ghi ra ngoài transaction, có %d row trong `decks`", n)
	}
}
