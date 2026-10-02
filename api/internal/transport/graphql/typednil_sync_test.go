package graphql_test

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// Lớp lỗi `typed-nil interface` — lần thứ 3 trong dự án:
//
//  1. M2 F2: `txOf` rơi về pool im lặng khi handle sai kiểu.
//  2. M7a B2: `Wire` truyền `syncinfra.NewSchemaLoader(nil)` cho `sync.NewService`.
//     `*SchemaLoader` là CON TRỎ → nhét vào tham số `SnapshotLoader` (interface)
//     cho interface KHÔNG nil ⇒ `s.loader == nil` trong `Service.Sync` SAI ⇒
//     code đi vào `SchemaLoader.Load` với `peer` nil ⇒ gọi method trên `*gorm.DB`
//     nil ⇒ **PANIC**.
//  3. M7a B1: `Options.Backup` nhận `(*pgBackup)(nil)` ⇒ `port == nil` SAI ⇒
//     panic thay vì 501.
//
// B2 là lần nguy hiểm nhất: `mutation.sync` là endpoint user bấm được, và
// `gin.Recovery()`/gqlgen KHÔNG đổi panic thành lỗi nghiệp vụ cho client —
// client nhận 500 với stack trace.

// mustNotPanic biến panic thành FAIL rõ ràng.
//
// Vì sao cần: panic trong goroutine test KHÔNG dừng test hiện tại, nó làm sập
// TOÀN BỘ binary test ⇒ 26 package cùng FAIL vì 1 chỗ, và dấu vết thật (panic
// value + stack) bị chôn dưới hàng loạt lỗi của package khác. `t.Errorf` (không
// phải `t.Fatal`) để test hiện tại vẫn chạy hết và các test sau vẫn báo lỗi.
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: PANIC (lớp lỗi typed-nil interface): %v\n%s", what, r, debug.Stack())
		}
	}()
	fn()
}

// mergeResp là shape `mutation.sync` tối thiểu: chỉ `ok` + `error`.
type mergeResp struct {
	Sync struct {
		Ok    bool       `json:"ok"`
		Error *userError `json:"error"`
	} `json:"sync"`
}

// Test_sync_mutation_returns_business_error_instead_of_panicking là test B2.
//
// Kịch bản: `platform.Wire` (composition root thật) dựng `sync.Service` với
// `syncinfra.NewSchemaLoader(nil)`. Trước fix, `Service.Sync` thấy
// `s.loader == nil` là FALSE (con trỏ nil trong interface ≠ nil interface) nên
// gọi `Load` → `l.peer.WithContext(...)` với `*gorm.DB` nil → panic.
//
// Sau fix: `Wire` truyền interface nil THẬT ⇒ `Service.Sync` trả lỗi nghiệp vụ
// tiếng Việt, và `mutation.sync` trả `ok=false` + `error` — không panic.
func Test_sync_mutation_returns_business_error_instead_of_panicking(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)

	var resp mergeResp
	mustNotPanic(t, "mutation.sync (payload hợp lệ)", func() {
		c.MustPost(`mutation { sync { ok error { message code } } }`, &resp)
	})

	require.False(t, resp.Sync.Ok, "sync chưa có nguồn snapshot peer thì phải báo thất bại, không phải báo thành công")
	require.NotNil(t, resp.Sync.Error, "phải trả lỗi nghiệp vụ tiếng Việt, không được im lặng")
	require.Contains(t, resp.Sync.Error.Message, "chưa cấu hình",
		"message phải nói rõ nguyên nhân bằng tiếng Việt cho user đọc được")
	require.NotEqual(t, "INTERNAL", string(resp.Sync.Error.Code),
		"lỗi này là NGHIỆP VỤ (chưa cấu hình), không phải lỗi hệ thống — nếu là INTERNAL "+
			"thì `toUserError` đã che message tiếng Việt thành \"lỗi hệ thống\"")

	// Mã ĐÚNG NGHĨA, không phải mã "thoát assert ở trên".
	//
	// Gate M6 nêu nghi ngờ 409 ở đây được chọn chỉ để tránh INTERNAL chứ không
	// vì đúng ngữ nghĩa. M7b đổi sang 501 (Not Implemented) — cùng nghĩa với
	// `/api/backup` khi chưa bật. 409 nghĩa là "dữ liệu tôi xung đột với peer",
	// mà ở đây KHÔNG có peer nào để xung đột; 503 nghĩa là "tạm thời, thử lại
	// sau", mà thiếu cấu hình thì thử lại vô ích.
	require.Equal(t, "NOT_IMPLEMENTED", string(resp.Sync.Error.Code),
		"'chưa cấu hình nguồn snapshot peer' là tính năng chưa bật ⇒ NOT_IMPLEMENTED, không phải CONFLICT/INTERNAL")
	require.NotEqual(t, "CONFLICT", string(resp.Sync.Error.Code),
		"409 khiến client hiểu sai là dữ liệu xung đột với peer — app không có peer để đối chiếu")
}

