package roadmap

import (
	"fmt"
	"strings"
)

// BookmarkStatus là trạng thái 1 dòng trong kho link độc lập
// (`roadmap_bookmarks`, amendment A1 — STACK-V2-PLAN §5).
//
// CỐ Ý KHÁC `Status` của node roadmap: bookmark không nằm trên đường đi nên
// không có "chưa mở" và không có "bỏ qua" — đọc xong thì `done`, không còn giá
// trị thì `archived`. CHECK ở DB (migration `00003_roadmap_a1.sql`) là
// backstop và là hợp đồng đóng băng: 2 nơi phải sửa cùng lúc.
type BookmarkStatus string

const (
	BookmarkToRead   BookmarkStatus = "to_read"
	BookmarkReading  BookmarkStatus = "reading"
	BookmarkDone     BookmarkStatus = "done"
	BookmarkArchived BookmarkStatus = "archived"
)

// BookmarkStatusDefault là DEFAULT của cột `roadmap_bookmarks.status`.
const BookmarkStatusDefault = BookmarkToRead

// AllBookmarkStatuses là tập hợp hợp lệ, dùng cho validate và vòng lặp test.
var AllBookmarkStatuses = []BookmarkStatus{
	BookmarkToRead, BookmarkReading, BookmarkDone, BookmarkArchived,
}

// Valid báo status có thuộc tập 4 hằng không.
func (s BookmarkStatus) Valid() bool {
	for _, v := range AllBookmarkStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ParseBookmarkStatus chuyển input người dùng thành BookmarkStatus. Rỗng →
// DEFAULT, sai thì lỗi liệt kê đủ 4 giá trị để client hiện được lựa chọn hợp lệ.
func ParseBookmarkStatus(raw string) (BookmarkStatus, error) {
	s := BookmarkStatus(strings.ToLower(strings.TrimSpace(raw)))
	if s == "" {
		return BookmarkStatusDefault, nil
	}
	if !s.Valid() {
		names := make([]string, 0, len(AllBookmarkStatuses))
		for _, v := range AllBookmarkStatuses {
			names = append(names, string(v))
		}
		return "", fmt.Errorf("status bookmark chỉ nhận: %s", strings.Join(names, ", "))
	}
	return s, nil
}

// Giới hạn tag. Số phần tử chặt vì cột `tags` là CSV trong 1 TEXT và mỗi
// bookmark là 1 dòng của kho link đọc được — 10 tag là quá đủ cho mục đích lọc.
const (
	MaxBookmarkTags     = 10
	MaxBookmarkTagRunes = 40
)

// NormalizeTags chuẩn hoá danh sách tag trước khi ghi: trim, hạ chữ thường, bỏ
// phần tử rỗng, bỏ trùng (so trên dạng đã hạ chữ thường), giữ nguyên thứ tự
// user gõ.
//
// Hạ chữ thường là để lọc ổn định: `NormalizeTagFilter` áp đúng biến đổi này
// cho đầu vào lọc, nên ghi "HSK3" mà lọc "hsk3" vẫn ra. Ô nhập tag trong UI là
// 1 ô trống, không phải danh sách — chuẩn hoá ở đây giữ được 1 nguồn sự thật.
//
// Lỗi trả về là lỗi thuần của domain; tầng application bọc thành 400 kèm
// message tiếng Việt (xem `ValidateBookmarkTags`).
func NormalizeTags(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if strings.Contains(t, ",") {
			return nil, fmt.Errorf("tag không được chứa dấu phẩy")
		}
		if len([]rune(t)) > MaxBookmarkTagRunes {
			return nil, fmt.Errorf("tag dài tối đa %d ký tự", MaxBookmarkTagRunes)
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxBookmarkTags {
		return nil, fmt.Errorf("tối đa %d tag mỗi bookmark", MaxBookmarkTags)
	}
	return out, nil
}

// EncodeTags ghi tag thành CSV. Rỗng → "" (không phải ",") để khớp DEFAULT của
// cột `tags` NOT NULL.
//
// CSV KHÔNG có khoảng trắng quanh dấu phẩy: `TagsContain` và truy vấn lọc ở
// tầng hạ tầng (`infrastructure/roadmap`) dựa vào đúng hình dạng này.
func EncodeTags(tags []string) string { return strings.Join(tags, ",") }

// DecodeTags tách CSV thành mảng, bỏ phần tử rỗng (dữ liệu peer có thể do
// phiên bản khác ghi). Rỗng trả slice rỗng chứ không nil để GraphQL trả `[]`.
func DecodeTags(csv string) []string {
	out := []string{}
	for _, part := range strings.Split(csv, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// NormalizeTagFilter chuẩn hoá tham số lọc `tag` của `ListBookmarks` — cùng
// phép biến đổi với `NormalizeTags` để ghi/đọc/lọc luôn khớp nhau. Rỗng = không
// lọc.
func NormalizeTagFilter(raw string) (string, error) {
	t, err := NormalizeTags([]string{raw})
	if err != nil {
		return "", err
	}
	if len(t) == 0 {
		return "", nil
	}
	return t[0], nil
}

// TagsContain báo CSV `tags` có chứa đúng phần tử `tag` hay không.
//
// Dấu phẩy biên ở CẢ HAI vế là điều kiện bắt buộc, không phải chi tiết trang
// trí: `LIKE '%a%'` khớp tag `ab`, còn `,a,` chỉ khớp khi `a` là phần tử
// riêng. Hàm này là bản thuần của điều kiện SQL
// `position(',' || ? || ',' in ',' || tags || ',') > 0` — 2 nơi phải cho cùng
// kết quả, và test viền `a` vs `ab` chạy trên CẢ HAI.
func TagsContain(csv, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	for _, part := range strings.Split(csv, ",") {
		if strings.ToLower(strings.TrimSpace(part)) == tag {
			return true
		}
	}
	return false
}
