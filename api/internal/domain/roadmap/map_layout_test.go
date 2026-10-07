package roadmap

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_terrain_valid_covers_frozen_whitelist(t *testing.T) {
	for _, tr := range AllTerrains {
		assert.True(t, tr.Valid(), "terrain %q phải hợp lệ", tr)
	}
	assert.Len(t, AllTerrains, 6, "whitelist 6 loại địa hình là hợp đồng đóng băng")
	for _, tr := range []Terrain{"", "Meadow", "forest", "swamp"} {
		assert.False(t, tr.Valid(), "terrain %q ngoài whitelist", tr)
	}
}

func Test_terrain_amplitude_orders_volcano_highest_city_lowest(t *testing.T) {
	// ROADMAP-MAP-IDEA §2: "desert phẳng hơn, volcano gấp thêm". `Amplitude` giờ
	// là TRỌNG SỐ TƯƠNG ĐỐI, không phải đơn vị viewBox — thứ tự là hợp đồng.
	assert.Equal(t, 70.0, TerrainVolcano.Amplitude())
	assert.Equal(t, 40.0, TerrainMeadow.Amplitude())
	assert.Equal(t, 35.0, TerrainSnow.Amplitude())
	assert.Equal(t, 30.0, TerrainOcean.Amplitude())
	assert.Equal(t, 25.0, TerrainDesert.Amplitude())
	assert.Equal(t, 20.0, TerrainCity.Amplitude())
	assert.Greater(t, TerrainVolcano.Amplitude(), TerrainDesert.Amplitude())
	assert.Greater(t, TerrainDesert.Amplitude(), TerrainCity.Amplitude())
}

// WaveDepth là con số ComputeLayout thật sự nhân vào biên độ. Nó phải giữ thứ
// tự terrain, và phải nằm trong dải hẹp — dải này là thứ giữ cho node không
// tràn khung mà đường vẫn ngoằn ngoèo (xem hằng số trong map_value_object.go).
func Test_terrain_wave_depth_keeps_order_inside_bounded_band(t *testing.T) {
	for _, tr := range AllTerrains {
		assert.GreaterOrEqual(t, tr.WaveDepth(), terrainWaveDepthMin, "terrain %q", tr)
		assert.LessOrEqual(t, tr.WaveDepth(), terrainWaveDepthMax, "terrain %q", tr)
	}
	assert.Equal(t, terrainWaveDepthMax, TerrainVolcano.WaveDepth(), "volcano = đỉnh dải")
	assert.Equal(t, terrainWaveDepthMin, TerrainCity.WaveDepth(), "city = đáy dải")
	assert.Greater(t, TerrainVolcano.WaveDepth(), TerrainMeadow.WaveDepth())
	assert.Greater(t, TerrainMeadow.WaveDepth(), TerrainDesert.WaveDepth())
	assert.Greater(t, TerrainDesert.WaveDepth(), TerrainCity.WaveDepth())
	// Terrain ngoài whitelist (Amplitude = 0) vẫn phải ra depth hợp lệ, KHÔNG
	// được âm/NaN — ComputeLayout đã Valid() trước, nhưng hàm phải tự vệ.
	bad := Terrain("forest").WaveDepth()
	assert.GreaterOrEqual(t, bad, terrainWaveDepthMin)
	assert.LessOrEqual(t, bad, terrainWaveDepthMax)
	assert.False(t, math.IsNaN(bad))
}

func Test_direction_valid_covers_two_values(t *testing.T) {
	assert.True(t, DirectionUp.Valid())
	assert.True(t, DirectionRight.Valid())
	assert.False(t, Direction("").Valid())
	assert.False(t, Direction("down").Valid())
	assert.False(t, Direction("Up").Valid())
	assert.Len(t, AllDirections, 2)
}

func Test_parse_terrain_defaults_blank_and_lists_whitelist_on_error(t *testing.T) {
	got, err := ParseTerrain("")
	require.NoError(t, err)
	assert.Equal(t, TerrainDefault, got, "rỗng = meadow, khớp DEFAULT của cột")

	got, err = ParseTerrain("  VOLCANO ")
	require.NoError(t, err)
	assert.Equal(t, TerrainVolcano, got)

	_, err = ParseTerrain("forest")
	require.Error(t, err)
	assert.Equal(t, "terrain chỉ nhận: meadow, desert, snow, volcano, ocean, city", err.Error())
}

