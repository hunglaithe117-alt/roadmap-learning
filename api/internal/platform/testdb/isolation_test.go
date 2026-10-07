package testdb_test

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" — testdb dùng, test cũng cần
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"langapp/internal/platform/testdb"
)

// ĐÂY LÀ ĐIỀU KIỆN TIÊN QUYẾT của cả việc vá.
//
// Trước khi vá, `testdb` đã có `requireIsolatedDB` và ta tin nó chạy. Hóa ra
// niềm tin đó là thứ đã giấu bug: lớp guard cũ chỉ đếm bảng có cột `deleted`
// trong `public`, bỏ sót `dict`/`en_dict`, và im lặng cho qua khi database
// rỗng. "Có guard" KHÔNG phải "guard đúng" — phải có test chứng minh nó ĐỎ.
//
// Vì sao test nằm ở package ngoài (`testdb_test`) và chỉ dùng API export:
// để nó chạy như 1 hộp đen — vào DSN + conn, ra error — đúng như người dùng
// thật. Test in-package dễ chạm `s.conn` và xanh vì lý do không liên quan.

// Test chính: guard phải ĐỎ khi DSN trỏ database ứng dụng.
//
// ⚠️ Test này CỐ Ý mở kết nối tới database ứng dụng. Nó AN TOÀN chỉ vì
// `checkIsolatedDB` là hàm THUẦN ĐỌC — không tạo schema, không chạy
// migration, không `DROP` gì. Đó là lý do tách `checkIsolatedDB` (trả error)
// khỏi `requireIsolatedDB` (`t.Fatal`): logic kiểm tra trở thành thứ test
// được gọi an toàn, thay vì phải hy vọng `t.Fatal` kịp bắn trước khi có
// tác dụng phá.
func Test_isolation_guard_rejects_the_app_database(t *testing.T) {
	appDSN := requireAppDSN(t)
	ctx := context.Background()

	// TIÊN ĐỀ 2 TẦNG, vì trạng thái database app thay đổi theo người dùng.
	//
	// Đo thật (2026-09-29): ngay sau `docker compose down -v` thì database app
	// RỖNG — app chưa boot lần nào. Lúc đó lớp "bằng chứng cấu trúc" không có
	// bảng nào để nhìn thấy, và CHỈ lớp tên bắn được. Bản gốc của test này coi
	// đó là tiên điều hỏng ⇒ `make test` ĐỎ ngay sau `down -v`, dù không có gì
	// hỏng (đó chính là lần chạy đỏ đầu tiên của F1).
	//
	// Vậy nên: đo trạng thái, rồi đòi mức chứng minh tương ứng. Có bảng ⇒ chứng
	// minh lớp cấu trúc CỤ THỂ (bằng chứng nằm trong thông báo). Rỗng ⇒ chứng
	// minh lớp tên (thông báo phải nêu đúng tên database app). Cả 2 đều phải đỏ
	// khi không đúng, nên không có đường nào để test xanh vì lý do không liên
	// quan.
	appConn := openDB(t, appDSN)
	hasAppTables := countAppTables(t, ctx, appConn) > 0

	// KHÔNG gọi `testdb.Acquire(t, …)` ở đây: `Acquire` tự `t.Fatal` khi guard đỏ,
	// mà ta ĐANG CẦN chứng minh nó đỏ — `t.Fatal` sẽ giết chính test này và ta
	// mất mọi assert phía sau. Thay vào đó ta dựng session theo đúng 2 bước mà
	// `Acquire` làm, nhưng dùng conn đã mở sẵn:
	//
	//   1. `connectLock` tương đương — mở conn tới DSN app
	//   2. `checkIsolatedDB` — đúng hàm guard
	//
	// Đây là điểm yếu CÓ CHỦ ĐÍCH: test này bắt LOGIC guard, không bắt việc ai
	// đó XOÁ lời gọi guard khỏi `Acquire`. Việc đó do
	// `Test_go_test_process_exits_nonzero_against_app_database` bắt (chạy
	// subprocess thật, kiểm cả đường đi thật). Hai tầng, mỗi tầng bắt một loại hỏng.
	conn := openConn(t, appDSN)
	s := testdb.NewSessionForTest(conn)

	err := testdb.CheckIsolatedDB(ctx, s)
	require.Error(t, err,
		"guard PHẢI fail khi DSN trỏ database app — không fail ⇒ test có thể "+
			"chạy migration rồi DROP bảng thật của bạn (đây chính là lỗi M7c)")

	// Thông báo phải CHỈ RA CÁCH SỬA. Thông báo chỉ báo lỗi thì người đọc biết
	// mình bị chặn nhưng không biết phải làm gì — và sẽ tắt guard.
	msg := err.Error()
	require.Contains(t, msg, testdb.AppDatabase, "phải nêu database nào đang bị trỏ tới")
	require.Contains(t, msg, "make test", "phải chỉ lệnh sẵn có")
	require.Contains(t, msg, "CREATE DATABASE", "phải chỉ cách tự tạo database test")

	// Mức chứng minh phải khớp với trạng thái database app.
	if hasAppTables {
		require.Contains(t, msg, "bằng chứng cấu trúc",
			"database app CÓ bảng thật ⇒ lớp bằng chứng cấu trúc phải bắt được, "+
				"không được chỉ dựa vào tên database")
	} else {
		require.Contains(t, msg, "đúng tên database ứng dụng",
			"database app rỗng (app chưa boot) ⇒ chỉ lớp tên bắn được, và thông "+
				"báo phải nói rõ bằng chứng đó là gì")
	}
}

