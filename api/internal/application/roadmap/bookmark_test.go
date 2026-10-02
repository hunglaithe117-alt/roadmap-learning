package roadmap

// Test của use case bookmark. Phần lọc tag chạy trên Postgres thật nằm ở
// `infrastructure/roadmap` — ở đây chỉ chứng minh tầng application chuẩn hoá
// input và trả lỗi đúng, vì fakeRepo không dựng SQL.

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/roadmap"
)

// ── Validate ────────────────────────────────────────────────────────────────

func Test_validate_bookmark_status_rejects_node_status_values(t *testing.T) {
	// Tập hằng của bookmark CỐ Ý khác tập của node roadmap: nhận nhầm
	// `not_started` (hợp lệ với node) là 400 chứ không im lặng thành `to_read`.
	for _, in := range []string{"not_started", "in_progress", "skipped", "pending", "to-read"} {
		_, err := ValidateBookmarkStatus(in)
		require.Error(t, err, "status %q không thuộc tập 4 hằng của bookmark", in)
		var appErr *Error
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, StatusBadRequest, appErr.Status)
	}
	got, err := ValidateBookmarkStatus("")
	require.NoError(t, err)
	assert.Equal(t, "to_read", got, "rỗng = DEFAULT của cột")
	got, err = ValidateBookmarkStatus("  ARCHIVED  ")
	require.NoError(t, err)
	assert.Equal(t, "archived", got,
		"khác ValidateStatus của node: ở đây hạ chữ thường vì client gửi enum SCREAMING_CASE qua GraphQL")
	_, err = ValidateBookmarkStatus("TO_READ")
	require.NoError(t, err, "enum TO_READ của GraphQL phải qua được")
}

func Test_validate_bookmark_tags_normalises_and_rejects_too_many(t *testing.T) {
	got, err := ValidateBookmarkTags([]string{" HSK3 ", "hsk3", "", "   ", "Ngữ pháp"})
	require.NoError(t, err)
	assert.Equal(t, "hsk3,ngữ pháp", got, "trim + hạ chữ thường + bỏ trùng + bỏ rỗng")

	empty, err := ValidateBookmarkTags(nil)
	require.NoError(t, err)
	assert.Equal(t, "", empty, "rỗng = \"\" chứ không phải \",\"")

	_, err = ValidateBookmarkTags([]string{"a,b"})
	require.Error(t, err, "dấu phẩy là ký tự phân cách của cột CSV")
	assert.Contains(t, err.Error(), "dấu phẩy")

	// Trần số tag lấy từ domain, không hardcode 2 lần — nếu domain đổi hằng thì
	// test này phải đỏ chứ không im lặng kiểm 1 con số cũ. Tag phải KHÁC NHAU
	// vì `NormalizeTags` bỏ trùng: 11 chữ "t" giống nhau là 1 tag.
	long := make([]string, domain.MaxBookmarkTags+1)
	for i := range long {
		long[i] = "t" + strconv.Itoa(i)
	}
	_, err = ValidateBookmarkTags(long)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tối đa 10 tag")
}

// ── Use case ────────────────────────────────────────────────────────────────

