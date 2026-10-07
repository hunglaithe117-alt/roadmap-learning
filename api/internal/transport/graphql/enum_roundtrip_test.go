// Test enum ⇄ domain ⇄ DB — phần bổ sung cho `enum_contract_test.go`.
//
// `enum_contract_test.go` so whitelist danh sách (schema ⇄ domain ⇄ CHECK).
// File này kiểm những thứ DANH SÁCH không nói được:
//
//  1. `ErrorCode` không phải whitelist danh sách mà là ÁNH XẠ sang HTTP status
//     — phải kiểm bằng cách gọi hàm ánh xạ thật.
//  2. `terrainOf` / `terrainText` là 2 chiều ĐI QUA SCHEMA: enum sai thì
//     `terrainOf` rơi về `default` và trả giá trị hợp lệ ⇒ UI nhận địa hình
//     sai mà không có lỗi nào. Test danh sách ở trên KHÔNG bắt được; chỉ
//     round-trip mới bắt.

package graphql

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	syncapp "langapp/internal/application/sync"
	domainroadmap "langapp/internal/domain/roadmap"
	"langapp/internal/transport/graphql/model"
)

// Test_graphql_error_code_covers_business_status — `ErrorCode` ⇄ HTTP status.
//
// `ErrorCode` là enum DUY NHẤT không ánh xạ 1-1 với whitelist domain: nó ánh
// xạ sang HTTP status mà 6 package application mang trong `Error.Status`.
//
// Chiều 1: mỗi status NGHIỆP VỤ (400/404/409/501) phải có 1 `ErrorCode` riêng —
// gộp 2 status vào 1 code là mất thông tin cho client ("không tìm thấy" và
// "trùng" là 2 việc khác nhau, UI hiển thị khác nhau).
// Chiều 2: mỗi `ErrorCode` trong schema phải được `codeForStatus` trả về
// được — enum có giá trị không ai sinh ra là code chết, và code chết thì
// client so sánh `code` rỡ thành so sánh message (thứ M4 đã cấm — message là
// tiếng Việt dành cho người đọc, không phải khoá máy).
func Test_graphql_error_code_covers_business_status(t *testing.T) {
	business := map[int]model.ErrorCode{
		http.StatusBadRequest:     model.ErrorCodeBadRequest,
		http.StatusNotFound:       model.ErrorCodeNotFound,
		http.StatusConflict:       model.ErrorCodeConflict,
		http.StatusNotImplemented: model.ErrorCodeNotImplemented,
	}
	for status, want := range business {
		t.Run(fmt.Sprintf("HTTP %d", status), func(t *testing.T) {
			assert.Equal(t, want, codeForStatus(status),
				"HTTP %d phải ánh xạ sang ErrorCode riêng, không gộp", status)
		})
	}
	// Không phải nghiệp vụ ⇒ hệ thống. 500 là hằng của application
	// (`StatusInternalServerError`), dùng ở đây để test KHÔNG phụ thuộc
	// `net/http` cho giá trị này.
	//
	// 503 CỐ Ý nằm trong danh sách này: 503 nghĩa là "tạm thời không phục vụ
	// được" (quá tải, bảo trì) — nó là lỗi HỆ THỐNG, không phải lời hứa với
	// client. `application/sync` dùng 501 chứ không dùng 503 cho "chưa cấu
	// hình" đúng vì lý do này; test khoá ranh giới đó.
	for _, status := range []int{500, 502, 503, 418} {
		t.Run(fmt.Sprintf("HTTP %d", status), func(t *testing.T) {
			assert.Equal(t, model.ErrorCodeInternal, codeForStatus(status), "HTTP %d phải là INTERNAL", status)
		})
	}
	// Mọi code trong schema đều phải sinh ra được.
	produced := map[model.ErrorCode]bool{}
	for _, code := range business {
		produced[code] = true
	}
	produced[model.ErrorCodeInternal] = true
	for _, code := range model.AllErrorCode {
		t.Run(code.String(), func(t *testing.T) {
			assert.True(t, produced[code], "enum %s có trong schema nhưng không status nào sinh ra nó — code chết", code)
		})
	}
}

