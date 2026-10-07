package txtx

// Test nội bộ (`package txtx`, không phải `txtx_test`) vì cần dựng được
// `handle{}` rỗng để chốt nhánh "handle đúng kiểu nhưng tx bên trong nil" —
// nhánh này chỉ quan sát được từ bên trong package.

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"langapp/internal/platform/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// openSchema dựng 1 schema Postgres tạm bằng `testdb.Acquire` — transaction
// phải test trên Postgres THẬT, không dùng mock: đây là cơ chế bảo vệ dữ liệu.
func openSchema(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	s := testdb.Acquire(t, ctx)
	db, _ := s.OpenSchema(t, ctx)
	return db
}

func countDecks(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM decks").Scan(&n).Error; err != nil {
		t.Fatalf("đếm row `decks`: %v", err)
	}
	return n
}

// Test_do_rollback_khi_callback_tra_error chốt: callback trả error ⇒ rollback,
// database KHÔNG đổi. Assert trên hàng thật (không phải "hàm trả lỗi").
func Test_do_rollback_khi_callback_tra_error(t *testing.T) {
	db := openSchema(t)
	ctx := context.Background()
	boom := errors.New("boom")

	err := Do(ctx, db, func(tx any) error {
		h, err := Of(db, tx)
		if err != nil {
			return err
		}
		if e := h.Exec(`INSERT INTO decks (name, created_at) VALUES (?, ?)`, "x", "t").Error; e != nil {
			return e
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Do phải trả nguyên error của callback, got %v", err)
	}
	if n := countDecks(t, db); n != 0 {
		t.Errorf("rollback phải không để lại row nào, got %d row trong `decks`", n)
	}
}

// Test_do_commit_khi_callback_tra_nil chốt: callback trả nil ⇒ commit, row sống.
func Test_do_commit_khi_callback_tra_nil(t *testing.T) {
	db := openSchema(t)
	ctx := context.Background()

	if err := Do(ctx, db, func(tx any) error {
		h, err := Of(db, tx)
		if err != nil {
			return err
		}
		return h.Exec(`INSERT INTO decks (name, created_at) VALUES (?, ?)`, "x", "t").Error
	}); err != nil {
		t.Fatalf("Do (commit): %v", err)
	}
	if n := countDecks(t, db); n != 1 {
		t.Errorf("commit phải giữ đúng 1 row, got %d", n)
	}
}

// Test_of_with_nil_handle_tra_db_goc chốt: tx == nil là đường hợp lệ (gọi
// ngoài transaction), phải trả đúng `db` gốc và không lỗi.
func Test_of_with_nil_handle_tra_db_goc(t *testing.T) {
	db := openSchema(t)

	got, err := Of(db, nil)
	if err != nil {
		t.Fatalf("tx == nil không được lỗi, got %v", err)
	}
	if got != db {
		t.Errorf("tx == nil phải trả đúng db gốc, got %p want %p", got, db)
	}
}

// Test_of_with_foreign_handle_returns_error_not_root_db là test LOAD-BEARING.
//
// Nếu ai đó đổi `Of` thành `return db, nil` khi handle lạ, mọi test khác vẫn
// xanh — chỉ test này bắt được. Đó là hành vi làm nên `ErrTxMismatch` tồn tại:
// fail loud thay vì ghi nhầm ra NGOÀI transaction.
func Test_of_with_foreign_handle_returns_error_not_root_db(t *testing.T) {
	db := openSchema(t)

	cases := map[string]any{
		"chuỗi":              "không phải handle",
		"struct lạ":          struct{ ID int }{1},
		"handle rỗng":        handle{},
		"handle không có tx": handle{tx: nil},
	}
	for name, foreign := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Of(db, foreign)
			if !errors.Is(err, ErrTxMismatch) {
				t.Errorf("%s: phải trả ErrTxMismatch, got err=%v", name, err)
			}
			if got != nil {
				t.Errorf("%s: KHÔNG được rơi về db gốc, got %p", name, got)
			}
		})
	}

	// Handle hợp lệ thì qua, và trả đúng *gorm.DB của transaction (khác pool).
	if err := Do(context.Background(), db, func(tx any) error {
		got, err := Of(db, tx)
		if err != nil {
			return err
		}
		if got == db {
			t.Error("handle hợp lệ phải trả *gorm.DB của transaction, không phải của pool")
		}
		return nil
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
}
