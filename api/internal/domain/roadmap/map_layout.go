package roadmap

import (
	"math"
	"sort"
)

// Hằng số viewBox của bản đồ. Tỉ lệ 1:2 (cao hơn rộng) vì direction `up`
// chiếm ưu thế: 1 path có ~5 stage × ~5 topic = 25 node, đi dọc dài hơn đi ngang.
const (
	// MapViewWidth × MapViewHeight là viewBox canvas ảo, M6 đặt
	// `<svg viewBox="0 0 1000 2000">` và scale theo viewport. Mọi toạ độ
	// ComputeLayout trả về nằm trong hộp này — không có node nào tràn ra
	// ngoài, kể cả khi stage vô tình có hàng trăm topic.
	MapViewWidth  = 1000.0
	MapViewHeight = 2000.0

	// MapMargin là lề an toàn (đơn vị viewBox) giữ node khỏi chạm mép canvas.
	// Cũng là khoảng trống dành cho terrain vẽ nền.
	MapMargin = 120.0

	// mapWavePeriods là số chu kỳ đường sin dọc theo trục chính. Cố ý dùng
	// SỐ NGUYÊN: với node đầu và node cuối (t = 0 và t = 1) sin đều về 0, hai
	// đầu bản đồ thẳng nhau, giống đường núi có đường lên xuống rõ ràng.
	mapWavePeriods = 1.0
)