// Test_sync_missing_peer_maps_to_not_implemented_not_internal khoá đúng hậu quả
// của việc thêm 501: `codeForStatus` có nhánh riêng, nhưng nếu ai đó sửa
// `isBusinessStatus` mà quên thêm 501 thì message tiếng Việt bị che thành
// `"lỗi hệ thống"` + `INTERNAL` — tức quay về đúng bài toán M7c đã sửa.
//
// Test này gọi `toUserError` với đúng lỗi mà `Service.Sync` trả, không dựa
// vào Postgres.
func Test_sync_missing_peer_maps_to_not_implemented_not_internal(t *testing.T) {
	// Dựng lỗi đúng hình dạng tầng application dựng, không đi vòng qua
	// `Service.Sync`: `newError` không export, còn test này cần khẳng định
	// hành vi của TRANSPORT trên 1 lỗi có status 501. Dùng hằng
	// `StatusNotImplemented` chứ không literal `501` — nếu hằng bị đổi thì
	// test phải đỏ chứ không âm thầm kiểm nhầm mã khác.
	appErr := &syncapp.Error{
		Status:  syncapp.StatusNotImplemented,
		Message: "chưa cấu hình nguồn snapshot peer",
	}

	ue := toUserError(appErr)
	require.NotNil(t, ue)
	assert.Equal(t, "chưa cấu hình nguồn snapshot peer", ue.Message,
		"message tiếng Việt phải ra nguyên văn — 501 là nghiệp vụ, không phải lỗi hệ thống")
	assert.Equal(t, model.ErrorCodeNotImplemented, ue.Code,
		"client cần phân biệt 'tính năng chưa bật' với 'server hỏng' để không bảo user thử lại vô ích")
	assert.NotEqual(t, model.ErrorCodeInternal, ue.Code)
}

// Test_terrain_survives_db_to_client_to_db_roundtrip — trái tim của M7a.
//
// Đây là test bắt được bug `enum Terrain` cũ. Chuỗi lỗi gốc:
//
//	DB lưu "snow"  →  terrainOf("snow") không match case nào
//	                 →  default trả "PLAIN"  →  client đổi thành "Cao nguyên"
//	                 →  terrainText trả "hill"  →  DB nhận "hill"
//	                 →  CHECK constraint (23514) từ chối  →  hoặc tệ hơn:
//	                    nếu không có CHECK thì ghi sai địa hình, im lặng.
//
// Test khẳng định: MỌI hằng domain đi qua `terrainOf` rồi `terrainText` thì
// về đúng giá trị ban đầu — trừ hằng DEFAULT (`meadow`) vì `terrainText` cố ý
// trả "" để DB gán DEFAULT (xem comment hàm).
func Test_terrain_survives_db_to_client_to_db_roundtrip(t *testing.T) {
	for _, want := range domainroadmap.AllTerrains {
		t.Run(string(want), func(t *testing.T) {
			// DB → client.
			enum := terrainOf(string(want))
			assert.True(t, enum.IsValid(), "terrainOf(%q) trả %q không phải giá trị hợp lệ của enum", want, enum)
			if want != domainroadmap.TerrainDefault {
				assert.NotEqual(t, model.TerrainMeadow, enum,
					"terrainOf(%q) rơi về DEFAULT MEADOW — enum schema lệch whitelist domain", want)
			} else {
				// DEFAULT đi tới đúng 1 enum: rơi về giá trị khác cũng là lỗi
				// (client sẽ vẽ sai địa hình cho mọi stage còn `meadow`).
				assert.Equal(t, model.TerrainMeadow, enum,
					"terrainOf(%q) phải trả MEADOW, không phải %q", want, enum)
			}

			// client → DB.
			back, err := terrainText(&enum)
			require.NoError(t, err, "mọi giá trị hợp lệ của enum phải map được")
			if want == domainroadmap.TerrainDefault {
				assert.Empty(t, back,
					"terrainText(MEADOW) phải trả \"\" để DB gán DEFAULT, không ghi \"meadow\" (ghi sẽ đổi updated_at mỗi patch)")
				return
			}
			assert.Equal(t, string(want), back,
				"vòng DB→client→DB đổi %q thành %q", want, back)
		})
	}
}

