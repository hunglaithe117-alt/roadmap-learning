package typednil_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"langapp/internal/typednil"
)

type impl struct{ n int }

func (p *impl) Do() int { return p.n }

// iface là interface để dựng "con trỏ tới interface nil" (typed-nil 2 tầng).
type iface interface{ Do() int }

func Test_Is_detects_every_nil_shape(t *testing.T) {
	t.Run("interface nil thật", func(t *testing.T) {
		var port iface
		require.True(t, typednil.Is(port), "truyền `nil` vào tham số interface phải ra true")
	})

	t.Run("con trỏ nil trong interface", func(t *testing.T) {
		// ĐÂY là bẫy B1/B2. Kiểm chứng tiền đề: `port == nil` ở đây là FALSE,
		// nên nếu test này xanh mà `typednil.Is` cũng xanh thì hàm chỉ "đúng
		// một cách tình cờ" chứ chưa chứng minh nó bắt được bẫy.
		var p *impl
		var port iface = p
		require.False(t, port == nil, "tiền đề: con trỏ nil trong interface KHÔNG bằng nil")
		require.True(t, typednil.Is(port), "đây mới là hình thức phải bắt được")
	})

	t.Run("con trỏ tới interface nil", func(t *testing.T) {
		var inner iface
		port := &inner
		require.False(t, port == nil, "tiền đề: `port == nil` FALSE")
		require.True(t, typednil.Is(port), "typed-nil 2 tầng cũng phải bắt được")
		// Chi tiết ở Test_Is_pierces_pointer_to_nil_interface.
	})

	t.Run("con trỏ nil đã ép vào any", func(t *testing.T) {
		var p *impl
		var v any = p
		require.False(t, v == nil, "tiền đề: `any` chứa con trỏ nil vẫn khác nil")
		require.True(t, typednil.Is(v), "phải bắt được qua `any` — đó là chữ ký thật của hàm")
	})
}

func Test_Is_keeps_usable_values(t *testing.T) {
	// Phía dễ quên: hàm soi nil phải TRẢ FALSE cho mọi thứ dùng được. Hỏng
	// chỗ này cũng nguy hiểm không kém — `mutation.sync` sẽ 501 "chưa cấu
	// hình nguồn snapshot peer" vĩnh viễn dù peer đã được cấu hình, và test
	// nào cũng xanh vì "vẫn trả 501".
	real := &impl{n: 7}
	require.False(t, typednil.Is(real), "con trỏ TỚI là hiện thực hợp lệ, không phải nil")
	require.False(t, typednil.Is(impl{n: 7}), "giá trị (không phải con trỏ) cũng hợp lệ")
	require.False(t, typednil.Is(func() {}), "func không nil")
	require.False(t, typednil.Is(0), "số không nil")
	require.False(t, typednil.Is(""), "chuỗi rỗng KHÔNG phải nil — bẫy phụ, dễ viết `== \"\"` nhầm chỗ")
	require.False(t, typednil.Is(false), "bool false không phải nil")
}

// Test_Is_ignores_nil_map_and_slice khoá RANH GIỚI của hàm sau khi thu hẹp.
//
// Bản đầu tiên trả TRUE cho nil map/slice/chan. Đó SAI về ngữ nghĩa Go, và
// sai ở 2 chiều:
//
//   - nil `map`: `range`/`len`/đọc/`delete` đều hợp lệ (0 vòng). CHỈ ghi mới
//     panic. "Không dùng được" là khẳng định sai.
//   - nil `slice`: `len`/`range`/`append`/`copy` đều hợp lệ — append vào nil
//     slice CHÍNH LÀ cách khởi tạo slice trong Go. Sai hoàn toàn.
//
// Nên `Is` trả FALSE cho cả nil lẫn rỗng. Caller muốn hỏi "ghi vào cái này
// được không" thì hỏi bằng câu của `map`/`slice` — câu đó không phải câu mà
// hàm này sinh ra để trả lời (`== nil` trên interface hỏi "interface có chứa
// con trỏ nil không").
//
// Vì sao việc này là QUYẾT ĐỊNH chứ không phải sơ suất: khảo sát M7b xác nhận
// toàn repo có 1 call site (`SnapshotLoader`), là interface port có hiện thực
// bằng con trỏ ⇒ thu hẹp không làm hỏng gì. Nếu
// sau này có call site truyền nil map/slice, test này sẽ đỏ và bắt người viết
// phải nghĩ lại thay vì âm thầm nhận hành vi sai.
func Test_Is_ignores_nil_map_and_slice(t *testing.T) {
	var nilMap map[string]int
	require.False(t, typednil.Is(nilMap),
		"nil map KHÔNG phải Ptr/Func/UnsafePointer ⇒ hàm không soi tới. "+
			"Đọc/range/len vẫn hợp lệ; chỉ GHI mới panic, và việc đó là của caller")

	var nilSlice []int
	require.False(t, typednil.Is(nilSlice),
		"nil slice hợp lệ hoàn toàn: len=0, range 0 vòng, append tạo slice mới")

	var nilCh chan int
	require.False(t, typednil.Is(nilCh),
		"nil chan nằm ngoài phạm vi: hàm này bảo vệ call site interface, không thay luật của chan")

	// Bản RỖNG thì chưa bao giờ phải TRUE (đã đúng từ bản đầu) — giữ lại để
	// thấy ranh giới KHÔNG dịch chuyển: trước và sau đều FALSE.
	require.False(t, typednil.Is(map[string]int{}), "map rỗng dùng được tự nhiên")
	require.False(t, typednil.Is([]int{}), "slice rỗng dùng được")
}