// SortedTopics trả bản SAO của slice đã sắp xếp theo (position, id) — không
// mutate input. Mọi hàm layout/state đều sort trước để kết quả không phụ thuộc
// thứ tự caller đưa vào; `position` có thể trùng (user nhập tay) nên `id` là
// khoá phụ cho thứ tự ổn định.
func SortedTopics(topics []Topic) []Topic {
	out := make([]Topic, len(topics))
	copy(out, topics)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// LevelState là trạng thái hiển thị của 1 node trên bản đồ. Suy ra từ
// `status`, KHÔNG lưu DB (ROADMAP-MAP-IDEA §3) — không có chuyện lệch trạng
// thái giữa DB và bản đồ.
type LevelState string

const (
	// LevelDone: đã qua (status = done hoặc skipped), có ngôi sao, chơi lại được.
	LevelDone LevelState = "done"
	// LevelCurrent: node sáng, vòng hào động, có nút "vào".
	LevelCurrent LevelState = "current"
	// LevelLocked: mờ, icon ổ khoá, không bấm được.
	LevelLocked LevelState = "locked"
)

// AllLevelStates là tập hợp hợp lệ, dùng cho vòng lặp test.
var AllLevelStates = []LevelState{LevelDone, LevelCurrent, LevelLocked}

// LevelStates suy ra trạng thái từng node theo thứ tự position tăng dần.
//
// Quy tắc (ROADMAP-MAP-IDEA §3):
//   - `done` khi status = done HOẶC skipped — skipped coi như done để không
//     khoá màn sau (người dùng cố ý bỏ qua 1 màn, không phải bị chặn vĩnh viễn).
//   - `current` cho node đầu tiên chưa done, và cho mọi node mà màn trước đã
//     done. Chỉ có tối đa 1 `current` mở khóa chuỗi tại mỗi thời điểm.
//   - `locked` còn lại.
//
// Trả về slice CÙNG THỨ TỰ với SortedTopics đầu vào — caller ghép index.
func LevelStates(topics []Topic) []LevelState {
	ordered := SortedTopics(topics)
	out := make([]LevelState, len(ordered))
	// Node đầu tiên luôn mở: màn khoá ngay từ đầu thì user không có cách nào
	// bắt đầu, và level 1 vốn không có bài trước để làm mốc.
	frontierOpen := true
	for i, t := range ordered {
		switch {
		case t.Status.IsTerminal():
			out[i] = LevelDone
			frontierOpen = true
		case frontierOpen:
			out[i] = LevelCurrent
			frontierOpen = false
		default:
			out[i] = LevelLocked
		}
	}
	return out
}

// ComputeLayout tính toạ độ node trên bản đồ. HÀM THUẦN — cùng input luôn ra
// cùng output, không đọc đồng hồ, không random (ROADMAP-MAP-IDEA §2: "cùng
// position luôn ra cùng toạ độ, không phụ thuộc render").
//
// ViewBox: 0 0 1000 2000 (cao hơn rộng vì direction `up` chiếm ưu thế).
//   - `up`: node 0 ở gần đáy (Y lớn), node cuối ở gần đỉnh (Y nhỏ).
//   - `right`: node 0 ở bên trái (X nhỏ), node cuối bên phải (X lớn).
//
// Trục chính chia đều theo index; trục phụ là đường sin dao động quanh tâm với
// biên độ theo terrain (volcano 70 > meadow 40 > ocean 30 > snow 35 > desert 25
// > city 20).
//
// Node nào đã có `map_x`/`map_y` do user đặt tay thì lấy giá trị đó, bỏ qua
// đường sin — ưu tiên dữ liệu user hơn layout tự động. Nếu chỉ set 1 trục thì
// trục đó lấy của user, trục còn lại lấy từ layout.
func ComputeLayout(topics []Topic, terrain Terrain, dir Direction) []MapPoint {
	ordered := SortedTopics(topics)
	if len(ordered) == 0 {
		return []MapPoint{}
	}
	// Terrain/direction rỗng hoặc sai (stage đọc từ DB cũ chưa có 4 cột M2)
	// → rơi về mặc định, KHÔNG đoán. Sai 1 giá trị không được làm hỏng cả map.
	if !terrain.Valid() {
		terrain = TerrainDefault
	}
	if !dir.Valid() {
		dir = DirectionDefault
	}

	axis := mainAxis(dir)
	span := axis.span()
	n := len(ordered)
	amp := terrain.Amplitude()

	points := make([]MapPoint, n)
	for i, t := range ordered {
		var auto MapPoint
		// t đi từ 0 (node đầu) tới 1 (node cuối). Node đơn lẻ không có
		// "cuối" nên t=0: dấu hiệu "bắt đầu ở đây" khớp với node 0 của mọi
		// bản đồ dài hơn, và sin(0)=0 nên nó nằm đúng trục giữa.
		progress := 0.0
		if n > 1 {
			progress = float64(i) / float64(n-1)
		}
		// Đường sin: điểm đầu (t=0) và điểm cuối (t=1) cùng về 0 → 2 đầu map
		// thẳng, nhìn như đường núi có nhịp lên rõ.
		wave := math.Sin(2 * math.Pi * mapWavePeriods * progress)
		along := MapMargin + progress*span
		across := axis.center + amp*wave

		if dir == DirectionUp {
			auto = MapPoint{X: across, Y: axis.end - progress*span}
		} else {
			auto = MapPoint{X: along, Y: across}
		}

		points[i] = overrideWithUserPoint(auto, t)
	}
	return points
}

// overrideWithUserPoint thay toạ độ tự tính bằng giá trị user đặt tay ở những
// trục có set (MapX/MapY khác nil).
func overrideWithUserPoint(auto MapPoint, t Topic) MapPoint {
	if t.MapX != nil {
		auto.X = *t.MapX
	}
	if t.MapY != nil {
		auto.Y = *t.MapY
	}
	return auto
}

// axis mô tả trục chính của bản đồ theo direction: `up` dùng trục Y (đi từ
// `end` về `start` tức từ dưới lên), `right` dùng trục X (đi từ trái sang
// phải). `end` là toạ độ lớn (đáy map), `start` là toạ độ nhỏ (đỉnh map).
type axis struct {
	start  float64
	end    float64
	center float64
}

// span là chiều dài dùng được của trục chính (đã trừ 2 lề).
func (a axis) span() float64 { return a.end - a.start }

// mainAxis trả thông số trục chính cho 1 direction. Biên lề MapMargin giữ
// node khỏi chạm mép canvas.
func mainAxis(dir Direction) axis {
	if dir == DirectionRight {
		return axis{start: MapMargin, end: MapViewWidth - MapMargin, center: MapViewHeight / 2}
	}
	return axis{start: MapMargin, end: MapViewHeight - MapMargin, center: MapViewWidth / 2}
}

// TerrainWavePeriods là số chu kỳ đường sin dọc trục chính (hiện 1 vì 1 node
// = 1 nhịp lên). Export cho frontend M6 dùng CÙNG hằng số để vẽ đường đi
// khớp toạ độ node, thay vì tự chọn con số khác.
const TerrainWavePeriods = mapWavePeriods