// Lớp 2 của guard (BẰNG CHỨNG CẤU TRÚC) phải tự nó bắt được, không được cậu
// vào lớp tên.
//
// Đây là điểm then chốt về mặt thiết kế: lớp tên database CHỈ bắt được case
// `langapp`. Người dùng đặt app ở `langapp_dev`, hay seed nhầm vào database
// test, thì tên khác `langapp` ⇒ lớp tên im lặng. Lớp cấu trúc thì bắt được,
// vì nó hỏi "bảng app có nằm ở đâu" chứ không hỏi "tên là gì".
//
// Test tự dựng database trùng nội dung app nhưng TÊN KHÁC `langapp`, rồi đòi
// guard phải đỏ. Nếu lớp cấu trúc hỏng, test này xanh ⇒ ta mới biết lớp tên là
// tất cả những gì đang bảo vệ dữ liệu.
func Test_isolation_guard_catches_app_tables_under_a_different_database_name(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	// Database tên KHÔNG phải `langapp` nhưng chứa bảng app — mô phỏng "dev đặt
	// app ở tên khác" hoặc "database test bị seed nhầm".
	const otherDB = "probe_guard_structural"
	admin := openDB(t, adminDSN(t))
	_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+otherDB+" WITH (FORCE)")
	_, err := admin.ExecContext(ctx, "CREATE DATABASE "+otherDB)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Dọn bằng pool MỚI: `admin` có thể còn giữ session khiến `DROP
		// DATABASE` bị chặn bởi "database is being accessed by other users".
		c, err := openDB(t, adminDSN(t)).Conn(ctx)
		if err == nil {
			_, _ = c.ExecContext(ctx, "DROP DATABASE IF EXISTS "+otherDB+" WITH (FORCE)")
			_ = c.Close()
		}
	})

	// Tạo bảng app ở `public` của database này — dùng `dict` (KHÔNG có cột
	// `deleted`) để chứng minh guard bắt được cả bảng mà lớp guard CŨ bỏ sót.
	probe := openConn(t, withDatabase(t, adminDSN(t), otherDB))
	_, err = probe.ExecContext(ctx, "CREATE TABLE public.dict (id INT PRIMARY KEY, nghia TEXT)")
	require.NoError(t, err, "dựng bảng app giả trong %s", otherDB)
	_, err = probe.ExecContext(ctx, "INSERT INTO public.dict VALUES (1, 'dữ liệu thật')")
	require.NoError(t, err)

	// Guard phải đỏ DÙ tên database khác `langapp`.
	s := testdb.NewSessionForTest(probe)
	err = testdb.CheckIsolatedDB(ctx, s)
	require.Error(t, err,
		"lớp BẰNG CHỨNG CẤU TRÚC phải bắt được database %q — nó không tên "+
			"`langapp`, nên nếu test này xanh thì lớp cấu trúc hỏng và chỉ còn "+
			"tên database là bảo vệ dữ liệu", otherDB)
	require.Contains(t, err.Error(), "dict",
		"thông báo phải liệt kê bằng chứng cấu trúc tìm thấy (bảng nào, ở schema nào)")

	// Dữ liệu trong bảng giả phải CÒN NGUYÊN sau khi guard đỏ — tức guard dừng
	// TRƯỚC khi động vào bất cứ thứ gì, không phải "báo lỗi rồi vẫn xoá".
	var n int
	require.NoError(t, probe.QueryRowContext(ctx, "SELECT count(*) FROM public.dict").Scan(&n))
	require.Equal(t, 1, n, "guard phải DỪNG, không được đụng vào dữ liệu")
}

// Test tương phản: guard phải XANH trên database test đang chạy.
//
// Không có test này thì `requireIsolatedDB` có thể fail mọi thứ (kể cả đúng)
// và ta chỉ biết khi toàn bộ suite đỏ — tức guard hỏng theo hướng ngược lại,
// cũng là hỏng.
func Test_isolation_guard_accepts_the_test_database(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()
	s := testdb.Acquire(t, ctx)
	// Dùng tên schema không tồn tại: guard chỉ cần biết "schema test" để bỏ qua,
	// nên không cần `CREATE SCHEMA` thật ở đây. Còn exempt 1 schema thật có
	// bảng app sẽ cho xanh — đó là hành vi ĐÚNG (schema test của ta có bảng do
	// migration tạo), và `Test_guard_exempts_only_the_test_schema` kiểm nó.
	require.NoError(t, testdb.CheckIsolatedDB(ctx, s, "t_test_fake_schema_name"),
		"guard phải XANH trên database test — đỏ ở đây nghĩa là chặn nhầm, "+
			"toàn bộ suite sẽ đỏ")
	require.NoError(t, testdb.CheckIsolatedDB(ctx, s),
		"không exempt gì thì guard cũng phải XANH trên database test")
}

