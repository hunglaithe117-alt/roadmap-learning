package roadmapinfra_test

// Test bookmark CRUD trên Postgres thật. Test tầng application
// (`internal/application/roadmap/bookmark_test.go`) chỉ kiểm validate + điều
// phối vì fakeRepo không dựng SQL — truy vấn lọc tag, trigger `updated_at` và
// tombstone chỉ kiểm được ở đây.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	roadmapapp "langapp/internal/application/roadmap"
)

func Test_bookmark_crud_roundtrip_on_postgres(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	link := "https://example.com/hsk3"
	created, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{
		Title: "Bài tập ngữ pháp HSK3", URL: &link, Note: "đọc lại", Tags: []string{"HSK3", "ngữ pháp"},
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.NotEmpty(t, created.GUID, "guid rỗng sẽ đụng ux_roadmap_bookmarks_guid ở dòng thứ 2")
	assert.Equal(t, "hsk3,ngữ pháp", created.Tags, "CSV không khoảng trắng quanh dấu phẩy")
	assert.Equal(t, "to_read", created.Status)

	// Cột `url` NULLABLE phải ghi SQL NULL thật, không phải chuỗi rỗng.
	var nullURLs int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_bookmarks WHERE id = ? AND url IS NULL", created.ID).Scan(&nullURLs).Error)
	assert.Equal(t, 0, nullURLs, "có url thì không được NULL")

	updated, err := svc.UpdateBookmark(ctx, created.ID, roadmapapp.BookmarkPatch{
		Title: strp("Bài tập HSK3 (rút gọn)"), Note: strp("đã đọc 1/2"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Bài tập HSK3 (rút gọn)", updated.Title)
	assert.Equal(t, "đã đọc 1/2", updated.Note)
	assert.Equal(t, link, *updated.URL, "patch không đụng url thì url giữ nguyên")

	after, err := svc.SetBookmarkStatus(ctx, created.ID, "reading")
	require.NoError(t, err)
	assert.Equal(t, "reading", after.Status)

	// Trigger `langapp_touch_updated_at` phải chạm khi UPDATE không set
	// updated_at — nếu không, merge LWW phía peer chọn nhầm bản cũ.
	var updatedAt string
	require.NoError(t, db.Raw("SELECT updated_at FROM roadmap_bookmarks WHERE id = ?", created.ID).Scan(&updatedAt).Error)
	assert.NotEqual(t, fixedNow.Format("2006-01-02T15:04:05Z"), updatedAt,
		"UPDATE phải để trigger chạm updated_at, không ghi mốc của mốc test")

	require.NoError(t, svc.DeleteBookmark(ctx, created.ID))

	var live int
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM roadmap_bookmarks WHERE id = ? AND deleted = 0", created.ID).Scan(&live).Error)
	assert.Equal(t, 0, live, "xoá mềm: tombstone phải còn dòng cho sync")

	_, err = svc.BookmarkByID(ctx, created.ID)
	require.Error(t, err)
	assert.True(t, roadmapapp.IsNotFound(err))
}

func Test_create_bookmark_rolls_back_on_unique_violation(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	first, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: "Một"})
	require.NoError(t, err)

	// Guid trùng = vi phạm UNIQUE ở `ux_roadmap_bookmarks_guid`. Ghi thẳng bằng
	// SQL để mô phỏng guid trùng: `NewGUID` sinh ngẫu nhiên nên không tự tái
	// hiện, mà test phải chứng minh đường xử lý 409 chứ không phải 500.
	dup := first.GUID
	err = db.Exec(
		"INSERT INTO roadmap_bookmarks (title, created_at, guid) VALUES ('Trùng', '2026-01-01', ?)", dup).Error
	require.Error(t, err, "guid trùng phải bị UNIQUE chặn ở DB")

	// 2 bookmark khác guid vẫn tạo được ⇒ lỗi trên là UNIQUE, không phải bảng
	// hỏng.
	second, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: "Hai"})
	require.NoError(t, err)
	assert.NotEqual(t, first.GUID, second.GUID)
}

