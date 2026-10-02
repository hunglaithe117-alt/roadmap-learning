package graphql_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"

	srsapp "langapp/internal/application/srs"
)

// dropReviews bỏ bảng `reviews` để ép 1 lỗi HỆ THỐNG THẬT từ Postgres.
//
// Vì sao dùng cách này thay vì fake: đường leak đã xảy ra với lỗi GORM/Postgres
// thật (`application/insight` bọc `fmt.Errorf("đọc lịch sử ôn: %w", err)`), và
// chỉ lỗi thật mới mang đủ thông tin nhạy cảm cần kiểm — SQLSTATE, tên bảng, tên
// cột. Lỗi giả lập trong test sẽ tự cho ta chọn message sạch rồi tự khẳng định
// là sạch, tức test rỗng.
func dropReviews(t *testing.T, h *harness) {
	t.Helper()
	require.NoError(t, h.container.DB.Exec("DROP TABLE IF EXISTS reviews").Error)
}

// Test system error không lộ SQLSTATE/tên bảng cho client.
//
// Đây là lỗi BẢO MẬT: app v1 không có auth, nên một response chứa
// `ERROR: relation "reviews" does not exist (SQLSTATE 42P01)` + câu SQL là đủ để
// kẻ khách lập bản đồ DB. `errorPresenter` không cứu được vì resolver trả payload
// với `error = nil` cho GraphQL — che ở presenter là che quá muộn.
func Test_system_error_from_resolver_does_not_leak_sql_to_client(t *testing.T) {
	h := newHarness(t)
	dropReviews(t, h)

	var resp struct {
		Stats struct {
			Ok    bool                    `json:"ok"`
			Stats *struct{ Range string } `json:"stats"`
			Error *userError              `json:"error"`
		} `json:"stats"`
	}
	h.client(t).MustPost(`query { stats(range: "week") { ok stats { range } error { message code } } }`, &resp)

	require.False(t, resp.Stats.Ok, "query chắc chắn phải fail khi bảng reviews đã bị drop")
	require.NotNil(t, resp.Stats.Error)
	require.Nil(t, resp.Stats.Stats, "lỗi thì không được trả payload dở dang")

	msg := resp.Stats.Error.Message
	leaks := []string{
		"SQLSTATE", "42P01", "relation", "reviews", "SELECT", "pg_", "ERROR:",
	}
	for _, l := range leaks {
		require.NotContains(t, msg, l,
			"message lộ chi tiết driver (%q): %q", l, msg)
	}
	require.Equal(t, "lỗi hệ thống", msg)
	require.Equal(t, "INTERNAL", resp.Stats.Error.Code)
}

// Che ở `toUserError` không được phá vỡ lỗi NGHIỆP VỤ: message tiếng Việt của
// application vẫn phải ra nguyên văn, vì đó là hợp đồng M4 chốt.
func Test_business_error_message_is_not_masked_by_the_system_error_guard(t *testing.T) {
	h := newHarness(t)
	dropReviews(t, h) // bảng hỏng, nhưng lỗi 404 vẫn phải ra tiếng Việt

	// Trùng slug sau khi đã seed 1 path ⇒ lỗi 409 NGHIỆP VỤ, trong khi DB đang
	// hỏng (`reviews` đã drop) — chứng minh che lỗi hệ thống KHÔNG nuốt mất lỗi
	// nghiệp vụ xảy ra ở cùng lúc.
	_, err := h.container.Roadmap.CreatePath(context.Background(), roadmapInput("trung"))
	require.NoError(t, err)

	var created struct {
		CreatePath struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createPath"`
	}
	h.client(t).MustPost(`mutation { createPath(input: {slug: "trung", title: "B"}) { ok error { message code } } }`, &created)

	require.False(t, created.CreatePath.Ok)
	require.Equal(t, "CONFLICT", created.CreatePath.Error.Code)
	require.Contains(t, created.CreatePath.Error.Message, "đã tồn tại",
		"lỗi nghiệp vụ phải còn message tiếng Việt của application")
}

// Lỗi của chính TRANSPORT (`since` sai định dạng) cũng phải ra nguyên văn — nó
// do tầng này soạn, không phải chi tiết driver.
func Test_transport_error_message_is_not_masked_either(t *testing.T) {
	h := newHarness(t)
	seedPathWithTopics(t, h, "zh", 1)

	var resp struct {
		PathProgress struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"pathProgress"`
	}
	h.client(t).MustPost(
		`query($s: String) { pathProgress(slug: "zh", since: $s) { ok error { message code } } }`,
		&resp, client.Var("s", "khong-phai-ngay"))

	require.False(t, resp.PathProgress.Ok)
	require.Equal(t, "BAD_REQUEST", resp.PathProgress.Error.Code)
	require.Contains(t, resp.PathProgress.Error.Message, "YYYY-MM-DD",
		"lỗi tham số của transport phải nói rõ format mong muốn")
}

// Lỗi hệ thống ở MUTATION cũng phải bị che — cùng đường `toUserError`.
//
// Phải tạo thẻ THẬT trước: `RecordReview` kiểm thẻ tồn tại trước rồi mới chạm
// `reviews`, nên với `cardId` không có thì nó dừng ở 404 và không chạm tới bảng đã
// drop — test sẽ "xanh" mà không chứng minh gì về đường leak của mutation.
func Test_system_error_in_mutation_does_not_leak_sql(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	deck, err := h.container.SRS.CreateDeck(ctx, "HSK1", "zh")
	require.NoError(t, err)
	card, err := h.container.SRS.CreateCard(ctx, deck.ID, srsCardInput())
	require.NoError(t, err)
	dropReviews(t, h)

	var resp struct {
		RecordReview struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"recordReview"`
	}
	h.client(t).MustPost(
		`mutation($id: ID!, $g: Int!) { recordReview(input: {cardId: $id, grade: $g}) { ok error { message code } } }`,
		&resp, client.Var("id", strconv.FormatInt(card.ID, 10)), client.Var("g", 4))

	require.False(t, resp.RecordReview.Ok)
	require.NotNil(t, resp.RecordReview.Error)
	require.Equal(t, "INTERNAL", resp.RecordReview.Error.Code)
	require.Equal(t, "lỗi hệ thống", resp.RecordReview.Error.Message)
}