func Test_parse_direction_defaults_blank_and_lists_whitelist_on_error(t *testing.T) {
	got, err := ParseDirection("")
	require.NoError(t, err)
	assert.Equal(t, DirectionDefault, got)

	got, err = ParseDirection("RIGHT")
	require.NoError(t, err)
	assert.Equal(t, DirectionRight, got)

	_, err = ParseDirection("left")
	require.Error(t, err)
	assert.Equal(t, "direction chỉ nhận: up, right", err.Error())
}

// ── LevelStates ─────────────────────────────────────────────────────────────

func Test_level_states_first_node_current_then_done_then_locked(t *testing.T) {
	// Màn 1 chưa làm, màn 2 đã xong, màn 3 chưa làm, màn 4 chưa làm.
	got := LevelStates([]Topic{
		{ID: 1, Position: 0, Status: NotStarted},
		{ID: 2, Position: 1, Status: Done},
		{ID: 3, Position: 2, Status: NotStarted},
		{ID: 4, Position: 3, Status: NotStarted},
	})
	assert.Equal(t, []LevelState{LevelCurrent, LevelDone, LevelCurrent, LevelLocked}, got)
}

func Test_level_states_first_node_done_unlocks_second(t *testing.T) {
	got := LevelStates([]Topic{
		{ID: 1, Position: 0, Status: Done},
		{ID: 2, Position: 1, Status: NotStarted},
		{ID: 3, Position: 2, Status: NotStarted},
	})
	assert.Equal(t, []LevelState{LevelDone, LevelCurrent, LevelLocked}, got)
}

func Test_level_states_treat_skipped_as_done(t *testing.T) {
	// ROADMAP-MAP-IDEA §3: skipped không khoá màn sau.
	got := LevelStates([]Topic{
		{ID: 1, Position: 0, Status: Skipped},
		{ID: 2, Position: 1, Status: NotStarted},
	})
	assert.Equal(t, []LevelState{LevelDone, LevelCurrent}, got)
}

func Test_level_states_all_done_is_all_done(t *testing.T) {
	got := LevelStates([]Topic{
		{ID: 1, Status: Done},
		{ID: 2, Status: Done},
		{ID: 3, Status: Skipped},
	})
	assert.Equal(t, []LevelState{LevelDone, LevelDone, LevelDone}, got)
}

func Test_level_states_empty_and_single(t *testing.T) {
	assert.Empty(t, LevelStates(nil))
	assert.Equal(t, []LevelState{LevelCurrent}, LevelStates([]Topic{{ID: 1, Status: NotStarted}}))
	assert.Equal(t, []LevelState{LevelDone}, LevelStates([]Topic{{ID: 1, Status: Done}}))
}

func Test_level_states_follow_position_not_input_order(t *testing.T) {
	got := LevelStates([]Topic{
		{ID: 3, Position: 2, Status: NotStarted},
		{ID: 1, Position: 0, Status: Done},
		{ID: 2, Position: 1, Status: Done},
	})
	assert.Equal(t, []LevelState{LevelDone, LevelDone, LevelCurrent}, got,
		"thứ tự phải theo position, không theo thứ tự caller đưa vào")
}

func Test_level_states_only_one_node_current_at_a_time(t *testing.T) {
	// 2 màn liên tiếp chưa làm sau 1 màn đã xong: chỉ màn kề màn done mở,
	// không mở cả chuỗi phía sau.
	got := LevelStates([]Topic{
		{ID: 1, Status: Done},
		{ID: 2, Status: InProgress},
		{ID: 3, Status: InProgress},
		{ID: 4, Status: NotStarted},
	})
	assert.Equal(t, []LevelState{LevelDone, LevelCurrent, LevelLocked, LevelLocked}, got)
}

// ── ComputeLayout ───────────────────────────────────────────────────────────

func layoutFixture(n int) []Topic {
	out := make([]Topic, n)
	for i := range out {
		out[i] = Topic{ID: int64(i + 1), Position: i}
	}
	return out
}

// extent là bbox của tập toạ độ — đơn vị đo giống hệt
// `web/src/roadmap/map/geometry.ts fitFrame` (bỏ phần padding, vì padding là
// quyết định của lớp trình bày Phase B chứ không phải của hợp đồng hình học).
func extent(points []MapPoint) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range points {
		minX = math.Min(minX, p.X)
		minY = math.Min(minY, p.Y)
		maxX = math.Max(maxX, p.X)
		maxY = math.Max(maxY, p.Y)
	}
	return
}

