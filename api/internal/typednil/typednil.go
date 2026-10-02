// Package typednil soi "con trỏ nil bị nhét vào interface" — bẫy kinh điển của Go.
//
// Vì sao cần 1 package riêng thay vì 1 hàm `x == nil` ở chỗ cần: dự án này đã
// GẶP LỚP LỖI NÀY 3 LẦN, mỗi lần một chỗ khác nhau, mỗi lần đều dẫn tới
// PANIC 500 thay vì lỗi nghiệp vụ rõ ràng:
//
//	M2 F2 — `txOf` (infrastructure/sync) rơi về pool im lặng khi handle sai kiểu.
//	M7a B2 — `Wire` truyền `syncinfra.NewSchemaLoader(nil)` cho `SnapshotLoader`
//	         ⇒ `s.loader == nil` sai ⇒ `l.peer.WithContext(ctx)` deref
//	           `*gorm.DB` nil ⇒ `mutation.sync` panic.
//	M7a B1 — `Options.Backup` nhận `(*pgBackup)(nil)` ⇒ `port == nil` sai ⇒
//	         `port.Dump(...)` panic thay vì 501.
//
// Nguyên nhân gốc LUÔN NHƯ NHAU: so sánh `iface == nil` chỉ kiểm tra 2 từ của
// interface value (type + con trỏ dữ liệu), KHÔNG đi vào dữ liệu mà con trỏ
// trỏ tới. Sên `(*T)(nil)` ép vào interface ⇒ interface KHÔNG nil.
//
// Gói 1 hàm vào 1 package CÓ TÊN để lỗi này, lần sau gặp lại, chỉ cần gọi
// `typednil.Is(...)` — thay vì mỗi chỗ tự viết `x == nil` rồi quên.
//
// Hướng phụ thuộc: package LÁ, chỉ import `reflect`. Nhờ vậy `application`,
// `transport` và `infrastructure` đều dùng được mà không tạo vòng phụ thuộc
// (luật tầng 1 chiều của STACK-V2-PLAN §2).
package typednil

import "reflect"

