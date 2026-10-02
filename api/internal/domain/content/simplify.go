package content

import (
	"strings"
)

// tradToSimp là bảng map phồn→giản tối thiểu, đồng bộ với
// web/src/player/diff.ts. Sửa 1 bên phải mirror bên kia + test cả 2.
// Bảng thuần Go, KHÔNG cgo: GOROOT máy này thiếu unicode/norm (xem
// STACK-V2-PLAN §8).
var tradToSimp = map[rune]rune{
	'國': '国', '語': '语', '學': '学', '習': '习', '漢': '汉', '麼': '么',
	'嗎': '吗', '們': '们', '說': '说', '話': '话', '愛': '爱', '樂': '乐',
	'龍': '龙', '鳳': '凤', '體': '体', '點': '点', '麵': '面', '頭': '头',
	'會': '会', '過': '过', '還': '还', '這': '这', '個': '个', '來': '来',
	'時': '时', '間': '间', '門': '门', '問': '问', '聽': '听', '讀': '读',
	'寫': '写', '開': '开', '關': '关', '車': '车', '馬': '马', '魚': '鱼',
	'鳥': '鸟', '麥': '麦', '黃': '黄', '傳': '传', '統': '统', '設': '设',
	'備': '备', '術': '术', '雜': '杂', '難': '难', '讓': '让', '認': '认',
	'識': '识', '覺': '觉', '親': '亲', '兒': '儿', '臺': '台', '灣': '湾',
	'廣': '广', '東': '东', '廠': '厂', '遠': '远', '運': '运', '連': '连',
	'適': '适', '選': '选', '錢': '钱', '銀': '银', '辦': '办', '發': '发',
	'長': '长', '對': '对', '導': '导', '後': '后', '復': '复', '複': '复',
	'團': '团', '圖': '图', '價': '价', '優': '优', '單': '单', '標': '标',
	'準': '准', '樣': '样', '檢': '检', '驗': '验', '豐': '丰', '豔': '艳',
	'勵': '励', '慶': '庆',
}

// ToSimplified đổi chữ phồn sang giản theo bảng trên; ký tự ngoài bảng giữ
// nguyên, đầu vào đã giản thì idempotent.
//
// Whisper trả chữ phồn (廣東話, 學習) trong khi deck/diff/UI dùng chữ giản —
// không chuẩn hóa sẽ chấm sai oan dù đọc đúng.
func ToSimplified(s string) string {
	if s == "" {
		return s
	}
	changed := false
	out := []rune(s)
	for i, r := range out {
		if simp, ok := tradToSimp[r]; ok {
			out[i] = simp
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(out)
}

// HasTraditional báo chuỗi còn chữ phồn cần chuẩn hóa — dùng để kiểm tra
// bảng tradToSimp có đủ ký tự trong test data hay không.
func HasTraditional(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		_, ok := tradToSimp[r]
		return ok
	})
}