// assertInsideCanvas kiểm node nằm trong canvas ĐÚNG CỦA STAGE NÀY: bề rộng
// động theo số node, chiều cao cố định — không dùng cặp hằng "biên validate"
// vì cặp đó rộng hơn canvas thật.
func assertInsideCanvas(t *testing.T, points []MapPoint, dir Direction) {
	t.Helper()
	n := len(points)
	alongHi := MapCanvasWidth(n)
	if dir == DirectionUp {
		// `up` hoán đổi 2 trục: trục chính là Y, trục phụ là X.
		for i, p := range points {
			assert.GreaterOrEqual(t, p.Y, 0.0, "node %d tràn ngoài canvas", i)
			assert.LessOrEqual(t, p.Y, alongHi, "node %d tràn ngoài canvas", i)
			assert.GreaterOrEqual(t, p.X, 0.0, "node %d tràn ngoài canvas", i)
			assert.LessOrEqual(t, p.X, MapCanvasHeight, "node %d tràn ngoài canvas", i)
		}
		return
	}
	for i, p := range points {
		assert.GreaterOrEqual(t, p.X, 0.0, "node %d tràn ra ngoài canvas bên trái", i)
		assert.LessOrEqual(t, p.X, alongHi, "node %d tràn ra ngoài canvas bên phải", i)
		assert.GreaterOrEqual(t, p.Y, 0.0, "node %d tràn ra ngoài canvas phía trên", i)
		assert.LessOrEqual(t, p.Y, MapCanvasHeight, "node %d tràn ra ngoài canvas phía dưới", i)
	}
}

// ── HỢP ĐỒNG HÌNH HỌC (6 điều kiện gate Phase A) ────────────────────────────
//
// Bug nền: bản đồ là dải DỌC (SVG 261×1952 trong khung 1377×620, aspect 0.13).
// Sáu test dưới đây là tiêu chí gate — mỗi test khoá MỘT điều kiện, và mỗi
// điều kiện đều đã được mutation-check (sửa ngược ⇒ test phải FAIL).

// (1) X tăng NGHIÊM NGẶT theo index, và mọi node nằm trong
//
//	[MapMargin, MapCanvasWidth(n) − MapMargin].
func Test_contract_right_axis_increases_strictly_and_respects_margin(t *testing.T) {
	for _, terrain := range AllTerrains {
		for _, n := range []int{2, 3, 4, 5, 9, 12, 51, 60} {
			points := ComputeLayout(layoutFixture(n), terrain, DirectionRight)
			require.Len(t, points, n)

			for i := 1; i < n; i++ {
				assert.Greater(t, points[i].X, points[i-1].X,
					"terrain=%s n=%d: node %d phải nằm bên phải node %d", terrain, n, i, i-1)
			}
			// Node đầu sát lề trái, node cuối sát lề phải ⇒ dùng hết bề ngang.
			assert.Equal(t, MapMargin, points[0].X)
			assert.InDelta(t, MapCanvasWidth(n)-MapMargin, points[n-1].X, 1e-9)
			for i, p := range points {
				assert.GreaterOrEqual(t, p.X, MapMargin, "terrain=%s n=%d node %d", terrain, n, i)
				assert.LessOrEqual(t, p.X, MapCanvasWidth(n)-MapMargin, "terrain=%s n=%d node %d", terrain, n, i)
			}
		}
	}
}

// (2) Y NGOẰN NGOÈO THẬT: (maxY−minY)/MapCanvasHeight ≥ MapMinWiggleRatio, với
//
//	MỌI terrain và mọi n ≥ 2. Đây là điều kiện chống "đường thẳng" — biên độ
//	cũ 20–70 trên canvas cao 900 chỉ là 2–8% ⇒ đường thẳng tuyệt đối.
//
//	n=1 không kiểm: 1 node thì maxY = minY theo định nghĩa, không có "đường".
func Test_contract_secondary_axis_really_wiggles_for_every_terrain_and_size(t *testing.T) {
	for _, terrain := range AllTerrains {
		for n := 2; n <= 60; n++ {
			points := ComputeLayout(layoutFixture(n), terrain, DirectionRight)
			_, minY, _, maxY := extent(points)
			ratio := (maxY - minY) / MapCanvasHeight
			assert.GreaterOrEqual(t, ratio, MapMinWiggleRatio,
				"terrain=%s n=%d: (maxY−minY)/height = %.4f < %.2f ⇒ đường thẳng, không phải đường uốn",
				terrain, n, ratio, MapMinWiggleRatio)
		}
	}
}