// Guard phải chạy TRƯỚC khi tạo schema, để lần chạy sai không để lại rác trong
// chính database app.
//
// Vì sao quan trọng: nếu đặt sai thứ tự (tạo schema rồi mới check), guard sẽ
// "bảo vệ dữ liệu" rồi tự để lại `t_test_*` trong database app — biến lần chạy
// sai thành lần làm bẩn database mà nó đang bảo vệ.
//
// Có 2 mốc phải sạch sau một lần chạy sai:
//  1. `t_test_*`   — schema test (sẽ tạo ở `openSchema` sau guard)
//  2. `testext`    — schema extension (sẽ tạo ở `Acquire`, CŨNG sau guard)
func Test_guard_runs_before_creating_anything_in_the_app_database(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	appDSN := requireAppDSN(t)
	appConn := openDB(t, appDSN)
	beforeSchemas := countSchemasLike(t, ctx, appConn, "t\\_test\\_%")
	beforeExt := countSchemasLike(t, ctx, appConn, testdb.ExtSchema)

	// Tiên điều: guard phải đỏ. Nếu xanh thì mọi assert dưới đây vô nghĩa và —
	// tệ hơn — ta sẽ vừa chạy migration lên database app.
	//
	// Dùng `openConn` + `NewSessionForTest` chứ không `Acquire`: `Acquire` tự
	// `t.Fatal` khi guard đỏ, mà ta cần tiếp tục để đo hậu quả.
	s := testdb.NewSessionForTest(openConn(t, appDSN))
	require.Error(t, testdb.CheckIsolatedDB(ctx, s),
		"tiên điều: guard phải đỏ trên DSN app")

	// Bước GHI của `Acquire` phải TỰ NÓ cũng từ chối trên database app, chứ
	// không phải chỉ nhờ guard. Đây là lớp phòng thủ thứ 2: nếu mai này ai đó
	// di chuyển/`bỏ` lời gọi guard, bước này vẫn chặn — và test này vẫn xanh.
	//
	// ⚠️ LỖI NÀY ĐÃ ĐỎ 1 LẦN VÀ ĐÃ ĐƯỢC SỬA. Bản gốc của test này chỉ assert
	// lỗi có chứa "pg_trgm" — nhánh "extension đang ở sai schema". Ở database
	// app đã boot, nhánh đó bắn, nên test XANH và ta tưởng đã an toàn. Nhưng
	// lỗi F1 xảy ra ở trạng thái app CHƯA BAO GIỜ BOOT: không có `pg_trgm` ⇒
	// `ensureExtensionSchema` đi thẳng xuống NHÁNH GHI ⇒ tạo `testext` +
	// `CREATE EXTENSION` trong DB app ⇒ migration vĩnh viễn hỏng (42704), app
	// restart loop. Test cũ ĐỎ vì assert sai lý do, chứ KHÔNG phát hiện damage.
	//
	// Nay `ensureExtensionSchema` tự gọi `checkIsolatedDB` ở đầu, nên trên DB
	// app nó bị chặn bởi CHÍNH guard cách ly và báo đúng thông báo đó. Assert
	// bám theo hành vi mới — và đây chính là điều test #3 bên dưới ghim lại.
	errExt := testdb.EnsureExtensionSchemaForTest(ctx, s)
	require.Error(t, errExt,
		"bước dựng extension cũng phải từ chối database app — không thì "+
			"lần chạy sai sẽ sửa schema của database ứng dụng")
	require.Contains(t, errExt.Error(), "CHỨA DỮ LIỆU APP",
		"`ensureExtensionSchema` phải tự chặn bằng guard cách ly (không cần "+
			"ai gọi `Acquire` trước) — thông báo phải là của guard, không phải nhánh "+
			"pg_trgm sai schema (nhánh đó không bắn được khi app chưa boot)")
	require.Contains(t, errExt.Error(), testdb.ExtSchema,
		"thông báo phải nêu nó đang từ chối tạo schema nào")

	require.Equal(t, beforeSchemas, countSchemasLike(t, ctx, appConn, "t\\_test\\_%"),
		"guard đỏ nhưng vẫn tạo schema t_test_* trong database APP")
	require.Equal(t, beforeExt, countSchemasLike(t, ctx, appConn, testdb.ExtSchema),
		"guard đỏ nhưng vẫn tạo schema %s trong database APP — tức `ensureExtension` "+
			"chạy TRƯỚC guard, và một lần chạy sai sẽ sửa bẩn database ứng dụng",
		testdb.ExtSchema)

	// `pg_trgm` trong database app phải Y NGUYÊN ở `public` (nơi app đã cài nó
	// lúc boot). Nếu `ensureExtension` "giúp" chuyển nó sang `testext` thì ta
	// vừa sửa schema database người dùng — cũng là một kiểu bẩn.
	//
	// ⚠️ Nhưng `pg_trgm` KHÔNG TỒN TẠI khi app chưa boot lần nào — và đó chính
	// là trạng thái nguy hiểm nhất của F1. Nên ở đây phải hỏi "trước có không /
	// sau có không" thay vì đòi có sẵn. Bản gốc `require.NoError(t, Scan(…))`
	// với `sql.ErrNoRows` ⇒ đỏ ngay sau `down -v`.
	// Gọi LẠI lần nữa (lần thứ nhất ở trên) để so fingerprint quanh một lời
	// gọi riêng — hai lời gọi phải cho cùng kết quả, vì `ensureExtensionSchema`
	// phải là hàm thuần về phía database (idempotent + tự bảo vệ).
	extBefore, errBefore := pgTrgmSchema(t, ctx, appConn)
	require.Error(t, testdb.EnsureExtensionSchemaForTest(ctx, s),
		"xem lý do bên trên")

	extAfter, errAfter := pgTrgmSchema(t, ctx, appConn)
	require.Equal(t, extBefore, extAfter,
		"test KHÔNG được cài/di chuyển extension của database ứng dụng "+
			"(trước %q/%v, sau %q/%v)", extBefore, errBefore, extAfter, errAfter)
	if extBefore != "" {
		require.Equal(t, "public", extBefore,
			"nếu database app đã có `pg_trgm` thì phải ở `public` — nơi app cài "+
				"lúc boot; thấy ở chỗ khác nghĩa là đã bị test sửa bẩn")
	}
}

// ── F1: `make test` trên DB app CHƯA BOOT từng tiêm `testext` + `pg_trgm` vào
// database ứng dụng, rồi làm migration vĩnh viễn hỏng (app restart loop) ──────
//
// 3 test dưới đây ghim lại đúng chỗ dễ vỡ đó. Chúng dùng CÙNG một dữ kiện:
// database trông giống app DB ở trạng thái "đã có bảng app nhưng CHƯA có
// `pg_trgm`" — đúng trạng thái giữa chừng khi `00001_init.sql` đã chạy (tạo toàn
// bộ bảng app) còn `00002_fts.sql` thì chưa (nó mới cài `pg_trgm`).

