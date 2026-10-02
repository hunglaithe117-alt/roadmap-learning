package roadmap

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parse_bookmark_status_defaults_and_lists_all_four(t *testing.T) {
	got, err := ParseBookmarkStatus("")
	require.NoError(t, err)
	assert.Equal(t, BookmarkStatusDefault, got)
	assert.Equal(t, BookmarkToRead, got, "DEFAULT của cột là to_read")

	for _, want := range AllBookmarkStatuses {
		got, err := ParseBookmarkStatus(string(want))
		require.NoError(t, err, "giá trị %q phải hợp lệ", want)
		assert.Equal(t, want, got)
	}
	require.Len(t, AllBookmarkStatuses, 4, "hợp đồng đóng băng 4 hằng")

	_, err = ParseBookmarkStatus("not_started")
	require.Error(t, err, "trạng thái node roadmap không hợp lệ với bookmark")
	assert.Contains(t, err.Error(), "to_read, reading, done, archived")
}

func Test_normalize_tags_trims_lowercases_dedupes_and_keeps_order(t *testing.T) {
	got, err := NormalizeTags([]string{" HSK3 ", "hsk3", "", "  ", "Ngữ Pháp", "hsk3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"hsk3", "ngữ pháp"}, got)

	empty, err := NormalizeTags(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.Equal(t, "", EncodeTags(empty), "CSV rỗng = \"\" chứ không phải \",\"")
}

func Test_normalize_tags_rejects_comma_and_overlong_tag(t *testing.T) {
	_, err := NormalizeTags([]string{"a,b"})
	require.Error(t, err, "dấu phẩy là ký tự phân cách CSV — nhận vào là 2 tag hỏng")
	assert.Contains(t, err.Error(), "dấu phẩy")

	_, err = NormalizeTags([]string{strings.Repeat("a", MaxBookmarkTagRunes+1)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag dài tối đa")

	many := make([]string, MaxBookmarkTags+1)
	for i := range many {
		many[i] = "t" + strconv.Itoa(i)
	}
	_, err = NormalizeTags(many)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tối đa 10 tag")
}

// Test_tags_contain_is_element_wise_not_substring là test VIỀN chốt cách khớp
// tag: tag `a` KHÔNG được khớp bookmark mang tag `ab`.
//
// Đây là lý do tầng hạ tầng dùng `position(',' || ? || ',' in ',' || tags || ',')`
// thay vì `LIKE '%a%'` — hàm này là bản thuần của điều kiện SQL đó, và test
// viền tương ứng ở `infrastructure/roadmap` chạy trên Postgres thật.
func Test_tags_contain_is_element_wise_not_substring(t *testing.T) {
	assert.False(t, TagsContain("ab", "a"), "tag `a` không được khớp tag `ab`")
	assert.False(t, TagsContain("abc", "a"))
	assert.False(t, TagsContain("ba", "a"), "không được khớp chuỗi con ở giữa")
	assert.True(t, TagsContain("a", "a"))
	assert.True(t, TagsContain("a,ab", "a"))
	assert.True(t, TagsContain("a,ab", "ab"))
	assert.True(t, TagsContain("ab,a", "a"), "thứ tự tag không quan trọng")
	assert.True(t, TagsContain("a,ab", " A "), "đầu vào lọc cũng phải chuẩn hoá")
	assert.True(t, TagsContain("HSK3", "hsk3"), "so khớp không phân biệt hoa thường")
	assert.False(t, TagsContain("a,ab", ""), "tag rỗng = không lọc, không phải khớp tất cả")
	assert.False(t, TagsContain("", "a"))
}

func Test_decode_tags_drops_empty_elements_from_peer_data(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, DecodeTags("a,b"))
	assert.Equal(t, []string{"a", "b"}, DecodeTags("a,,b,"), "phần tử rỗng do peer ghi bị bỏ, phần còn lại giữ nguyên")
	assert.Equal(t, []string{}, DecodeTags(""), "rỗng trả slice rỗng chứ không nil — GraphQL phân biệt null với []")
}

func Test_normalize_tag_filter_matches_normalize_tags(t *testing.T) {
	got, err := NormalizeTagFilter("  HSK3 ")
	require.NoError(t, err)
	assert.Equal(t, "hsk3", got, "lọc phải dùng đúng phép biến đổi lúc ghi, không lệch 1 bên")

	empty, err := NormalizeTagFilter("   ")
	require.NoError(t, err)
	assert.Equal(t, "", empty, "rỗng = không lọc")

	_, err = NormalizeTagFilter("a,b")
	require.Error(t, err)
}