// Test_terrain_of_rejects_out_of_range_values ràng chặn DB hỏng.
//
// Cột `terrain` có CHECK nên DB không cho ghi giá trị lạ — NHƯNG merge của
// `sync` ghi thẳng SQL và CHECK bị bỏ qua khi restore từ file `.dump` cũ hơn.
// `terrainOf` phải trả 1 giá trị HỢP LỆ (không phải enum không tồn tại) cho
// mọi input, vì GraphQL không có "enum rỗng" và trả cái gì đó ngoài whitelist
// là lỗi serialization 500.
func Test_terrain_of_rejects_out_of_range_values(t *testing.T) {
	for _, bad := range []string{"", "SNOW", "swamp", "meadow ", "0", "PLAIN"} {
		t.Run(bad, func(t *testing.T) {
			got := terrainOf(bad)
			assert.True(t, got.IsValid(), "terrainOf(%q) = %q không hợp lệ — GraphQL sẽ serialize lỗi 500", bad, got)
			assert.Equal(t, model.TerrainMeadow, got,
				"giá trị không hợp lệ phải rơi về DEFAULT để DB/UI không nhận địa hình bịa ra")
		})
	}
}

// Test_terrain_text_of_every_enum_value_is_valid_db_value khẳng định chiều
// ngược cả 6 giá trị enum, không chỉ 5 giá trị domain.
//
// Bổ sung cho test trên: test trên đi từ domain sang enum, test này đi từ
// enum sang domain. Chiều này bắt được việc thêm 1 giá trị enum mà không thêm
// hằng domain — trường hợp `Test_domain_constant_has_matching_enum_value` ở
// file kia đã bắt, nhưng test ở đây bắt thêm hậu quả cụ thể: giá trị đó sẽ
// đẩy vào DB một chuỗi mà CHECK từ chối.
func Test_terrain_text_of_every_enum_value_is_valid_db_value(t *testing.T) {
	for _, enum := range model.AllTerrain {
		t.Run(enum.String(), func(t *testing.T) {
			got, err := terrainText(&enum)
			require.NoError(t, err, "enum %q hợp lệ mà không map được", enum)
			if got == "" {
				require.Equal(t, model.TerrainMeadow, enum,
					"chỉ DEFAULT (meadow) được phép rỗng; %q rỗng nghĩa là giá trị bị nuốt", enum)
				return
			}
			tn := domainroadmap.Terrain(got)
			assert.True(t, tn.Valid(),
				"enum %q ánh xạ thành %q — CHECK constraint của cột terrain sẽ từ chối (23514)", enum, got)
		})
	}
}

// Test_direction_survives_db_to_client_to_db_roundtrip — cùng lớp lỗi, enum
// kế bên.
//
// `Direction` có 2 giá trị nên bug "rơi về default" kém hiển trị hơn `Terrain`
// (5/6), nhưng cùng cấu trúc: `terrain` cũ rơi về PLAIN vì 5 case lệch tên;
// `direction` có `right` khớp nên chưa lộ. Không có test thì 1 lần đổi tên
// hằng sẽ lặp lại đúng lỗi đó.
func Test_direction_survives_db_to_client_to_db_roundtrip(t *testing.T) {
	for _, want := range domainroadmap.AllDirections {
		t.Run(string(want), func(t *testing.T) {
			enum := directionOf(string(want))
			back, err := directionText(&enum)
			require.NoError(t, err, "mọi giá trị hợp lệ của enum phải map được")
			if want == domainroadmap.DirectionDefault {
				assert.Empty(t, back, "directionText(UP) phải trả \"\" để DB gán DEFAULT")
				return
			}
			assert.Equal(t, string(want), back, "vòng DB→client→DB đổi %q thành %q", want, back)
		})
	}
}

// ── Chiều GHI: giá trị không map được phải bị TỪ CHỐI, không rơi về DEFAULT ──

