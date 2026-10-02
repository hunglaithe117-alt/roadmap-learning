// Dữ liệu bút thuận cho chữ HSK. Port NGUYÊN VĂN từ api/chinese_hsk.go
// (strokeSeed) — thứ tự nét là dữ liệu học thuật, sai 1 nét là dạy sai.
//
// Nguồn: quy tắc bút thuận thông phổ theo 教育部《通用规范汉字笔顺规范》.
// Chữ phức tạp hơn (VD 马, 又) để phase sau khi có nguồn kiểm chứng được —
// nét sai còn tệ hơn thiếu.
package contentinfra

import (
	"sort"

	"langapp/internal/domain/content"
)

// StrokeStep là 1 nét trong thứ tự bút thuận chuẩn. Mã nét rút gọn cho mục
// đích sư phạm (h=ngang, s=sổ, p=phẩy, n=mác, d=chấm, hz=gập, sg=sổ móc,
// pd=phẩy-chấm) — KHÔNG phải mã CDL đầy đủ.
type StrokeStep struct {
	Order int
	Code  string
	Name  string
}

// StrokeInfo là dữ liệu bút thuận của 1 chữ Hán.
type StrokeInfo struct {
	Hanzi       string
	PinyinMarks string
	Level       string
	StrokeCount int
	Strokes     []StrokeStep
}

// strokeIndexItem là 1 dòng index nhẹ cho lazy-load (client tải danh sách
// chữ trước, rồi mới hỏi chi tiết từng chữ).
type strokeIndexItem struct {
	Hanzi       string
	StrokeCount int
}

// strokeSeed: 12 chữ HSK1 đơn thể chắc chắn về bút thuận + 4 chữ đơn
// HSK2-4 chắc chắn (千 L2, 门 L2, 万 L3, 刀 L4).
var strokeSeed = map[string]StrokeInfo{}

func init() {
	for _, s := range []StrokeInfo{
		stroke("人", "ren2", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("大", "da4", "HSK1", [][2]string{{"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("小", "xiao3", "HSK1", [][2]string{{"sg", "sổ móc (竖钩)"}, {"d", "chấm (点)"}, {"d", "chấm (点)"}}),
		stroke("中", "zhong1", "HSK1", [][2]string{{"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}}),
		stroke("日", "ri4", "HSK1", [][2]string{{"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}}),
		stroke("月", "yue4", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"hz", "gập ngang (横折)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}}),
		stroke("水", "shui3", "HSK1", [][2]string{{"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("女", "nü3", "HSK1", [][2]string{{"pd", "phẩy-chấm (撇点)"}, {"p", "phẩy (撇)"}, {"h", "ngang (横)"}}),
		stroke("子", "zi3", "HSK1", [][2]string{{"h", "ngang (横)"}, {"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}}),
		stroke("好", "hao3", "HSK1", [][2]string{{"pd", "phẩy-chấm (撇点)"}, {"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}, {"sg", "sổ móc (竖钩)"}, {"h", "ngang (横)"}}),
		stroke("生", "sheng1", "HSK1", [][2]string{{"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}, {"h", "ngang (横)"}}),
		stroke("天", "tian1", "HSK1", [][2]string{{"h", "ngang (横)"}, {"h", "ngang (横)"}, {"p", "phẩy (撇)"}, {"n", "mác (捺)"}}),
		stroke("千", "qian1", "HSK2", [][2]string{{"p", "phẩy (撇)"}, {"h", "ngang (横)"}, {"s", "sổ (竖)"}}),
		stroke("门", "men2", "HSK2", [][2]string{{"d", "chấm (点)"}, {"s", "sổ (竖)"}, {"hz", "gập ngang (横折)"}}),
		stroke("万", "wan4", "HSK3", [][2]string{{"h", "ngang (横)"}, {"hz", "gập ngang (横折)"}, {"p", "phẩy (撇)"}}),
		stroke("刀", "dao1", "HSK4", [][2]string{{"hz", "gập ngang (横折)"}, {"p", "phẩy (撇)"}}),
	} {
		strokeSeed[s.Hanzi] = s
	}
}

// stroke dựng 1 StrokeInfo. `PinyinMarks` gọi domain/content (nguồn duy nhất
// của quy tắc đặt dấu) thay vì copy hàm `PinyinMarks` của app v1.
func stroke(hanzi, pinyin, level string, codes [][2]string) StrokeInfo {
	steps := make([]StrokeStep, len(codes))
	for i, c := range codes {
		steps[i] = StrokeStep{Order: i + 1, Code: c[0], Name: c[1]}
	}
	return StrokeInfo{Hanzi: hanzi, PinyinMarks: content.PinyinMarks(pinyin),
		Level: level, StrokeCount: len(steps), Strokes: steps}
}

// StrokeIndex là index nhẹ 1 level: chữ + số nét, để client lazy-load.
func StrokeIndex(level string) []strokeIndexItem {
	out := []strokeIndexItem{}
	for _, s := range strokeSeed {
		if s.Level == level {
			out = append(out, strokeIndexItem{Hanzi: s.Hanzi, StrokeCount: s.StrokeCount})
		}
	}
	// Map iteration không có thứ tự: sắp theo chữ để response ổn định giữa
	// 2 lần gọi (UI không nhảy vị trí khi refresh).
	sort.Slice(out, func(i, j int) bool { return out[i].Hanzi < out[j].Hanzi })
	return out
}

// LookupStroke trả chi tiết bút thuận của 1 chữ trong level. Chữ không có dữ
// liệu HOẶC thuộc level khác → (nil, false); app v1 trả 404 cho cả 2 trường
// hợp vì client chỉ hỏi chữ của level đang xem.
func LookupStroke(level, hanzi string) (StrokeInfo, bool) {
	s, ok := strokeSeed[hanzi]
	if !ok || s.Level != level {
		return StrokeInfo{}, false
	}
	return s, true
}