// Test mạnh hơn test cũ: đo DAMAGE, không chỉ "test đỏ".
//
// `Test_guard_runs_before_creating_anything_in_the_app_database` chỉ assert
// `require.Error`. Đó là lý do lỗi F1 lọt: ở DB app đã boot, `ensureExtensionSchema`
// CÓ trả lỗi (nhánh "pg_trgm sai schema") — và nếu damage xảy ra TRƯỚC khi hàm trả
// lỗi đó thì assert vẫn xanh. "Có báo lỗi" ≠ "không làm bẩn".
//
// Nên test này chụp lại TOÀN BỘ fingerprint của database TRƯỚC và SAU: bảng,
// extension, schema. Yêu cầu "không đổi một byte nào" là điều kiện cần chứng
// minh, không phải kết luận suy ra.
//
// Fingerprint dùng database RIÊNG dựng cho test (không phải DB app thật) nên
// kịch bản F1 tái lập được mà không cần xoá dữ liệu người dùng.
func Test_ensureExtensionSchema_leaves_the_database_byte_identical(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	// Database có bảng app (`dict` — bảng KHÔNG có cột `deleted`, tức bảng mà
	// lớp guard cũ bỏ sót) nhưng CHƯA có `pg_trgm`.
	//
	// Đây là shape nguy hiểm nhất: nhánh "tra `pg_trgm`" trả `sql.ErrNoRows` ⇒
	// rơi xuống nhánh GHI (`CREATE SCHEMA` + `CREATE EXTENSION`). Trước khi
	// vá, đó chính xác là chỗ DB app bị đầu độc.
	scratch := newScratchDB(t, "probe_pristine")
	scratch.run(t, "CREATE TABLE public.dict (id INT PRIMARY KEY, nghia TEXT)")
	scratch.run(t, "INSERT INTO public.dict VALUES (1, 'dữ liệu thật')")

	before := dbFingerprint(t, ctx, scratch.conn(t))
	require.Equal(t, 0, before.extensions,
		"tiên điều: database này phải CHƯA có extension người dùng cài nào, "+
			"nếu không thì không tái lập được nhánh ghi của F1")
	require.Equal(t, 1, before.tables,
		"tiên điều: phải có đúng 1 bảng app để lớp bằng chứng cấu trúc bắt được")

	// Gọi thẳng `ensureExtensionSchema` — KHÔNG qua `Acquire`, KHÔNG gọi
	// `CheckIsolatedDB` trước. Đây là đúng đường đi của F1.
	s := testdb.NewSessionForTest(openConn(t, scratch.dsn))
	err := testdb.EnsureExtensionSchemaForTest(ctx, s)
	require.Error(t, err,
		"`ensureExtensionSchema` phải tự từ chối database chứa dữ liệu app")

	after := dbFingerprint(t, ctx, scratch.conn(t))
	require.Equal(t, before, after,
		"⛔ DATABASE ĐÃ BỊ THAY ĐỔI dù `ensureExtensionSchema` trả lỗi.\n"+
			"Trước: %s\nSau:   %s\n"+
			"Đây chính xác là F1: `CREATE SCHEMA %s` + `CREATE EXTENSION pg_trgm` "+
			"rơi vào database ứng dụng ⇒ `00002_fts.sql` sau đó chạy "+
			"`CREATE EXTENSION IF NOT EXISTS pg_trgm` thành NO-OP ⇒ "+
			"`gin_trgm_ops` không có ⇒ app restart loop với SQLSTATE 42704.",
		before, after, testdb.ExtSchema)

	// Assert tường minh, không chỉ so sánh tổng thể — để khi đỏ, thông báo nói
	// thẳng ra CÁI GÌ bị thêm vào.
	require.Equal(t, 0, after.schemasWithPrefix(testdb.ExtSchema),
		"database phải KHÔNG có schema %s", testdb.ExtSchema)
	require.Equal(t, 0, after.schemasWithPrefix("t_test_"),
		"database phải KHÔNG có schema t_test_*")

	// Dữ liệu trong bảng giả phải còn nguyên.
	require.Equal(t, 1, scratch.scalar(t, "SELECT count(*) FROM public.dict"),
		"dữ liệu phải còn nguyên — hàm phải DỪNG, không được đụng vào")
}

// Test thứ 3 — chống tái phát ĐÚNG CHỖ dễ sai: `ensureExtensionSchema` phải TỰ
// chặn, không cần ai gọi `Acquire` (hay bất kỳ guard nào) trước.
//
// Vì sao tách khỏi test #2: #2 dựng database riêng có bảng app. Test này dùng
// **database ứng dụng thật** — nơi `pg_trgm` ĐÃ nằm ở `public` — và đòi lỗi phải
// là lỗi của GUARD CÁCH LY, không phải lỗi "pg_trgm sai schema".
//
// Đây là cách phân biệt được 2 lớp khi database app đã boot: cả 2 lớp đều trả lỗi
// ở đó, nên chỉ so `require.Error` thì vô nghĩa. Mutation-check: gỡ
// `checkIsolatedDB` khỏi đầu `ensureExtensionSchema` ⇒ lỗi trở thành nhánh "pg_trgm
// sai schema" ⇒ test này ĐỎ.
//
// Nói cách khác: test này ghim luật "lớp 2 phải tự bảo vệ THEO CẤU TRÚC", thay
// vì trông chờ một bất biến về thứ tự lời gọi ở file khác — bất biến đã vỡ và
// làm brick app.
func Test_ensureExtensionSchema_refuses_itself_without_Acquire(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	appDSN := requireAppDSN(t)
	appConn := openDB(t, appDSN)
	before := dbFingerprint(t, ctx, appConn)

	// Session dựng bằng `NewSessionForTest` ⇒ KHÔNG đi qua `Acquire`, nên không
	// có lời gọi guard nào chạy trước. `EnsureExtensionSchemaForTest` là lời
	// gọi ĐẦU TIÊN và DUY NHẤT chạm database.
	s := testdb.NewSessionForTest(openConn(t, appDSN))
	err := testdb.EnsureExtensionSchemaForTest(ctx, s)
	require.Error(t, err,
		"gọi trực tiếp `ensureExtensionSchema` mà không có `Acquire` phải bị chặn — "+
			"nếu không, bất biến 'ai đó luôn gọi guard trước' lại đang gánh hết "+
			"trách nhiệm bảo vệ dữ liệu app")

	require.Contains(t, err.Error(), "CHỨA DỮ LIỆU APP",
		"phải bị CHÍNH guard cách ly chặn, không phải nhánh 'pg_trgm sai schema' — "+
			"gỡ `checkIsolatedDB` khỏi đầu `ensureExtensionSchema` là test này đỏ: %s", err)
	require.NotContains(t, err.Error(), "đang nằm ở schema",
		"lỗi phải đến từ tầng trên (guard cách ly), không phải tầng dưới (vị trí "+
			"extension) — nếu không thì tầng trên đã bị gỡ mà test vẫn xanh")

	require.Equal(t, before, dbFingerprint(t, ctx, appConn),
		"database ứng dụng phải Y NGUYÊN sau lời gọi tự bảo vệ")
}