// (3) Khung LANDSCAPE: MapCanvasWidth(n)/MapCanvasHeight ≥ MapMinLandscapeAspect
//
//	khi n ≥ 4. n < 4 không ép (1 node không cần bản đồ rộng).
func Test_contract_canvas_is_landscape_for_four_nodes_or_more(t *testing.T) {
	for n := 4; n <= 60; n++ {
		aspect := MapCanvasWidth(n) / MapCanvasHeight
		assert.GreaterOrEqual(t, aspect, MapMinLandscapeAspect,
			"n=%d: width/height = %.3f < %.2f ⇒ canvas portrait, cuộn dọc", n, aspect, MapMinLandscapeAspect)
	}
	// Bảo đảm n<4 KHÔNG bị ép (nếu ai đó đổi công thức thành luôn ≥ 1.6 thì
	// 1 node sẽ nằm giữa trang trắng).
	assert.Less(t, MapCanvasWidth(3)/MapCanvasHeight, MapMinLandscapeAspect)
}

// (4) Bề rộng TĂNG theo số node, chiều cao CỐ ĐỊNH — đây là điều làm bản đồ
//
//	cuộn NGANG thay vì cuộn dọc.
func Test_contract_width_grows_with_node_count_and_height_is_fixed(t *testing.T) {
	prev := 0.0
	for n := 1; n <= 60; n++ {
		w := MapCanvasWidth(n)
		assert.Greater(t, w, prev, "n=%d: bề rộng phải tăng so với n=%d", n, n-1)
		prev = w
		// Chiều cao không đổi: node của stage 60 node vẫn nằm trong cùng dải
		// cao 900 như stage 2 node — dải cao không phình theo số node.
		points := ComputeLayout(layoutFixture(n), TerrainVolcano, DirectionRight)
		for i, p := range points {
			assert.LessOrEqual(t, p.Y, MapCanvasHeight, "n=%d node %d", n, i)
		}
	}
	// Tỉ lệ bề rộng/càng tăng ⇒ đường dài ra theo hướng đi, đây là cơ chế
	// khiến bản đồ cuộn NGANG thay vì cuộn dọc.
	assert.Greater(t, MapCanvasWidth(60)/MapCanvasWidth(5), 10.0)
}

// (5) TẤT ĐỊNH: cùng input ⇒ cùng output, không phụ thuộc thứ tự caller đưa vào.
func Test_compute_layout_is_deterministic_under_shuffled_input(t *testing.T) {
	base := layoutFixture(6)

	for _, terrain := range AllTerrains {
		for _, dir := range AllDirections {
			want := ComputeLayout(base, terrain, dir)
			require.Len(t, want, 6)

			// Đảo thứ tự input 3 kiểu khác nhau — kết quả phải y hệt.
			shuffles := [][]Topic{
				{base[5], base[0], base[3], base[1], base[4], base[2]},
				{base[2], base[4], base[1], base[5], base[0], base[3]},
				{base[3], base[1], base[5], base[2], base[0], base[4]},
			}
			for _, shuffled := range shuffles {
				got := ComputeLayout(shuffled, terrain, dir)
				assert.Equal(t, want, got, "terrain=%s dir=%s: layout phải phụ thuộc position, không phụ thuộc thứ tự input", terrain, dir)
			}

			// Cùng input gọi 3 lần — không đọc đồng hồ, không random.
			assert.Equal(t, want, ComputeLayout(base, terrain, dir))
			assert.Equal(t, want, ComputeLayout(base, terrain, dir))
		}
	}
}