func Test_list_bookmarks_filters_by_status(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	ids := seedBookmarks(t, svc, "A", "B", "C")
	_, err := svc.SetBookmarkStatus(ctx, ids[1], "reading")
	require.NoError(t, err)
	_, err = svc.SetBookmarkStatus(ctx, ids[2], "reading")
	require.NoError(t, err)

	all, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 3, "filter rỗng = xem tất cả")
	assert.Equal(t, ids[0], all[0].ID, "thứ tự là thứ tự thêm (id tăng dần)")

	reading, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Status: "reading"})
	require.NoError(t, err)
	require.Len(t, reading, 2)
	for _, b := range reading {
		assert.Equal(t, "reading", b.Status)
	}

	_, err = svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Status: "not_started"})
	require.Error(t, err, "status của node roadmap không lọc được bookmark")
}

func Test_list_bookmarks_filters_by_status_with_invalid_value(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)

	_, err := svc.ListBookmarks(context.Background(), roadmapapp.BookmarkFilter{Status: "pending"})
	require.Error(t, err)
	var appErr *roadmapapp.Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, roadmapapp.StatusBadRequest, appErr.Status)
}

// Test_list_bookmarks_tag_filter_matches_whole_element là test VIỀN bắt buộc:
// lọc tag `a` KHÔNG được ra bookmark mang tag `ab`.
//
// `tags` lưu CSV nên cách khớp sai là `LIKE '%a%'` — nó ra kết quả "đúng một
// nửa" (ra cả `ab`) và UI không có cách nào phát hiện. Bản thuần của quy tắc là
// `domain.TagsContain`, test tương ứng ở `domain/roadmap/bookmark_test.go`.
func Test_list_bookmarks_tag_filter_matches_whole_element(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	// Ghi CSV bằng SQL thẳng: tầng application chỉ cho tạo qua `CreateBookmark`,
	// mà hàm đó tự validate nên không tạo được bookmark mang tag "ab" khi lọc
	// "a" một cách có chủ đích — cần dữ liệu thô để kiểm đường SQL.
	rawInsert := func(title, tags string) int64 {
		var id int64
		require.NoError(t, db.Raw(
			"INSERT INTO roadmap_bookmarks (title, tags, status, created_at, guid) "+
				"VALUES (?, ?, 'to_read', '2026-01-01', ?) RETURNING id",
			title, tags, "gb-"+title).Scan(&id).Error)
		return id
	}
	exact := rawInsert("Chính xác", "a")
	other := rawInsert("Tiền tố", "ab")
	middle := rawInsert("Giữa", "ba")
	multi := rawInsert("Nhiều tag", "a,ab,abc")

	got, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "a"})
	require.NoError(t, err)
	gotIDs := bookmarkIDs(got)
	assert.Equal(t, []int64{exact, multi}, gotIDs,
		"tag `a` chỉ khớp phần tử `a`; `ab`/`abc`/`ba` không được lọc ra")

	got, err = svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "ab"})
	require.NoError(t, err)
	assert.Equal(t, []int64{other, multi}, bookmarkIDs(got))

	// `ba` chứa `a` và `b` ở vị trí đầu/cuối — cả 2 đều không phải phần tử.
	got, err = svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "ba"})
	require.NoError(t, err)
	assert.Equal(t, []int64{middle}, bookmarkIDs(got))

	// Lọc kèm status = 2 điều kiện AND.
	_, err = svc.SetBookmarkStatus(ctx, exact, "reading")
	require.NoError(t, err)
	got, err = svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "a", Status: "reading"})
	require.NoError(t, err)
	assert.Equal(t, []int64{exact}, bookmarkIDs(got))
}