// Test_sync_mutation_full_payload_does_not_panic bảo vệ CHIỀU GHI của payload.
//
// Cùng lỗi, hiện ra bằng hình thức khác: `MergeResult` trong schema có
// `merged: Merged!` + `conflicts: [Conflict!]!` + `warnings: [String!]!` —
// 3 field NON-NULL. Nếu nhánh lỗi trả con trỏ nil cho chúng, gqlgen báo
// "null value for non-nullable field" ⇒ client nhận lỗi GraphQL rác thay vì
// đọc được message tiếng Việt trong `error`.
//
// Nhánh lỗi vẫn phải trả ĐỦ shape: đây là hợp đồng payload, không phải chuyện
// "chỉ cần ok=false".
func Test_sync_mutation_full_payload_does_not_panic(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)

	var resp struct {
		Sync struct {
			Ok     bool      `json:"ok"`
			Merged *struct { // pointer để phân biệt "null" với "thiếu field"
				Decks, Cards, Reviews, Notes                      int
				RoadmapPaths, RoadmapStages, RoadmapMilestones    int
				RoadmapTopics, RoadmapResources, RoadmapBookmarks int
			} `json:"merged"`
			Conflicts []struct{ GUID, Table, Field, Winner, Detail, ResolvedAt string } `json:"conflicts"`
			Warnings  []string                                                          `json:"warnings"`
			LastSync  string                                                            `json:"lastSyncAt"`
			Error     *userError                                                        `json:"error"`
		} `json:"sync"`
	}
	mustNotPanic(t, "mutation.sync (payload đầy đủ)", func() {
		c.MustPost(`mutation { sync {
			ok
			merged { decks cards reviews notes roadmapPaths roadmapStages
			         roadmapMilestones roadmapTopics roadmapResources roadmapBookmarks }
			conflicts { guid table field local incoming winner detail resolvedAt }
			warnings lastSyncAt
			error { message code }
		} }`, &resp)
	})

	require.False(t, resp.Sync.Ok)
	require.NotNil(t, resp.Sync.Error)
	require.NotEmpty(t, resp.Sync.Error.Message)
	require.NotNil(t, resp.Sync.Merged, "merged là non-null: nhánh lỗi cũng phải trả shape đầy đủ")
	require.NotNil(t, resp.Sync.Conflicts, "conflicts là non-null list: nil sẽ thành lỗi non-nullable")
	require.NotNil(t, resp.Sync.Warnings, "warnings là non-null list: nil sẽ thành lỗi non-nullable")
}

// Test_sync_mutation_invalid_payload_does_not_panic — nhánh "payload sai".
//
// `mutation.sync` KHÔNG có tham số nào (`sync: MergeResult!` trong
// `root.graphqls`), nên "payload sai" ở đây là query sai hình dạng: field không
// tồn tại, đối số thừa, sub-field trên scalar, cú pháp hỏng. Những lỗi này
// phải bị gqlgen chặn ở tầng validation và trả về cho test dưới dạng error —
// KHÔNG được nổ panic, và KHÔNG được chạm tới `Service.Sync` (tức là cũng
// không được chạm tới `SchemaLoader` nil).
func Test_sync_mutation_invalid_payload_does_not_panic(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)

	// Mỗi case đều là cách SAI THẬT SỰ, không phải cách "ít field hơn": GraphQL
	// cho phép chọn subset nên `sync { ok }` là hợp lệ — đừng nhầm 2 thứ này,
	// nếu không test sẽ "xanh" vì thứ vốn dĩ không sai.
	cases := []struct {
		name  string
		query string
	}{
		{"field không tồn tại", `mutation { sync { ok khôngCóFieldNày } }`},
		{"đối số thừa", `mutation { sync { ok(bogusArg: 1) } }`},
		{"chọn sub-field trên scalar", `mutation { sync { ok { viết sai kiểu } } }`},
		{"operation rỗng", `mutation { }`},
		{"field gốc sai tên", `mutation { syncAll { ok } }`},
		{"dấu ngoặc không cân", `mutation { sync { ok `},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp mergeResp
			mustNotPanic(t, "mutation.sync ("+tc.name+")", func() {
				// `Post` (không `MustPost`) vì ta KỲ vọNG lỗi validation.
				if err := c.Post(tc.query, &resp); err == nil {
					t.Errorf("query sai hình dạng phải bị gqlgen từ chối, nhưng lại chạy được")
				}
			})
		})
	}
}

// Test_sync_status_and_conflicts_still_work_after_sync_disabled bảo vệ đường
// ĐỌC cạnh nhánh vừa sửa.
//
// `Service.Sync` giờ dừng ở cổng `loader == nil` — nghĩa là `Merge` không bao
// giờ chạy trong app thật. Hai query đọc `syncStatus`/`syncConflicts` đi cùng
// service, nên phải chứng minh chúng vẫn trả dữ liệu thật (tức sửa B2 KHÔNG
// vô tình chặn luôn phần đọc). Không có test này thì "trả lỗi sớm" rất dễ biến
// thành "hỏng cả context sync" mà vẫn xanh.
func Test_sync_status_and_conflicts_still_work_after_sync_disabled(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)

	var resp struct {
		Status struct {
			Ok            bool `json:"ok"`
			StatusPayload *struct {
				Enabled       bool   `json:"enabled"`
				Strategy      string `json:"strategy"`
				LastSyncAt    string `json:"lastSyncAt"`
				ConflictCount int    `json:"conflictCount"`
			} `json:"status"`
			Error *userError `json:"error"`
		} `json:"syncStatus"`
		Conflicts struct {
			Ok        bool       `json:"ok"`
			Conflicts []struct{} `json:"conflicts"`
			Error     *userError `json:"error"`
		} `json:"syncConflicts"`
	}
	mustNotPanic(t, "query syncStatus + syncConflicts", func() {
		c.MustPost(`query {
			syncStatus { ok status { enabled strategy lastSyncAt conflictCount } error { message code } }
			syncConflicts(limit: 10) { ok conflicts { guid } error { message code } }
		}`, &resp)
	})

	require.True(t, resp.Status.Ok, "query syncStatus phải còn chạy được sau khi sửa B2")
	require.NotNil(t, resp.Status.StatusPayload)
	require.Equal(t, "last-write-win", resp.Status.StatusPayload.Strategy)
	require.True(t, resp.Conflicts.Ok, "query syncConflicts phải còn chạy được sau khi sửa B2")
}