// Test_Is_ignores_nil_map_hidden_behind_interface khoá ranh giới ở đúng hình
// thức nguy hiểm: nil map BỌC TRONG interface.
//
// Đây chính là hình thức mà `Is` sinh ra để bắt — nhưng với `map` thì bắt là
// SAI. `var p map[string]int; var v any = p` cho `v == nil` là FALSE (đúng
// như với con trỏ), và `v` vẫn dùng được cho mọi thao tác đọc. Hàm trả TRUE ở
// đây sẽ khiến 1 cổng "chưa cấu hình" chạy đúng lúc user đang dùng bình
// thường.
func Test_Is_ignores_nil_map_hidden_behind_interface(t *testing.T) {
	var p map[string]int
	var v any = p
	require.False(t, v == nil, "tiền đề: nil map trong `any` cũng khác nil")
	require.False(t, typednil.Is(v),
		"nil map bọc trong interface vẫn là FALSE — bắt nó là sai về ngữ nghĩa Go")

	var sl []int
	var vs any = sl
	require.False(t, vs == nil, "tiền đề: nil slice trong `any` cũng khác nil")
	require.False(t, typednil.Is(vs))
}

// Test_Is_pierces_pointer_to_nil_interface chốt tầng "typed-nil 2".
//
// `var p *I = &i` với `i` là interface nil: `p != nil` nên so sánh thường bỏ
// sót, nhưng `p.Do()` nổ ngay vì bên trong không có gì để gọi. Test này có
// thật sự cần trong codebase không? Chưa — nhưng hàm được viết để dùng ở MỮI
// tầng, và đây là hình dạng hợp lệ của Go. Bỏ nhánh này thì hàm sai lúc nào
// cũng, tức là sai khi đã không còn ai kiểm tra nữa.
func Test_Is_pierces_pointer_to_nil_interface(t *testing.T) {
	var inner iface // nil
	port := &inner
	require.False(t, port == nil, "tiền đề: so sánh thường bỏ sót hình thức này")
	require.True(t, typednil.Is(port), "phải đào thêm 1 tầng mới thấy interface nil bên trong")

	// Ngược lại: con trỏ tới interface ĐANG có giá trị thì phải FALSE.
	ok := iface(&impl{n: 3})
	good := &ok
	require.False(t, typednil.Is(good), "con trỏ tới interface có giá trị là hợp lệ")
}

func Test_Is_does_not_call_any_method(t *testing.T) {
	// `Is` chỉ soi bằng reflect. Nếu nó gọi method (VD để kiểm tra "còn sống"),
	// thì với con trỏ nil nó sẽ PANIC — tức là chính cái hàm sinh ra để chặn
	// panic lại trở thành nguồn panic.
	panicky := &panicsOnCall{}
	require.NotPanics(t, func() { typednil.Is(panicky) })
}

type panicsOnCall struct{}

func (p *panicsOnCall) Do() {
	panic("gọi method trên nil phải nổ, nhưng typednil.Is không được gọi")
}

// Test_Is_documented_contract ràng buộc hàm với đúng lời hứa trong doc.
func Test_Is_documented_contract(t *testing.T) {
	// `error` nil bọc trong interface cũng là một trong những nơi
	// `err != nil` sai — chính là lý do `errors.Is/As` ra đời. `typednil` phải
	// nhất quán với đó chứ không tự đặt ra tiêu chuẩn riêng cho nil.
	require.True(t, typednil.Is(nil), "nil thuần")
	var err error = errors.New("x")
	require.False(t, typednil.Is(err), "error thật")
	var nilErr error
	require.True(t, typednil.Is(nilErr), "error nil phải ra true")

	// Và phải là hàm TỔNG QUÁT: nhận `any` nên mọi port ở mọi tầng dùng được,
	// không cần viết lại cho từng interface.
	var s fmt.Stringer
	require.True(t, typednil.Is(s), "interface từ package chuẩn cũng phải soi được")
	require.False(t, typednil.Is(time.Second), "giá trị không nil của bất kỳ kiểu nào")
}