// Test_terrain_text_rejects_value_it_cannot_map — chặn MẤT DỮ LIỆU Ở CHIỀU GHI.
//
// Đây là hậu quả của `enum Terrain` cũ ở **chiều ngược lại** (chiều ghi), khác
// hẳn chiều đọc mà `Test_terrain_survives_db_to_client_to_db_roundtrip` bắt.
//
// Đường mất dữ liệu trước khi sửa:
//
//	client chọn 1 địa hình mà `switch` chưa biết
//	  → terrainText trả "" (default của switch)
//	  → application/roadmap.parseMapField("" , domain.ParseTerrain)
//	  → ParseTerrain("") coi rỗng là "lấy DEFAULT" và trả "meadow", KHÔNG LỖI
//	  → UPDATE ghi "meadow"
//	  → user thấy bản đồ đổi sang Đồng cỏ, KHÔNG có message lỗi nào
//	  → updated_at vẫn đổi nên node còn bị đẩy qua sync
//
// Còn hôm nay enum GraphQL đóng (gqlgen chặn giá trị lạ ở tầng validate) nên
// đường này **chưa kích hoạt được từ client**. Nó kích hoạt ngay khi ai đó thêm
// giá trị thứ 7 vào `enum Terrain` mà quên cập nhật `switch` — đúng cái lỗi đã
// xảy ra 2 lần trước (F9 M2, F1 M4) và 1 lần ở chính `enum Terrain`.
//
// Vì vậy test dựng giá trị lạ TRỰC TIẾP (`model.Terrain` là kiểu string) thay
// vì đi vòng qua GraphQL — GraphQL không cho gửi giá trị ngoài enum, nên đó là
// cách duy nhất chạm tới nhánh này.
func Test_terrain_text_rejects_value_it_cannot_map(t *testing.T) {
	bad := model.Terrain("SWAMP")
	got, err := terrainText(&bad)

	require.Error(t, err,
		"giá trị không map được phải trả LỖI — trả \"\" khiến ParseTerrain rơi về DEFAULT và user mất lựa chọn mà không có tín hiệu gì")
	assert.Empty(t, got, "không được trả giá trị nào kèm lỗi")
	assert.Equal(t, http.StatusBadRequest, statusOf(err),
		"phải là 400 (lỗi tham số của client) chứ không phải 500 — client cần biết đó là lỗi của mình để hiện lựa chọn hợp lệ")
	// Message phải liệt kê đủ 6 giá trị hợp lệ: user cần biết chọn cái gì, không
	// phải đi đoán.
	for _, name := range domainroadmap.AllTerrains {
		assert.Contains(t, err.Error(), string(name),
			"message lỗi phải liệt kê %q để client hiện được lựa chọn đúng", name)
	}
}

// Test_terrain_text_accepts_absent_and_default — 2 trạng thái HỢP LỆ trả "" và
// KHÔNG lỗi, để bảo đảm fix trên không làm hỏng 2 trường hợp hợp lệ đó.
//
// `nil` = client không gửi (PATCH: không đụng cột; create: lấy DEFAULT).
// `MEADOW` = client chọn Đồng cỏ ⇒ `ParseTerrain("")` gán "meadow" ⇒ vẫn đúng.
func Test_terrain_text_accepts_absent_and_default(t *testing.T) {
	got, err := terrainText(nil)
	require.NoError(t, err, "không gửi terrain KHÔNG phải lỗi")
	assert.Empty(t, got)

	meadow := model.TerrainMeadow
	got, err = terrainText(&meadow)
	require.NoError(t, err, "MEADOW là DEFAULT, phải qua")
	assert.Empty(t, got, "MEADOW đi qua \"\" để ParseTerrain gán DEFAULT, tránh ghi thừa mỗi patch")
}

// Test_direction_text_rejects_value_it_cannot_map — cùng lớp lỗi, nặng hơn.
//
// `directionText` trước khi sửa rơi về `"right"` cho mọi giá trị lạ — tức KHÔNG
// phải DEFAULT, nên bản đồ vẽ sai CHIỀU cuộn (vuốt dọc thành vuốt ngang) mà
// không có lỗi nào cho user.
func Test_direction_text_rejects_value_it_cannot_map(t *testing.T) {
	bad := model.Direction("DIAGONAL")
	got, err := directionText(&bad)

	require.Error(t, err, "hướng đi lạ phải bị từ chối, không rơi về \"right\"")
	assert.Empty(t, got)
	assert.Equal(t, http.StatusBadRequest, statusOf(err))
	assert.Contains(t, err.Error(), "up")
	assert.Contains(t, err.Error(), "right")
}