// (6) Node không tràn ra ngoài khung, và còn chỗ cho nhãn + vòng hào động: biên
//
//	độ lớn nhất (volcano) phải nằm trong [MapMargin, height−MapMargin].
func Test_contract_nodes_stay_inside_frame_with_room_for_labels_and_halo(t *testing.T) {
	for _, terrain := range AllTerrains {
		for _, dir := range AllDirections {
			for _, n := range []int{1, 2, 3, 5, 12, 51, 60} {
				points := ComputeLayout(layoutFixture(n), terrain, dir)
				require.Len(t, points, n, "terrain=%s dir=%s n=%d phải ra đúng n node", terrain, dir, n)
				assertInsideCanvas(t, points, dir)

				for i, p := range points {
					if dir == DirectionUp {
						assert.GreaterOrEqual(t, p.X, MapMargin, "node %d sát mép trái", i)
						assert.LessOrEqual(t, p.X, MapCanvasHeight-MapMargin, "node %d sát mép phải", i)
					} else {
						assert.GreaterOrEqual(t, p.Y, MapMargin, "node %d sát mép trên", i)
						assert.LessOrEqual(t, p.Y, MapCanvasHeight-MapMargin, "node %d sát mép dưới", i)
					}
				}
			}
		}
	}
}

func Test_compute_layout_does_not_mutate_input(t *testing.T) {
	input := []Topic{
		{ID: 3, Position: 2},
		{ID: 1, Position: 0},
		{ID: 2, Position: 1},
	}
	before := append([]Topic(nil), input...)
	ComputeLayout(input, TerrainMeadow, DirectionUp)
	LevelStates(input)
	assert.Equal(t, before, input, "sort trước rồi làm việc trên bản sao, không được sắp lại slice của caller")
}

// `up` phải VẪN vẽ được (tương thích ngược: DB của user có stage `up` cũ), và
// tọa độ phải hợp lệ chứ không phải toạ độ vô nghĩa.
func Test_compute_layout_up_still_renders_bottom_to_top(t *testing.T) {
	for _, n := range []int{1, 3, 5, 12} {
		points := ComputeLayout(layoutFixture(n), TerrainMeadow, DirectionUp)
		require.Len(t, points, n)
		assert.Equal(t, MapCanvasWidth(n)-MapMargin, points[0].Y, "n=%d: node 0 ở đáy", n)
		for i := 1; i < n; i++ {
			assert.Less(t, points[i].Y, points[i-1].Y, "node %d phải cao hơn node %d", i, i-1)
		}
		assert.InDelta(t, MapMargin, points[n-1].Y, 1e-9)
		// Trục phụ (X) vẫn ngoằn ngoèo, không dồn về 1 điểm.
		_, minX, _, maxX := extent(points)
		if n >= 2 {
			assert.Greater(t, maxX-minX, 0.0, "n=%d: dir=up mà X đứng yên", n)
		}
	}
}

func Test_compute_layout_empty_returns_empty_slice(t *testing.T) {
	got := ComputeLayout(nil, TerrainMeadow, DirectionUp)
	assert.NotNil(t, got, "trả slice rỗng chứ không nil — client JSON ra [] chứ không null")
	assert.Empty(t, got)
}

func Test_compute_layout_invalid_terrain_falls_back_to_meadow(t *testing.T) {
	topics := layoutFixture(4)
	want := ComputeLayout(topics, TerrainMeadow, DirectionUp)
	got := ComputeLayout(topics, Terrain("forest"), DirectionUp)
	assert.Equal(t, want, got, "terrain rỗng/ngoài whitelist phải rơi về meadow, không đoán")

	gotBad := ComputeLayout(topics, TerrainMeadow, Direction("diagonal"))
	require.NotEmpty(t, gotBad, "direction lạ phải vẫn ra được node — không panic, không rỗng")
	for i, p := range gotBad {
		assert.False(t, math.IsNaN(p.X) || math.IsNaN(p.Y), "node %d toạ độ NaN", i)
		assert.GreaterOrEqual(t, p.X, 0.0)
		assert.GreaterOrEqual(t, p.Y, 0.0)
	}
}

func Test_compute_layout_user_override_wins_over_wave(t *testing.T) {
	x, y := 321.0, 654.0
	topics := layoutFixture(3)
	topics[1].MapX = &x
	topics[1].MapY = &y

	points := ComputeLayout(topics, TerrainVolcano, DirectionRight)
	assert.Equal(t, MapPoint{X: 321, Y: 654}, points[1], "giá trị user phải thắng layout tự động")
	// Node không set vẫn theo layout → chứng minh chỉ node đó bị override.
	assert.NotEqual(t, MapPoint{X: 321, Y: 654}, points[0])
}