func Test_list_bookmarks_tag_filter_escapes_like_wildcards(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	// `%` và `_` là ký tự đặc biệt của `LIKE`. `position()` không có khái niệm
	// wildcard nên chúng thành ký tự thường — cùng cách `dict_search` phải khử
	// `%`/`_` vì lý do đó (F5 của migration 00002).
	var id int64
	require.NoError(t, db.Raw(
		"INSERT INTO roadmap_bookmarks (title, tags, status, created_at, guid) "+
			"VALUES ('Ký tự đặc biệt', 'a%b', 'to_read', '2026-01-01', 'gb-wild') RETURNING id").Scan(&id).Error)

	all, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{})
	require.NoError(t, err)
	require.Len(t, all, 1, "chỉ 1 dòng — nếu `%` thành wildcard thì lọc gì cũng ra hết")

	got, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "a%b"})
	require.NoError(t, err)
	assert.Equal(t, []int64{id}, bookmarkIDs(got), "% phải được so như ký tự thường")

	got, err = svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "%"})
	require.NoError(t, err)
	assert.Empty(t, got, "lọc `%` không được ra toàn bảng")
}

func Test_list_bookmarks_skips_soft_deleted(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	keep, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: "Còn", Tags: []string{"a"}})
	require.NoError(t, err)
	drop, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: "Xoá", Tags: []string{"a"}})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteBookmark(ctx, drop.ID))

	got, err := svc.ListBookmarks(ctx, roadmapapp.BookmarkFilter{Tag: "a"})
	require.NoError(t, err)
	assert.Equal(t, []int64{keep.ID}, bookmarkIDs(got))
}

func Test_update_bookmark_writes_null_url_not_empty_string(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)
	ctx := context.Background()

	link := "https://example.com"
	created, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: "T", URL: &link})
	require.NoError(t, err)

	_, err = svc.UpdateBookmark(ctx, created.ID, roadmapapp.BookmarkPatch{ClearURL: true})
	require.NoError(t, err)

	var isNull bool
	require.NoError(t, db.Raw("SELECT url IS NULL FROM roadmap_bookmarks WHERE id = ?", created.ID).Scan(&isNull).Error)
	assert.True(t, isNull, "clearUrl phải ghi SQL NULL, không phải ''")
}

func Test_update_bookmark_unknown_id_is_not_found(t *testing.T) {
	db := newTestDB(t)
	svc := newService(t, db, fixedNow)

	_, err := svc.UpdateBookmark(context.Background(), 999999,
		roadmapapp.BookmarkPatch{Title: strp("T")})
	require.Error(t, err)
	assert.True(t, roadmapapp.IsNotFound(err))

	err = svc.DeleteBookmark(context.Background(), 999999)
	require.Error(t, err)
	assert.True(t, roadmapapp.IsNotFound(err))
}

func Test_bookmark_status_check_constraint_rejects_node_statuses(t *testing.T) {
	db := newTestDB(t)

	// CHECK ở DB là backstop chung cho cả whitelist ở application và tập hằng ở
	// domain. Phải có dòng trước — `UPDATE` không khớp dòng nào thì Postgres
	// không kiểm CHECK, và test xanh với 0 dòng.
	require.NoError(t, db.Exec(
		"INSERT INTO roadmap_bookmarks (title, status, created_at, guid) "+
			"VALUES ('T', 'to_read', '2026-01-01', 'gb-check')").Error)

	err := db.Exec("UPDATE roadmap_bookmarks SET status = 'in_progress'").Error
	require.Error(t, err, "CHECK của cột phải từ chối status của node roadmap")

	require.NoError(t, db.Exec("UPDATE roadmap_bookmarks SET status = 'archived'").Error,
		"4 hằng hợp lệ phải ghi được")
}

// ── helpers ─────────────────────────────────────────────────────────────────

// seedBookmarks tạo 1 bookmark cho mỗi title và trả id theo thứ tự.
func seedBookmarks(t *testing.T, svc *roadmapapp.Service, titles ...string) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, len(titles))
	for _, title := range titles {
		b, err := svc.CreateBookmark(ctx, roadmapapp.BookmarkInput{Title: title})
		require.NoError(t, err)
		ids = append(ids, b.ID)
	}
	return ids
}

func bookmarkIDs(rows []roadmapapp.Bookmark) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}