// Test_direction_text_maps_right_explicitly — chống lại "fix" quá tay biến
// RIGHT thành lỗi (nó là giá trị hợp lệ, chỉ UP mới là DEFAULT).
func Test_direction_text_maps_right_explicitly(t *testing.T) {
	got, err := directionText(nil)
	require.NoError(t, err)
	assert.Empty(t, got, "nil = không gửi = không đụng")

	up := model.DirectionUp
	got, err = directionText(&up)
	require.NoError(t, err, "UP là DEFAULT, phải qua")
	assert.Empty(t, got)

	right := model.DirectionRight
	got, err = directionText(&right)
	require.NoError(t, err)
	assert.Equal(t, "right", got, "RIGHT phải ghi tường minh — nếu rơi về \"\" thì PATCH không đụng cột")
}

// ── BookmarkStatus: mẫu tiềm ẩn của F1, chưa sống nhưng phải khoá ──────────

// Test_bookmark_status_survives_db_to_client_to_db_roundtrip — chiều ĐỌC rồi
// CHIỀU GHI, giống hệt test của `terrain`.
//
// Cặp này là nơi mất dữ liệu sẽ xảy ra nếu 2 bảng lệch: DB lưu `"reading"`
// → `bookmarkStatusOf` → `READING` → client đổi thành `"Đang đọc"` →
// `bookmarkStatusText` phải trả LẠI `"reading"`. Lệch 1 tên là vòng đó đổi
// thành `"to_read"` và kho link của user tự dịch chuyển trạng thái.
func Test_bookmark_status_survives_db_to_client_to_db_roundtrip(t *testing.T) {
	for _, want := range domainroadmap.AllBookmarkStatuses {
		t.Run(string(want), func(t *testing.T) {
			enum := bookmarkStatusOf(string(want))
			back, err := bookmarkStatusText(&enum)
			require.NoError(t, err, "mọi giá trị hợp lệ của domain phải map được")
			assert.Equal(t, string(want), back, "vòng DB→client→DB đổi %q thành %q", want, back)
		})
	}
}

// Test_bookmark_status_text_rejects_value_it_cannot_map là test CỐT LÕI của
// việc tách 3 trạng thái.
//
// `bookmarkStatusText` trước khi sửa rơi về `"to_read"` cho MỌI giá trị lạ.
// Hôm nay enum GraphQL khớp domain 1-1 (4 hằng) và `enum_contract_test.go`
// khoá, nên nhánh này **chưa kích hoạt được từ client** — nhưng thêm status
// thứ 5 vào `enum BookmarkStatus` là đúng cơ chế F1 (M4): im lặng thành giá
// trị hợp lệ, không tín hiệu nào cho user.
//
// Đường mất dữ liệu nếu không chặn:
//
//	client chọn status mới (chưa có trong switch)
//	  → bookmarkStatusText trả "to_read"
//	  → ValidateBookmarkStatus("to_read") HỢP LỆ ⇒ không lỗi
//	  → UPDATE ghi "to_read"
//	  → user thấy bookmark tự nhảy về "Chưa đọc", KHÔNG có message lỗi nào
//	  → updated_at vẫn đổi nên dòng đó còn bị đẩy qua sync
//
// Vì vậy test dựng giá trị lạ TRỰC TIẾP (`model.BookmarkStatus` là kiểu
// string) thay vì đi vòng qua GraphQL — giống `Test_terrain_text_rejects_
// value_it_cannot_map`.
func Test_bookmark_status_text_rejects_value_it_cannot_map(t *testing.T) {
	// KHÔNG đưa `"TO_READ"` (tên enum) vào danh sách này: ở CHIỀU GHI, giá trị
	// đi vào `bookmarkStatusText` là tên enum GraphQL, nên `TO_READ` hợp lệ —
	// trộn 2 chiều vào 1 danh sách là loại test "xanh vì thứ vốn dĩ không sai".
	// Tên hằng DB (`to_read`) là thứ `bookmarkStatusOf` nhận, xem test kế bên.
	for _, bad := range []string{"SWAMP", "", "in_progress", "not_started", "0", "To_Read"} {
		t.Run(bad, func(t *testing.T) {
			v := model.BookmarkStatus(bad)
			got, err := bookmarkStatusText(&v)

			require.Error(t, err,
				"giá trị không map được phải trả LỖI — rơi về \"to_read\" khiến ValidateBookmarkStatus "+
					"HỢP LỆ và user mất lựa chọn mà không có tín hiệu nào")
			assert.Empty(t, got, "không được trả giá trị nào kèm lỗi")
			assert.Equal(t, http.StatusBadRequest, statusOf(err),
				"phải là 400 (lỗi tham số của client) chứ không phải 500")
			// Liệt kê đủ 4 giá trị hợp lệ: user cần biết chọn cái gì, không
			// phải đi đoán.
			for _, name := range domainroadmap.AllBookmarkStatuses {
				assert.Contains(t, err.Error(), string(name),
					"message lỗi phải liệt kê %q để client hiện được lựa chọn đúng", name)
			}
		})
	}
}

