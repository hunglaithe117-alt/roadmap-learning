package graphql_test

// Test `graph/client` cho bookmark + `TopError.cardId` — kiểm schema↔resolver
// wiring, không kiểm lại use case (đó là việc của test tầng application và
// infrastructure).

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"

	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
)

type bookmarkPayload struct {
	ID        string   `json:"id"`
	GUID      string   `json:"guid"`
	Title     string   `json:"title"`
	URL       *string  `json:"url"`
	Note      string   `json:"note"`
	Tags      string   `json:"tags"`
	TagList   []string `json:"tagList"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

const bookmarkFields = `id guid title url note tags tagList status createdAt updatedAt`

// Test_bookmark_crud_over_graphql chạy trọn vòng đời 1 bookmark: tạo → đổi
// trạng thái → sửa → xoá. Nếu `updateBookmark` nuốt mất `tags: []` (mảng rỗng
// bị hiểu là "không gửi") thì bước sửa sẽ ra kết quả khác — đó là lỗi mà chỉ
// đi qua GraphQL mới thấy vì model sinh là `[]string` chứ không phải `*[]string`.
func Test_bookmark_crud_over_graphql(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)

	var created struct {
		CreateBookmark struct {
			Ok       bool             `json:"ok"`
			Bookmark *bookmarkPayload `json:"bookmark"`
			Error    *userError       `json:"error"`
		} `json:"createBookmark"`
	}
	c.MustPost(
		`mutation($in: BookmarkInput!) { createBookmark(input: $in) { ok bookmark { `+bookmarkFields+` } error { message code } } }`,
		&created, client.Var("in", map[string]any{
			"title": "Bài tập ngữ pháp", "note": "đọc lại",
			"tags": []string{"HSK3", "ngữ pháp"}, "status": "TO_READ",
		}))
	require.True(t, created.CreateBookmark.Ok, "lỗi: %+v", created.CreateBookmark.Error)
	require.NotNil(t, created.CreateBookmark.Bookmark)
	require.NotEmpty(t, created.CreateBookmark.Bookmark.GUID, "mọi type phải có guid")
	require.Nil(t, created.CreateBookmark.Bookmark.URL, "không gửi url = null")
	require.Equal(t, "hsk3,ngữ pháp", created.CreateBookmark.Bookmark.Tags)
	require.Equal(t, []string{"hsk3", "ngữ pháp"}, created.CreateBookmark.Bookmark.TagList)
	require.Equal(t, "TO_READ", created.CreateBookmark.Bookmark.Status)
	id := created.CreateBookmark.Bookmark.ID

	var statusSet struct {
		SetBookmarkStatus struct {
			Ok       bool             `json:"ok"`
			Bookmark *bookmarkPayload `json:"bookmark"`
			Error    *userError       `json:"error"`
		} `json:"setBookmarkStatus"`
	}
	c.MustPost(
		`mutation($id: ID!, $s: BookmarkStatus!) { setBookmarkStatus(id: $id, status: $s) { ok bookmark { status } error { message } } }`,
		&statusSet, client.Var("id", id), client.Var("s", "READING"))
	require.True(t, statusSet.SetBookmarkStatus.Ok, "lỗi: %+v", statusSet.SetBookmarkStatus.Error)
	require.Equal(t, "READING", statusSet.SetBookmarkStatus.Bookmark.Status)

	var updated struct {
		UpdateBookmark struct {
			Ok       bool             `json:"ok"`
			Bookmark *bookmarkPayload `json:"bookmark"`
			Error    *userError       `json:"error"`
		} `json:"updateBookmark"`
	}
	c.MustPost(
		`mutation($id: ID!, $p: BookmarkPatch!) { updateBookmark(id: $id, patch: $p) { ok bookmark { `+bookmarkFields+` } error { message code } } }`,
		&updated, client.Var("id", id), client.Var("p", map[string]any{
			"title": "Bài tập (rút gọn)", "url": "https://example.com/a", "tags": []string{},
		}))
	require.True(t, updated.UpdateBookmark.Ok, "lỗi: %+v", updated.UpdateBookmark.Error)
	require.Equal(t, "Bài tập (rút gọn)", updated.UpdateBookmark.Bookmark.Title)
	require.NotNil(t, updated.UpdateBookmark.Bookmark.URL)
	require.Equal(t, "https://example.com/a", *updated.UpdateBookmark.Bookmark.URL)
	require.Empty(t, updated.UpdateBookmark.Bookmark.TagList, "tags: [] phải xoá hết tag, không phải giữ nguyên")
	require.Equal(t, "READING", updated.UpdateBookmark.Bookmark.Status,
		"patch không có status thì trạng thái phải giữ nguyên")

	var cleared struct {
		UpdateBookmark struct {
			Ok       bool             `json:"ok"`
			Bookmark *bookmarkPayload `json:"bookmark"`
			Error    *userError       `json:"error"`
		} `json:"updateBookmark"`
	}
	c.MustPost(
		`mutation($id: ID!, $p: BookmarkPatch!) { updateBookmark(id: $id, patch: $p) { ok bookmark { url } error { message } } }`,
		&cleared, client.Var("id", id), client.Var("p", map[string]any{"clearUrl": true}))
	require.True(t, cleared.UpdateBookmark.Ok, "lỗi: %+v", cleared.UpdateBookmark.Error)
	require.Nil(t, cleared.UpdateBookmark.Bookmark.URL, "clearUrl phải trả null")

	var deleted struct {
		DeleteBookmark struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"deleteBookmark"`
	}
	c.MustPost(`mutation($id: ID!) { deleteBookmark(id: $id) { ok error { message code } } }`,
		&deleted, client.Var("id", id))
	require.True(t, deleted.DeleteBookmark.Ok, "lỗi: %+v", deleted.DeleteBookmark.Error)

	// Xoá 2 lần phải ra 404 chứ không phải 500 — và `bookmark(id:)` trả null.
	var again struct {
		DeleteBookmark struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"deleteBookmark"`
	}
	c.MustPost(`mutation($id: ID!) { deleteBookmark(id: $id) { ok error { message code } } }`,
		&again, client.Var("id", id))
	require.False(t, again.DeleteBookmark.Ok)
	require.NotNil(t, again.DeleteBookmark.Error)
	require.Equal(t, "NOT_FOUND", again.DeleteBookmark.Error.Code)

	var listed struct {
		Bookmarks []bookmarkPayload `json:"bookmarks"`
	}
	c.MustPost(`query { bookmarks { `+bookmarkFields+` } }`, &listed)
	require.Empty(t, listed.Bookmarks, "bookmark đã xoá mềm không được còn trong list")
}

func Test_bookmarks_query_filters_by_status_and_tag(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	ctx := context.Background()

	// Tag "a" và "ab" đều hợp lệ nên tạo được qua use case — test viền cần đúng
	// cặp đó, không cần ghi tay SQL.
	_, err := h.container.Roadmap.CreateBookmark(ctx, roadmapapp.BookmarkInput{
		Title: "A", Tags: []string{"a"}, Status: "to_read",
	})
	require.NoError(t, err)
	_, err = h.container.Roadmap.CreateBookmark(ctx, roadmapapp.BookmarkInput{
		Title: "AB", Tags: []string{"ab"}, Status: "reading",
	})
	require.NoError(t, err)

	var byTag struct {
		Bookmarks []bookmarkPayload `json:"bookmarks"`
	}
	c.MustPost(`query($t: String) { bookmarks(tag: $t) { id title tags tagList } }`,
		&byTag, client.Var("t", "a"))
	require.Len(t, byTag.Bookmarks, 1, "tag `a` KHÔNG được khớp bookmark mang tag `ab`")
	require.Equal(t, "A", byTag.Bookmarks[0].Title)

	var byStatus struct {
		Bookmarks []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"bookmarks"`
	}
	c.MustPost(`query($s: BookmarkStatus) { bookmarks(status: $s) { id title } }`,
		&byStatus, client.Var("s", "READING"))
	require.Len(t, byStatus.Bookmarks, 1)
	require.Equal(t, "AB", byStatus.Bookmarks[0].Title)

	var byBoth struct {
		Bookmarks []bookmarkPayload `json:"bookmarks"`
	}
	c.MustPost(`query($s: BookmarkStatus, $t: String) { bookmarks(status: $s, tag: $t) { id } }`,
		&byBoth, client.Var("s", "READING"), client.Var("t", "a"))
	require.Empty(t, byBoth.Bookmarks, "2 điều kiện là AND, không có dòng nào khớp cả hai")

	var none struct {
		Bookmarks []bookmarkPayload `json:"bookmarks"`
	}
	c.MustPost(`query { bookmarks { id } }`, &none)
	require.Len(t, none.Bookmarks, 2, "không truyền filter = xem tất cả")
}