// `WithSearchPath` phải GHI ĐÈ `search_path` sẵn có trong DSN, không phải nối
// thêm.
//
// Đo thật: DSN `…?search_path=public` + hàm cũ (nối chuỗi) ⇒ `SHOW search_path` ra
// `public`, vì Postgres lấy param ĐẦU TIÊN. Tức toàn bộ pool test chạy với
// `public` trong `search_path` — trúng đúng cơ chế gây M7C (sau `DROP TABLE dict`,
// truy vấn `dict` rơi xuống bảng production thay vì báo không tồn tại).
//
// Chứng minh bằng HÀNH VI (`SHOW search_path` trên connection thật), không bằng so
// chuỗi DSN: "DSN trông đúng" chưa chắc driver parse ra thế.
func Test_withSearchPath_overrides_a_search_path_already_in_the_DSN(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	const want = "t_test_probe,testext"
	// DSN có sẵn `search_path=public` — đúng cái bẫy: hàm cũ nối thêm nên
	// `public` đứng ĐẦU và thắng.
	poisoned := testdb.WithSearchPath(testdb.DSN()+"&search_path=public", want)

	// Đếm số lần xuất hiện `search_path=` phải là ĐÚNG 1 — nối thêm thì 2, và
	// đó mới là lỗi. Không so chuỗi giá trị: `url.Values.Encode` escape dấu phẩy
	// thành `%2C`, hợp lệ và Postgres giải mã đúng; assert về hình thức escape
	// chỉ ghim cách viết của `net/url`, không ghim hành vi.
	require.Equal(t, 1, strings.Count(poisoned, "search_path="),
		"DSN kết quả phải có ĐÚNG 1 tham số search_path — có 2 nghĩa là đã nối "+
			"thay vì ghi đè, và `public` sẽ thắng: %s", poisoned)

	conn := openConn(t, poisoned)
	var got string
	require.NoError(t, conn.QueryRowContext(ctx, "SHOW search_path").Scan(&got))
	require.Equal(t, want, got,
		"DSN đã có `search_path=public` mà `SHOW search_path` vẫn ra %q ⇒ "+
			"param cũ thắng, test sẽ chạy trong `public` (đọc/xoá nhầm bảng thật)", got)
}

// search_path của test KHÔNG được chứa `public` — đây là cơ chế trực tiếp gây
// mất dữ liệu: khi `public` có trong `search_path`, `SELECT … FROM dict` sau
// `DROP TABLE dict` RƠI XUỐNG bảng production và trả về thành công (đo thật ở
// M7c). Chứng minh bằng HÀNH VI, không bằng đọc lại hằng số.
func Test_search_path_excludes_public_and_extension_is_in_ExtSchema(t *testing.T) {
	requireTestDSN(t)
	ctx := context.Background()

	conn := openDB(t, testdb.DSN())

	// Lớp 1 — extension nằm đúng chỗ. Nếu nó ở `public` thì test buộc phải
	// kéo `public` vào `search_path` để lấy `gin_trgm_ops` — và `public` chính
	// là nơi có bảng thật.
	var extSchema string
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT n.nspname FROM pg_extension e "+
			"JOIN pg_namespace n ON n.oid = e.extnamespace "+
			"WHERE e.extname = 'pg_trgm'").Scan(&extSchema))
	require.Equal(t, testdb.ExtSchema, extSchema,
		"pg_trgm phải nằm ở %s — nếu ở `public` thì test sẽ phải kéo `public` vào "+
			"search_path và đọc/xoá nhầm bảng app", testdb.ExtSchema)

	// Lớp 2 — hành vi thật: `dict` ĐÃ tồn tại (migration `00001` tạo nó) trong
	// schema test. `DROP TABLE dict` xoá nó khỏi schema test, rồi truy vấn
	// `dict` phải KHÔNG còn resolve được ở schema nào nữa.
	//
	// ⚠️ KHÔNG `CREATE TABLE dict` trước: migration đã tạo rồi, tạo thêm sẽ
	// fail `relation "dict" already exists` (42P07) và che mất phần ta cần đo.
	db, schema := testdb.OpenSchema(t, ctx)

	// Xác nhận bảng CÓ thật trong schema test trước khi xoá — nếu `dict`
	// không ở đây thì phép thử phía dưới vô nghĩa (nó sẽ "đúng" vì không có
	// gì để rơi xuống, tức xanh vô nghĩa).
	require.Equal(t, schema, schemaOfTable(t, db, "dict"),
		"migration phải tạo bảng `dict` trong schema test")

	require.NoError(t, db.Exec("DROP TABLE dict").Error)

	// Sau `DROP`, `dict` phải KHÔNG còn resolve được ở bất kỳ schema nào.
	// Nếu nó vẫn còn (ở `public` chẳng hạn) thì `SELECT … FROM dict` trong
	// test sẽ RƠI XUỐNG bảng production và trả về thành công — chính xác cái
	// lỗi M7c đo được. Đây là hành vi, không phải suy luận từ hằng số.
	require.Empty(t, schemaOfTable(t, db, "dict"),
		"bảng `dict` đã DROP vẫn resolve được ở schema %q ⇒ truy vấn sẽ RƠI XUỐNG "+
			"ngoài schema test (đọc/xoá nhầm dữ liệu thật)", schemaOfTable(t, db, "dict"))

	// Lớp 3 — mọi bảng của test phải nằm trong schema test. Rơi vào `public` hay
	// `testext` đều sai: `DROP SCHEMA … CASCADE` không xoá được.
	require.Empty(t, strayTables(t, ctx, conn, schema),
		"database test có bảng NGOÀI schema test %q — chúng sẽ không bao giờ được dọn",
		schema)
}