// Test_bookmark_status_text_separates_absent_from_default khoá 2 trạng thái
// HỢP LỆ mà việc tách 3 trạng thái dễ gộp nhầm.
//
// `nil` = client không gửi. `TO_READ` = client CHỌN "Chưa đọc". Hai thứ cho
// cùng kết quả ở `createBookmark` (`ValidateBookmarkStatus("")` gán DEFAULT)
// nhưng KHÁC nhau ở `bookmarks(status:)`: `""` = không lọc, `"to_read"` = lọc
// đúng nhóm đó. Gộp chúng ⇒ lọc `TO_READ` trả về toàn bộ kho link, sai lặng.
func Test_bookmark_status_text_separates_absent_from_default(t *testing.T) {
	got, err := bookmarkStatusText(nil)
	require.NoError(t, err, "không gửi status KHÔNG phải lỗi")
	assert.Empty(t, got, `nil = client không gửi = "" (create: DEFAULT, lọc: không lọc)`)

	toRead := model.BookmarkStatusToRead
	got, err = bookmarkStatusText(&toRead)
	require.NoError(t, err, "TO_READ là giá trị hợp lệ, phải qua")
	assert.Equal(t, "to_read", got,
		`TO_READ phải ghi TƯỜNG MINH "to_read" — nếu rút về "" thì lọc status:TO_READ trả về MỌI bookmark`)
}

// Test_bookmark_status_of_accepts_every_db_value — chiều ĐỌC: DB có thể trả
// gì, `bookmarkStatusOf` phải trả 1 enum HỢP LỆ (không phải enum không tồn
// tại, vì GraphQL sẽ serialize lỗi 500).
//
// `default` trả `TO_READ` ở đây là ĐÚNG và khác hẳn `bookmarkStatusText`:
// chiều đọc không có "ghi vào DB" để mất dữ liệu, chỉ cần không trả giá trị
// ngoài enum. Cột `status` có CHECK nên chỉ hỏng khi merge `sync` ghi thẳng
// SQL từ file `.dump` cũ hơn.
func Test_bookmark_status_of_accepts_every_db_value(t *testing.T) {
	// Đây là CHIỀU ĐỌC: đầu vào là chuỗi trong CỘT `roadmap_bookmarks.status`
	// (`to_read`/`reading`/…), KHÔNG phải tên enum GraphQL — nên `"TO_READ"`
	// (tên enum) là giá trị LẠ ở đây và phải rơi về DEFAULT.
	for _, bad := range []string{"", "TO_READ", "swamp", "reading ", "0", "not_started"} {
		t.Run(bad, func(t *testing.T) {
			got := bookmarkStatusOf(bad)
			assert.True(t, got.IsValid(), "bookmarkStatusOf(%q) = %q không hợp lệ — GraphQL sẽ serialize lỗi 500", bad, got)
			assert.Equal(t, model.BookmarkStatusToRead, got)
		})
	}
}