func Test_create_bookmark_reports_business_error_in_payload(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		CreateBookmark struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createBookmark"`
	}
	// Title rỗng → 400 của application; `ftp://` → 400. Cả hai phải về trong
	// payload chứ không thành lỗi GraphQL (mất `data` cho cả response).
	h.client(t).MustPost(
		`mutation { createBookmark(input: {title: "  ", url: "https://x.com"}) { ok error { message code } } }`,
		&resp)
	require.False(t, resp.CreateBookmark.Ok)
	require.NotNil(t, resp.CreateBookmark.Error)
	require.Equal(t, "BAD_REQUEST", resp.CreateBookmark.Error.Code)

	var bad struct {
		CreateBookmark struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createBookmark"`
	}
	h.client(t).MustPost(
		`mutation { createBookmark(input: {title: "T", url: "ftp://x.com"}) { ok error { message code } } }`,
		&bad)
	require.False(t, bad.CreateBookmark.Ok)
	require.Equal(t, "BAD_REQUEST", bad.CreateBookmark.Error.Code)
	require.Contains(t, bad.CreateBookmark.Error.Message, "http")
}

// Test_top_errors_expose_card_to_jump_into_review — nợ server M5 §8.4. Client
// trước đó chỉ có `word`, nên "bấm lỗi → nhảy review card" không làm được.
func Test_top_errors_expose_card_to_jump_into_review(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "你好", Back: "chào"})
	require.NoError(t, err)
	cardID := card.ID

	_, err = h.container.Practice.AppendError(ctx, &cardID, "你好", "ni hao", []string{"hau"})
	require.NoError(t, err)
	_, err = h.container.Practice.AppendError(ctx, nil, "自由", "zi you", []string{"you"})
	require.NoError(t, err)

	var resp struct {
		InsightTopErrors struct {
			Ok     bool `json:"ok"`
			Errors []struct {
				Word   string  `json:"word"`
				Count  int     `json:"count"`
				CardID *string `json:"cardId"`
				Front  *string `json:"front"`
			} `json:"errors"`
			Error *userError `json:"error"`
		} `json:"insightTopErrors"`
	}
	h.client(t).MustPost(
		`query { insightTopErrors(limit: 10) { ok errors { word count cardId front } error { message } } }`, &resp)
	require.True(t, resp.InsightTopErrors.Ok, "lỗi: %+v", resp.InsightTopErrors.Error)
	require.Len(t, resp.InsightTopErrors.Errors, 2)

	// Lỗi gắn thẻ: có cardId + front để bấm nhảy review.
	attached := resp.InsightTopErrors.Errors[0]
	require.Equal(t, "hau", attached.Word)
	require.NotNil(t, attached.CardID, "lỗi gắn thẻ phải trả cardId")
	require.NotNil(t, attached.Front)
	require.Equal(t, "你好", *attached.Front)
	// Lỗi luyện tự do: cardId null, KHÔNG phải trỏ nhầm thẻ khác.
	free := resp.InsightTopErrors.Errors[1]
	require.Equal(t, "you", free.Word)
	require.Nil(t, free.CardID, "lỗi không gắn thẻ ⇒ cardId null")
	require.Nil(t, free.Front)
}

func Test_top_errors_card_is_null_when_card_was_soft_deleted(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsapp.CardInput{Front: "谢谢", Back: "cảm ơn"})
	require.NoError(t, err)
	cardID := card.ID
	_, err = h.container.Practice.AppendError(ctx, &cardID, "谢谢", "xiexie", []string{"xie"})
	require.NoError(t, err)
	require.NoError(t, h.container.SRS.DeleteCard(ctx, card.ID))

	var resp struct {
		InsightTopErrors struct {
			Ok     bool `json:"ok"`
			Errors []struct {
				Word   string  `json:"word"`
				CardID *string `json:"cardId"`
				Front  *string `json:"front"`
			} `json:"errors"`
		} `json:"insightTopErrors"`
	}
	h.client(t).MustPost(
		`query { insightTopErrors(limit: 10) { ok errors { word cardId front } } }`, &resp)
	require.Len(t, resp.InsightTopErrors.Errors, 1)
	require.Nil(t, resp.InsightTopErrors.Errors[0].CardID,
		"thẻ đã xoá mềm thì không nhảy review được ⇒ cardId phải null")
	require.Nil(t, resp.InsightTopErrors.Errors[0].Front)
}