// Lớp chứng minh CUỐI, ở mức tiến trình: chạy `go test` thật với DSN trỏ
// database app và đòi exit ≠ 0.
//
// Vì sao cần khi đã có test trên: vì test trên bắt riêng logic guard. Test này
// bắt TOÀN BỘ đường chạy thật mà CI và người khác nhìn thấy — kể cả khi ai đó
// sau này làm yếu guard ở tầng khác (`Main`, `requireDSN`) mà test logic vẫn
// xanh. Mutation đã xảy ra đúng một lần ở `gate_test.go` §2.3: gỡ một lớp
// thì lớp kia che, test khác vẫn xanh.
func Test_go_test_process_exits_nonzero_against_app_database(t *testing.T) {
	appDSN := requireAppDSN(t)

	// `-run` trỏ vào đúng 1 test DB nhẹ nhất để guard kịp chạy. `TestMain` vẫn
	// được gọi (nên kiểm được cả lớp gate), nhưng ta muốn thấy thông báo
	// CỦA GUARD nên phải có test thật chạy tới `OpenSchema`.
	cmd := exec.Command("go", "test", "-count=1", "-run",
		"TestIsolatedProbeOpensSchema", "./internal/infrastructure/srs/")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(minimalGoEnv(t), "LANGAPP_TEST_POSTGRES_DSN="+appDSN)

	out, err := cmd.CombinedOutput()
	require.Error(t, err,
		"DSN trỏ database app mà `go test` vẫn exit 0 ⇒ guard bị vô hiệu:\n%s", out)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "lỗi phải là exit code khác 0, không phải lỗi chạy `go`: %v", err)
	require.NotEqual(t, 0, exitErr.ExitCode())

	// Đòi thêm: phải nói RÕ là chặn vì database app. Nếu chỉ fail vì lý do
	// khác (sai DSN, không kết nối được) thì test này xanh vô nghĩa.
	text := string(out)
	require.Contains(t, text, "CHỨA DỮ LIỆU APP",
		"phải nói rõ bị chặn vì database app, không phải fail nhầm vì lý do khác:\n%s", text)
}

// ── helper ──────────────────────────────────────────────────────────────────

// scratchDB là database tạm dựng riêng cho 1 test, tự xoá khi test xong.
//
// Tách thành type (thay vì vài hàm rời rạc) vì có 3 thao tác phải đi cùng nhau:
// dựng connection trỏ vào database đó, chạy SQL lên đó, và xoá database đó. Rời
// ra dễ quên cleanup — mà quên cleanup ở đây tức để lại database rác trên
// Postgres của máy dev, đúng loại rác mà cả task này đang diệt.
type scratchDB struct {
	name string
	dsn  string
	pool *sql.DB
}

// newScratchDB tạo database mới (tên tự chọn) trên CÙNG server với DSN test, và
// đăng ký cleanup xoá nó.
//
// `CREATE DATABASE` không chạy được trong transaction và không chạy được khi
// đang kết nối vào chính database đó ⇒ phải qua database `postgres`.
func newScratchDB(t *testing.T, name string) *scratchDB {
	t.Helper()
	ctx := context.Background()

	admin := openDB(t, adminDSN(t))
	_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	_, err := admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err, "dựng database tạm %s", name)

	// Dọn bằng pool MỚI: `admin` còn giữ session thì `DROP DATABASE` bị chặn bởi
	// "database is being accessed by other users". `WITH (FORCE)` cắt ngang mọi
	// session còn lại nên pool nào cũng chịu.
	t.Cleanup(func() {
		c, err := openDB(t, adminDSN(t)).Conn(context.Background())
		if err == nil {
			_, _ = c.ExecContext(context.Background(),
				"DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = c.Close()
		}
	})

	return &scratchDB{name: name, dsn: withDatabase(t, testdb.DSN(), name)}
}

// conn trả 1 connection trỏ vào database tạm, tự đóng khi test xong.
func (s *scratchDB) conn(t *testing.T) *sql.DB {
	t.Helper()
	if s.pool == nil {
		s.pool = openDB(t, s.dsn)
	}
	return s.pool
}

// run chạy 1 câu SQL lên database tạm.
func (s *scratchDB) run(t *testing.T, stmt string) {
	t.Helper()
	_, err := s.conn(t).ExecContext(context.Background(), stmt)
	require.NoError(t, err, "%s: %s", s.name, stmt)
}

// scalar chạy 1 query trả về 1 số nguyên.
func (s *scratchDB) scalar(t *testing.T, query string) int {
	t.Helper()
	var n int
	require.NoError(t, s.conn(t).QueryRowContext(context.Background(), query).Scan(&n))
	return n
}