// Is báo `v` có phải "không có gì dùng được" hay không.
//
// ── PHẠM VI CỐ Ý HẸP: CHỈ CON TRỎ / FUNC / UNSAFE-POINTER Ở TẦNG NGOÀI CÙNG ──
//
// TRUE cho 3 hình dạng mà `x == nil` bỏ sót hoặc xử lý sai:
//
//	nil                          — interface nil thật (truyền `nil`).
//	(*T)(nil) ép vào interface   — con trỏ nil BỌC TRONG interface. Đây là hình
//	                                thức nguy hiểm nhất: `v == nil` là FALSE,
//	                                nên mọi lời gọi method tiếp theo deref
//	                                con trỏ nil ⇒ PANIC.
//	(*I)(nil) ép vào interface   — con trỏ tới 1 interface đang nil, tức
//	                                "typed-nil 2 tầng": `v == nil` FALSE và
//	                                interface bên trong cũng nil.
//
// FALSE cho mọi giá trị dùng được, kể cả con trỏ TỚI (không phải nil) — vì đó
// là hiện thực hợp lệ của port.
//
// ── VÌ SAO KHÔNG SOI MAP / SLICE / CHAN ──────────────────────────────────
//
// Bản đầu tiên của hàm còn soi `Map`/`Slice`/`Chan` và coi nil là "chưa dùng
// được". Đó là SAI về ngữ nghĩa Go, và sai ở cả 2 chiều:
//
//	nil map   — `range`, `len`, đọc, `delete` đều HỢP LỆ (chạy 0 vòng). Chỉ
//	            GHI (`m[k] = v`) mới panic. Nên "nil map không dùng được" là
//	            khẳng định sai, và caller buộc phải tự dựng map mới trước khi
//	            ghi — tức `Is` đang trả lời thay 1 câu hỏi mà caller KHÔNG hỏi.
//	nil slice — `len`, `range`, `append`, `copy` đều HỢP LỆ. Append vào nil
//	            slice chính là cách khởi tạo slice trong Go. Coi nil slice là
//	            "chưa dùng được" là sai hoàn toàn.
//	nil chan  — gửi/nhận treo vĩnh viễn, nhưng đây là hành vi của `chan`,
//	            không phải thứ `== nil` trên interface trả lời sai.
//
// Và quyết định quan trọng nhất là về **ranh giới trách nhiệm**: hàm này tồn
// tại vì `== nil` trên interface trả lời SAI ("interface có chứa con trỏ nil
// không" ≠ "có dùng được không"). Đó là câu hỏi về TẦNG NGOÀI CÙNG. `map`/
// `slice`/`chan` là CÁI CHỨA bên trong; caller muốn hỏi "cái chứa này ghi
// được không" thì phải hỏi bằng câu của `map`/`slice`, và câu đó **không phải
// câu mà hàm này sinh ra để trả lời**. Giữ 3 loại đó vào chỉ làm hàm rộng hơn
// mà không thêm sức bắt nào cho 3 lỗi đã gặp.
//
// Khảo sát call site (M7b, 2026-09-29) xác nhận: toàn bộ repo có **2** chỗ
// gọi `Is` — `application/sync.Service.Sync` (`SnapshotLoader`) và
// `transport/http.backupDisabled` (`BackupPort`) — và cả hai đều là interface
// port có hiện thực bằng CON TRỎ. 15 phép `iface == nil` còn lại trong
// `internal/` (`content.data/tone/decks/cards`, `practice.stt/tts`,
// `roadmap.decks`, `*.uow`, `platform.di.go:275`) cũng đều là interface port.
// **Không chỗ nào truyền nil map/slice** ⇒ thu hẹp không làm hỏng gì, và
// `Test_Is_ignores_nil_map_and_slice` khoá ranh giới mới.
//
// ── HAI LOẠI "RỖNG" PHẢI PHÂN BIỆT ──────────────────────────────────────
//
//	map RỖNG (`map[string]int{}`) → FALSE: dùng được tự nhiên, ghi/đọc đều OK.
//	map NIL                     → FALSE (xem trên): caller tự quyết có ghi vào
//	                               không; hàm này không quyết thay.
//	slice RỖNG / slice NIL      → FALSE, cùng lý do.
//
// Ai đó viết `m != nil` để hỏi "map này có dữ liệu chưa" vẫn sai, nhưng cái
// sai đó thuộc về `m`, không thuộc về `Is`.
//
// Vì sao phải dùng `reflect` chứ không giải quyết ở chỗ gọi: người gọi chỉ giữ
// interface, không biết kiểu động bên trong. Chỉ `reflect` mới soi được — và
// soi 1 chỗ duy nhất tốt hơn mỗi call site tự viết.
//
// Lưu ý khi dùng: `Is` chỉ QUAN SÁT, không gọi bất kỳ method nào trên `v`.
// Đừng dùng nó để "kiểm tra xem object còn sống không" — với interface không
// có method thì không có gì để soi ngoài con trỏ nil.
func Is(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Func, reflect.UnsafePointer:
		// Đây là 3 kiểu mà nil ⇒ gọi bất kỳ thứ gì lên nó cũng nổ (func) hoặc
		// deref vùng nhớ không tồn tại (con trỏ). KHÔNG thêm `Map`/`Slice`/
		// `Chan` — xem khối giải thích ở doc trên.
		if rv.IsNil() {
			return true
		}
		// Con trỏ KHÔNG nil tới 1 interface đang nil: `var p *I = &i` với `i` nil.
		// `p != nil` nên nhánh trên không bắt được, nhưng `p.Do()` vẫn nổ vì bên
		// trong không có gì cả. Phải đào thêm 1 tầng.
		if rv.Kind() == reflect.Pointer && rv.Type().Elem().Kind() == reflect.Interface {
			return rv.Elem().IsNil()
		}
		return false
	case reflect.Interface:
		// `reflect.ValueOf` LUÔN trả về kiểu ĐỘNG, mà kiểu động của interface là
		// kiểu cụ thể — nên nhánh này không reachable qua chữ ký `any`. Giữ lại
		// để hàm đúng nếu sau này ai đó đổi tham số, và để nhánh `Interface`
		// trong doc (`interface nil thật`) có chỗ hiện thực tương ứng.
		return rv.IsNil()
	default:
		return false
	}
}