// Không được che quá tay: lỗi hệ thống vẫn phải KHÁC lỗi nghiệp vụ ở `code`,
// nếu không client không phân biệt được "bạn gửi sai" với "server hỏng".
//
// MỘT harness duy nhất, và THỨ TỰ quan trọng: nghiệp vụ trước, hệ thống sau.
// `newHarness` lần 2 trong cùng 1 test sẽ TREO — `testdb.Acquire` nắm
// `pg_advisory_lock` mà lock này không xếp hồng giữa 2 session, nên session thứ 2
// chờ vô hạn (đã ghi ở `testdb.Acquire`). Bản đầu của test này viết đúng như vậy và
// treo tới khi bị `timeout` giết ⇒ cleanup không chạy ⇒ rò schema.
func Test_system_and_business_errors_have_different_codes(t *testing.T) {
	h := newHarness(t)

	_, err := h.container.Roadmap.CreatePath(context.Background(), roadmapInput("x"))
	require.NoError(t, err)
	var biz struct {
		CreatePath struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"createPath"`
	}
	h.client(t).MustPost(`mutation { createPath(input: {slug: "x", title: "Y"}) { ok error { message code } } }`, &biz)
	require.False(t, biz.CreatePath.Ok)
	require.NotNil(t, biz.CreatePath.Error)
	require.Equal(t, "CONFLICT", biz.CreatePath.Error.Code)

	dropReviews(t, h)
	var sys struct {
		Stats struct {
			Ok    bool       `json:"ok"`
			Error *userError `json:"error"`
		} `json:"stats"`
	}
	h.client(t).MustPost(`query { stats(range: "week") { ok error { message code } } }`, &sys)
	require.False(t, sys.Stats.Ok)
	require.NotNil(t, sys.Stats.Error)
	require.Equal(t, "INTERNAL", sys.Stats.Error.Code)

	require.NotEqual(t, sys.Stats.Error.Code, biz.CreatePath.Error.Code,
		"client phải phân biệt được 'bạn gửi sai' với 'server hỏng'")
}

func srsCardInput() srsapp.CardInput {
	return srsapp.CardInput{Front: "你", Back: "bạn", Pinyin: "ni3"}
}

// GORM/Postgres lỗi có thể xuất hiện ở BẤT KỲ field nào; test này quét nhiều
// resolver để chắc không có chỗ nào sót. Chỉ 1 trong số đó fail là đủ để biết
// đường leak còn mở.
//
// Dùng `RawPost` (không `MustPost`) vì cần ĐỌC RAW toàn bộ response — `MustPost`
// giải mã vào struct nên chỗ khác trong payload (ví dụ `extensions`) không bị
// quét, mà đó mới là chỗ dễ sót nhất.
func Test_no_resolver_leaks_driver_detail_after_table_drop(t *testing.T) {
	h := newHarness(t)
	seedPathWithTopics(t, h, "zh", 2)
	dropReviews(t, h)

	queries := map[string]string{
		"stats":                `query { stats(range: "week") { ok error { message } } }`,
		"progressOverRange":    `query { progressOverRange(since: "2000-01-01") { ok error { message } } }`,
		"insightTopErrors":     `query { insightTopErrors { ok error { message } } }`,
		"shadowProgress":       `query { shadowProgress(cardId: "1") { loops } }`,
		"errors":               `query { errors { id } }`,
		"createDeck":           `mutation { createDeck(name: "X") { ok error { message } } }`,
		"dictSearch":           `query { dictSearch(q: "a") { hanzi } }`,
		"thieuSessions":        `query { thieuSessions { id } }`,
		"readerArticles":       `query { readerArticles { id } }`,
		"syncStatus":           `query { syncStatus { ok error { message } } }`,
		"recordReviewMutation": `mutation { recordReview(input: {cardId: "1", grade: 4}) { ok error { message } } }`,
	}
	leaks := []string{"sqlstate", "relation \"", "42p01", "pg_catalog", "error: relation"}
	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			raw, err := h.client(t).RawPost(q)
			require.NoError(t, err)
			body := strings.ToLower(string(mustJSON(raw.Data)))
			if raw.Errors != nil {
				body += strings.ToLower(string(mustJSON(raw.Errors)))
			}
			for _, leak := range leaks {
				require.NotContains(t, body, leak,
					"resolver %s lộ chi tiết driver trong response: %s", name, body)
			}
		})
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte{}
	}
	return b
}