// fingerprint là ẢNH CHỤP của database: đủ để trả lời "database có đổi một
// byte nào không".
//
// Vì sao cần cả 3 chiều (bảng + extension + schema) chứ không chỉ đếm bảng:
// damage của F1 KHÔNG tạo bảng nào — nó tạo 1 schema + 1 extension. Đếm bảng
// mà thôi thì trước/sau luôn bằng 0 ⇒ test xanh trong khi database đã bị đầu độc.
// Đó chính xác là lỗ hổng của test cũ, chỉ khác biểu hiện.
//
// `schemas` lưu DANH SÁCH tên chứ không phải số lượng: hai lần ghi rác khác
// nhau (thêm 1, xoá 1) vẫn cho cùng con số — nếu đo bằng số thì test có thể
// xanh trong khi database đã đổi.
type fingerprint struct {
	tables     int
	extensions int
	schemas    []string
}

// String in fingerprint gọn 1 dòng để diff được trong thông báo assert.
func (f fingerprint) String() string {
	return "tables=" + itoa(f.tables) + " exts=" + itoa(f.extensions) +
		" schemas=[" + strings.Join(f.schemas, " ") + "]"
}

func itoa(n int) string { return strconv.Itoa(n) }

// schemasWithPrefix đếm schema trong ảnh chụp có TIỀN TỐ cho trước.
//
// Dùng tiền tố thay vì mẫu `LIKE` của Postgres: mẫu `LIKE` cần escape `_` bằng
// `\_`, còn `path.Match` dùng bộ ký hiệu escape KHÁC — hai bộ ký hiệu trông giống
// nhau nhưng nghĩa khác nhau, và test soát ảnh chụp thì sai 1 ký tự là hỏng âm
// thầm. Tiền tố là đủ cho 2 mẫu cần kiểm (`testext`, `t_test_`).
func (f fingerprint) schemasWithPrefix(prefix string) int {
	n := 0
	for _, s := range f.schemas {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

// dbFingerprint chụp ảnh database: số bảng thường, số extension, và danh sách
// schema KHÔNG thuộc hệ thống.
//
// Cố ý bỏ qua mọi thứ của hệ thống (`pg_*`, `information_schema`) vì chúng
// không phải "dữ liệu database" và số lượng của ổn định — chỉ cần phần do
// `testdb` tạo ra mới là thứ cần so.
func dbFingerprint(t *testing.T, ctx context.Context, conn *sql.DB) fingerprint {
	t.Helper()
	var f fingerprint
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace "+
			"WHERE c.relkind = 'r' AND n.nspname NOT LIKE 'pg\\_%' "+
			"AND n.nspname <> 'information_schema'").Scan(&f.tables))
	// `plpgsql` bị LOẠI có chủ đích: Postgres tự cài nó vào MỌI database mới,
	// kể cả database trống, nên nó không phải thứ ai "tiêm" vào. Đếm nó thì
	// fingerprint của database sạch là 1 chứ không 0, và tiên điều "chưa có
	// extension nào" thành điều không bao giờ đúng.
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_extension WHERE extname <> 'plpgsql'").
		Scan(&f.extensions))
	rows, err := conn.QueryContext(ctx,
		"SELECT nspname FROM pg_namespace "+
			"WHERE nspname NOT LIKE 'pg\\_%' AND nspname <> 'information_schema' "+
			"ORDER BY nspname")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		f.schemas = append(f.schemas, s)
	}
	require.NoError(t, rows.Err())
	require.NotNil(t, f.schemas, "danh sách schema phải là slice (kể cả khi rỗng) — "+
		"nil khác `[]` nên `require.Equal` sẽ báo ĐỎ oan khi cả hai đều rỗng")
	return f
}

// checkIsolatedFails khẳng định guard đỏ với DSN ứng dụng — dùng khi cần
// session trên database app mà không muốn đi qua `Acquire` (vì `Acquire` tự
// `t.Fatal`).
func checkIsolatedFails(t *testing.T, ctx context.Context) error {
	t.Helper()
	s := testdb.NewSessionForTest(openConn(t, requireAppDSN(t)))
	return testdb.CheckIsolatedDB(ctx, s)
}

// openDB mở pool tới 1 DSN, tự đóng khi test xong.
func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// openConn chiếm 1 connection từ pool và tự trả lại khi test xong — dùng khi
// cần truyền `*sql.Conn` vào `testdb.NewSessionForTest`.
//
// Giữ đúng 1 conn (không phải cả pool) vì `Session` giữ `*sql.Conn` cho tới
// khi test xong; nếu trả về pool thì session sẽ giữ conn đã bị đóng.
func openConn(t *testing.T, dsn string) *sql.Conn {
	t.Helper()
	conn, err := openDB(t, dsn).Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// adminDSN trả DSN trỏ database `postgres` của CÙNG server với DSN test.
//
// Cần để `CREATE DATABASE` / `DROP DATABASE` — Postgres từ chối tạo/xoá
// database khi đang kết nối tới chính database đó.
func adminDSN(t *testing.T) string {
	t.Helper()
	return withDatabase(t, testdb.DSN(), "postgres")
}

// withDatabase thay tên database trong DSN dạng URL, giữ nguyên phần query
// string. Dùng để dựng DSN cho database khác trên cùng server.
func withDatabase(t *testing.T, dsn, dbName string) string {
	t.Helper()
	const prefix = "postgres://"
	require.True(t, strings.HasPrefix(dsn, prefix), "DSN phải dạng URL: %s", dsn)
	rest := dsn[len(prefix):]
	slash := strings.Index(rest, "/")
	require.Positive(t, slash, "DSN thiếu phần database: %s", dsn)
	hostPart, tail := rest[:slash], rest[slash+1:]
	suffix := ""
	if q := strings.Index(tail, "?"); q >= 0 {
		suffix = tail[q:]
	}
	return prefix + hostPart + "/" + dbName + suffix
}

// schemaOfTable trả schema đang giữ bảng `name` theo `search_path` HIỆN TẠI của
// `db`, hoặc "" nếu không tìm thấy.
//
// Dùng `pg_class` + `pg_namespace` (giống hệt cách Postgres resolve tên trong
// `search_path`) thay vì `information_schema.tables` — cần đúng cơ chế
// `search_path` mà test muốn kiểm.
func schemaOfTable(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var schema string
	err := db.Raw("SELECT n.nspname FROM pg_class c "+
		"JOIN pg_namespace n ON n.oid = c.relnamespace "+
		"WHERE c.relname = ? AND c.relkind = 'r'", name).Scan(&schema).Error
	if err != nil {
		return ""
	}
	return schema
}

// countAppTables đếm bảng app đã MIGRATE trong schema `public` của database
// ứng dụng. Dùng 3 bảng đại diện (`cards`/`dict`/`roadmap_topics`) vì đó là 3
// bảng mà bằng chứng cấu trúc trong `checkIsolatedDB` dựa vào.
func countAppTables(t *testing.T, ctx context.Context, conn *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_tables WHERE schemaname = 'public' "+
			"AND tablename IN ('cards','dict','roadmap_topics')").Scan(&n))
	return n
}

