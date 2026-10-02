package graphql

import (
	"context"
	"time"

	contentapp "langapp/internal/application/content"
	practiceapp "langapp/internal/application/practice"
	"langapp/internal/transport/graphql/model"
)

// Default của tham số tuỳ chọn. Chúng lấy từ hằng của tầng application chứ không
// tự chọn con số: `content.DefaultSearchLimit` = 10 khớp DEFAULT của hàm SQL
// `dict_search`, `practice.DefaultErrorLimit` = 50 giữ LIMIT của v1. Hai bên
// lệch nhau là 1 request mà vẫn xanh, và lệch đó chỉ lộ ra khi đối chiếu với DB.
const (
	contentDefaultLimit       = contentapp.DefaultSearchLimit
	practiceDefaultErrorLimit = practiceapp.DefaultErrorLimit
	insightDefaultTopErrors   = 20
	syncDefaultConflictLimit  = 50
)

// contentSourceDict là hằng `domain/content.SourceDict` — tra trong từ điển.
const contentSourceDict = "dict"

// errBadRequest + statusError: xem `errors.go`.

// derefOr trả giá trị con trỏ, hoặc `def` khi nil/rỗng.
//
// Rỗng cũng được coi là "không gửi" vì client hay gửi `""` cho field text tuỳ
// chọn (HTML `<input>` không gửi gì thì thành chuỗi rỗng). Với `UpdatePath`,
// `overview: ""` là "xoá nội dung" — còn ở `CreatePath` thì `overview: ""` và
// không gửi là một thứ, vì `ValidateText` cho phép rỗng.
func derefOr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

// limitOr trả limit của client, hoặc default khi không gửi. `limit <= 0` cũng
// rơi về default: use case đã tự clamp về trần (`MaxErrorLimit` 200) nhưng
// `limit = -5` là client hỏng, và cho nó đọc 0 dòng sẽ khiến UI tưởng sổ lỗi
// trống.
func limitOr(p *int, def int) int {
	if p == nil || *p <= 0 {
		return def
	}
	return *p
}

// parseIDPtr đọc `ID` tuỳ chọn thành *int64. Rỗng / không phải số = nil, tức
// "không gắn" — đúng ngữ nghĩa của `cardId: null` trong `AppendErrorInput` và
// `deckId: null` trong `StageInput`.
func parseIDPtr(s *string) *int64 {
	if s == nil || *s == "" {
		return nil
	}
	v := parseID(*s)
	if v <= 0 {
		return nil
	}
	return &v
}

// nowUTC là mốc thời gian cho resolver nào cần "bây giờ" mà không có đồng hồ
// inject (chỉ `Query.streak`). `insight.ComputeStreak` nhận `now` để test
// được; ở đây client KHÔNG được chọn mốc — nếu lộ, 1 máy lệch giờ sẽ tự báo
// streak ảo.
func nowUTC() time.Time { return time.Now().UTC() }

// ptrOf đưa giá trị non-pointer vào field `*string` của view model. Chỉ dùng ở
// chỗ đã kiểm tra khác nil (xem `insightTopErrorViews`).
func ptrOf[T any](v T) *T { return &v }

// loaders là đường ngắn tới bộ loader của request, dùng bởi field resolver.
func (r *Resolver) loaders(ctx context.Context) *Loaders {
	return LoadersFrom(ctx, r.Roadmap, r.SRS)
}

// ── input adapters ──────────────────────────────────────────────────────────
//
// Gom các input của application vào 1 kiểu trung gian thay vì dựng thẳng trong
// resolver: giữ resolver chỉ gọi 1 dòng, và khi application đổi chữ ký (M5+) thì
// chỉ 1 chỗ sửa.

type contentImportInput struct{ Level, Deck string }

func (in contentImportInput) toApp() contentapp.ImportInput {
	return contentapp.ImportInput{Level: in.Level, Deck: in.Deck}
}

type contentThieuInput struct {
	Session string
	Scores  map[string]int
	Note    string
}

func (in contentThieuInput) toApp() contentapp.ThieuInput {
	return contentapp.ThieuInput{Session: in.Session, Scores: in.Scores, Note: in.Note}
}

func contentZHEntry(in model.DictEntryInput) contentapp.ZHEntry {
	return contentapp.ZHEntry{Hanzi: in.Hanzi, Pinyin: in.Pinyin, Nghia: in.Nghia}
}

// practiceErrorEntry là alias để mutation resolver không phải import thêm 1
// package chỉ để khai slice 1 phần tử.
type practiceErrorEntry = practiceapp.ErrorEntry