func Test_create_bookmark_defaults_status_and_generates_guid(t *testing.T) {
	svc, _, _ := newTestService()
	got, err := svc.CreateBookmark(context.Background(), BookmarkInput{
		Title: "  Bài tập ngữ pháp  ", Note: " đọc lại ", Tags: []string{"HSK3"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bài tập ngữ pháp", got.Title)
	assert.Equal(t, "hsk3", got.Tags)
	assert.Equal(t, "to_read", got.Status, "status rỗng = to_read")
	assert.Nil(t, got.URL, "không gửi url = SQL NULL")
	assert.NotEmpty(t, got.GUID, "guid rỗng sẽ đụng UNIQUE ở bookmark thứ 2")
	assert.Equal(t, fixedClock.Format("2006-01-02T15:04:05Z"), got.CreatedAt)
}

func Test_create_bookmark_rejects_bad_url_and_status(t *testing.T) {
	svc, repo, _ := newTestService()
	bad := "ftp://x.com"
	_, err := svc.CreateBookmark(context.Background(), BookmarkInput{Title: "T", URL: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url chỉ nhận link http hoặc https")

	_, err = svc.CreateBookmark(context.Background(), BookmarkInput{Title: "T", Status: "not_started"})
	require.Error(t, err)

	_, err = svc.CreateBookmark(context.Background(), BookmarkInput{Title: "  "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tiêu đề không được rỗng")

	assert.Empty(t, repo.bookmarks, "lỗi validate không được ghi gì")
}

func Test_update_bookmark_returns_the_value_just_written(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	created, err := svc.CreateBookmark(ctx, BookmarkInput{
		Title: "Cũ", Note: "ghi chú cũ", Tags: []string{"a"},
	})
	require.NoError(t, err)

	bad := "javascript:alert(1)"
	_, err = svc.UpdateBookmark(ctx, created.ID, BookmarkPatch{URL: &bad})
	require.Error(t, err, "url sai scheme phải bị chặn trước khi ghi")

	got, err := svc.UpdateBookmark(ctx, created.ID, BookmarkPatch{
		Title: strPtr("Mới"), Note: strPtr("ghi chú mới"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Mới", got.Title)
	assert.Equal(t, "ghi chú mới", got.Note)
}

func Test_update_bookmark_clear_url_writes_sql_null(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	link := "https://example.com/a"
	created, err := svc.CreateBookmark(ctx, BookmarkInput{Title: "T", URL: &link})
	require.NoError(t, err)
	require.NotNil(t, created.URL)

	got, err := svc.UpdateBookmark(ctx, created.ID, BookmarkPatch{ClearURL: true})
	require.NoError(t, err)
	assert.Nil(t, got.URL, "clearUrl phải trả NULL, không phải chuỗi rỗng")
}

func Test_update_bookmark_empty_tags_list_clears_tags(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	created, err := svc.CreateBookmark(ctx, BookmarkInput{Title: "T", Tags: []string{"a", "b"}})
	require.NoError(t, err)
	require.Equal(t, "a,b", created.Tags)

	empty := []string{}
	got, err := svc.UpdateBookmark(ctx, created.ID, BookmarkPatch{Tags: &empty})
	require.NoError(t, err)
	assert.Equal(t, "", got.Tags, "gửi mảng rỗng là xoá hết tag")
}

func Test_update_bookmark_without_any_field_is_rejected(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	created, err := svc.CreateBookmark(ctx, BookmarkInput{Title: "T"})
	require.NoError(t, err)

	_, err = svc.UpdateBookmark(ctx, created.ID, BookmarkPatch{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "không có gì để cập nhật")
}

func Test_set_bookmark_status_persists_and_rejects_unknown_value(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	created, err := svc.CreateBookmark(ctx, BookmarkInput{Title: "T"})
	require.NoError(t, err)

	got, err := svc.SetBookmarkStatus(ctx, created.ID, "reading")
	require.NoError(t, err)
	assert.Equal(t, "reading", got.Status)
	assert.Equal(t, "reading", repo.bookmarks[created.ID].Status)

	_, err = svc.SetBookmarkStatus(ctx, created.ID, "in_progress")
	require.Error(t, err, "status của node roadmap không hợp lệ với bookmark")
}

func Test_delete_bookmark_is_soft_and_second_delete_is_404(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	created, err := svc.CreateBookmark(ctx, BookmarkInput{Title: "T"})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteBookmark(ctx, created.ID))
	assert.Equal(t, 1, repo.bookmarks[created.ID].Deleted, "xoá mềm: tombstone cho sync")

	_, err = svc.BookmarkByID(ctx, created.ID)
	require.Error(t, err, "đã xoá thì phải 404")
	assert.True(t, IsNotFound(err))

	err = svc.DeleteBookmark(ctx, created.ID)
	require.Error(t, err, "xoá 2 lần phải 404 chứ không phải 500")
	assert.True(t, IsNotFound(err))
}

func Test_list_bookmarks_rejects_unknown_filter_before_query(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	_, err := svc.ListBookmarks(ctx, BookmarkFilter{Status: "not_started"})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
}