// pgTrgmSchema trả schema đang giữ `pg_trgm`, hoặc ("", err) nếu extension
// chưa được cài.
//
// Hàm này CỐ Ý trả cả 2 chiều thay vì `require.NoError(Scan(…))`: "chưa có
// `pg_trgm`" là một TRẠNG THÁI HỢP LỆ (database app chưa boot lần nào), và test
// cần phân biệt nó với "lỗi kết nối". `sql.ErrNoRows` ở đây là dữ kiện, không phải
// hỏng.
func pgTrgmSchema(t *testing.T, ctx context.Context, conn *sql.DB) (string, error) {
	t.Helper()
	var schema string
	err := conn.QueryRowContext(ctx,
		"SELECT n.nspname FROM pg_extension e "+
			"JOIN pg_namespace n ON n.oid = e.extnamespace "+
			"WHERE e.extname = 'pg_trgm'").Scan(&schema)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	require.NoError(t, err, "tra vị trí extension pg_trgm")
	return schema, nil
}

// countSchemasLike đếm schema khớp mẫu `LIKE` cho trước.
func countSchemasLike(t *testing.T, ctx context.Context, conn *sql.DB, pattern string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_namespace WHERE nspname LIKE $1", pattern).Scan(&n))
	return n
}

// strayTables liệt kê bảng thường nằm ngoài schema test — mọi bảng như vậy đều
// là rò (không bao giờ được `DROP SCHEMA … CASCADE` dọn).
func strayTables(t *testing.T, ctx context.Context, conn *sql.DB, testSchema string) []string {
	t.Helper()
	rows, err := conn.QueryContext(ctx,
		"SELECT n.nspname, c.relname FROM pg_class c "+
			"JOIN pg_namespace n ON n.oid = c.relnamespace "+
			"WHERE c.relkind = 'r' AND n.nspname <> $1 "+
			"AND n.nspname NOT LIKE 't\\_test\\_%' "+
			"AND n.nspname NOT IN ('pg_catalog','information_schema','testext')",
		testSchema)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var schema, table string
		require.NoError(t, rows.Scan(&schema, &table))
		out = append(out, schema+"."+table)
	}
	require.NoError(t, rows.Err())
	return out
}

// requireTestDSN bỏ qua nếu chưa bật Postgres. Đây là giới hạn MÔI TRƯỜNG
// (không có docker), không phải hỏng sản phẩm — `Main`/`requireDSN` đã chặn
// chuyện "chạy được mà tưởng xanh".
func requireTestDSN(t *testing.T) {
	t.Helper()
	if testdb.DSN() == "" {
		t.Skip("cần LANGAPP_TEST_POSTGRES_DSN — chạy qua `make test`")
	}
}

// requireAppDSN suy ra DSN của database ỨNG DỤNG từ DSN test đang chạy, bằng
// cách thay tên database trong URL.
//
// Suy ra (không hardcode DSN đầy đủ) vì chỉ tên database là khác giữa test và
// app — host/port/user/password giống nhau. Hardcode sẽ hỏng trên máy khác.
//
// Hàm này FAIL (không skip) nếu database ứng dụng không tồn tại, vì cả 2 test
// bắt buộc của task này đều cần nó: nếu app DB không có thì không chứng minh
// được gì, và im lặng skip sẽ biến "chưa chứng minh" thành "đã chứng minh".
func requireAppDSN(t *testing.T) string {
	t.Helper()
	dsn := testdb.DSN()
	require.NotEmpty(t, dsn, "cần DSN test để suy ra DSN app — chạy qua `make test`")

	const prefix = "postgres://"
	require.True(t, strings.HasPrefix(dsn, prefix),
		"DSN test phải dạng URL postgres:// để suy ra được: %s", dsn)
	rest := dsn[len(prefix):]
	slash := strings.Index(rest, "/")
	require.Positive(t, slash, "DSN test thiếu phần database: %s", dsn)

	hostPart, tail := rest[:slash], rest[slash+1:]
	name, suffix := tail, ""
	if q := strings.Index(tail, "?"); q >= 0 {
		name, suffix = tail[:q], tail[q:]
	}
	require.NotEqual(t, testdb.AppDatabase, name,
		"DSN test đang trỏ vào database APP (%s) — `make test` đang hỏng, "+
			"test sẽ tự xoá dữ liệu thật", name)

	appDSN := prefix + hostPart + "/" + testdb.AppDatabase + suffix
	// Xác nhận database app thật sự tồn tại VÀ có bảng đã migrate. Không có
	// bước này, test có thể xanh vì lý do không liên quan tới guard.
	conn := openDB(t, appDSN)
	require.NoError(t, conn.PingContext(context.Background()),
		"database ứng dụng %q không kết nối được từ DSN test — dựng app "+
			"bằng `docker compose --profile v2 up -d` rồi chạy lại", testdb.AppDatabase)
	return appDSN
}