func Test_compute_layout_user_override_on_one_axis_only(t *testing.T) {
	x := 100.0
	topics := layoutFixture(3)
	topics[2].MapX = &x

	points := ComputeLayout(topics, TerrainMeadow, DirectionRight)
	assert.Equal(t, 100.0, points[2].X, "trục X do user đặt phải giữ nguyên")
	auto := ComputeLayout(layoutFixture(3), TerrainMeadow, DirectionRight)
	assert.Equal(t, auto[2].Y, points[2].Y, "trục Y chưa set thì lấy từ layout")
}

// Số chu kỳ đường sin: stage dài có ~2–3 chữ S thay vì 1 chữ S quá dài, và
// luôn ≥ 1 (bản đồ 2 node vẫn phải uốn, không được thẳng).
func Test_wave_periods_scale_with_node_count_and_stay_integral(t *testing.T) {
	assert.Equal(t, 1, WavePeriods(1))
	assert.Equal(t, 1, WavePeriods(5))
	assert.Equal(t, 2, WavePeriods(9))
	assert.Equal(t, 3, WavePeriods(13))
	assert.Equal(t, 15, WavePeriods(60))
	for n := 1; n <= 60; n++ {
		assert.GreaterOrEqual(t, WavePeriods(n), 1, "n=%d", n)
		assert.LessOrEqual(t, WavePeriods(n), 20, "n=%d", n)
	}
}

func Test_sorted_topics_breaks_position_tie_by_id(t *testing.T) {
	got := SortedTopics([]Topic{
		{ID: 9, Position: 1},
		{ID: 3, Position: 0},
		{ID: 7, Position: 1},
	})
	assert.Equal(t, []int64{3, 7, 9}, []int64{got[0].ID, got[1].ID, got[2].ID},
		"2 node cùng position phải ổn định theo id, không theo thứ tự input")
}

// ── MapDefaults ─────────────────────────────────────────────────────────────

func Test_map_defaults_match_roadmap_map_idea_section_6(t *testing.T) {
	// Bảng §6 là yêu cầu cứng: Trung meadow→meadow→desert→snow→volcano,
	// Anh meadow→ocean→city→snow→volcano.
	zhWant := []Terrain{TerrainMeadow, TerrainMeadow, TerrainDesert, TerrainSnow, TerrainVolcano}
	for i, want := range zhWant {
		got, dir := MapDefaults("zh", i)
		assert.Equal(t, want, got, "zh stage %d", i)
		assert.Equal(t, DirectionRight, dir, "toàn bộ path Trung đi ngang, stage %d", i)
	}
	enWant := []Terrain{TerrainMeadow, TerrainOcean, TerrainCity, TerrainSnow, TerrainVolcano}
	for i, want := range enWant {
		got, dir := MapDefaults("en", i)
		assert.Equal(t, want, got, "en stage %d", i)
		assert.Equal(t, DirectionRight, dir, "en stage %d", i)
	}
}

// C3: KHÔNG stage seed nào còn `up`.
//
// Bug class "hai nguồn sự thật": bảng seed trước đây có 7 stage `up` (gồm G0
// của CẢ 2 path) và 3 stage `right`, nên landing view của cả 2 path đều mở ra
// chặng `up` — bản đồ dọc. Test này là hàng rào: chỉ cần 1 dòng trong bảng
// đổi lại `up` là test đỏ ngay.
func Test_map_defaults_no_seed_stage_is_vertical(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for i := 0; i < 5; i++ {
			_, dir := MapDefaults(lang, i)
			assert.Equal(t, DirectionRight, dir,
				"%s-g%d phải đi ngang — landing view mở `up` là bản đồ dọc hỏng", lang, i)
		}
	}
}

func Test_map_defaults_out_of_range_falls_back_to_column_default(t *testing.T) {
	for _, c := range []struct {
		lang  string
		index int
	}{
		{"zh", 5}, {"en", 5}, {"zh", -1}, {"vi", 0}, {"", 0},
	} {
		t.Run(fmt.Sprintf("lang=%s index=%d", c.lang, c.index), func(t *testing.T) {
			terrain, dir := MapDefaults(c.lang, c.index)
			assert.Equal(t, TerrainDefault, terrain, "lang=%s index=%d", c.lang, c.index)
			assert.Equal(t, DirectionDefault, dir, "lang=%s index=%d", c.lang, c.index)
		})
	}
}
