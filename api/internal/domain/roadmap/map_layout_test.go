package roadmap

import (
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
	// ROADMAP-MAP-IDEA §2: "desert phẳng hơn, volcano gấp thêm".
	assert.Equal(t, 70.0, TerrainVolcano.Amplitude())
	assert.Equal(t, 40.0, TerrainMeadow.Amplitude())
	assert.Equal(t, 35.0, TerrainSnow.Amplitude())
	assert.Equal(t, 30.0, TerrainOcean.Amplitude())
	assert.Equal(t, 25.0, TerrainDesert.Amplitude())
	assert.Equal(t, 20.0, TerrainCity.Amplitude())
	assert.Greater(t, TerrainVolcano.Amplitude(), TerrainDesert.Amplitude())
	assert.Greater(t, TerrainDesert.Amplitude(), TerrainCity.Amplitude())
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

func assertInsideViewBox(t *testing.T, points []MapPoint) {
	t.Helper()
	for i, p := range points {
		assert.GreaterOrEqual(t, p.X, 0.0, "node %d tràn ra ngoài viewBox bên trái", i)
		assert.LessOrEqual(t, p.X, MapViewWidth, "node %d tràn ra ngoài viewBox bên phải", i)
		assert.GreaterOrEqual(t, p.Y, 0.0, "node %d tràn ra ngoài viewBox trên", i)
		assert.LessOrEqual(t, p.Y, MapViewHeight, "node %d tràn ra ngoài viewBox dưới", i)
	}
}

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

func Test_compute_layout_up_goes_bottom_to_top(t *testing.T) {
	points := ComputeLayout(layoutFixture(5), TerrainMeadow, DirectionUp)
	require.Len(t, points, 5)
	// Node 0 sát đáy, node cuối sát đỉnh.
	assert.Equal(t, MapViewHeight-MapMargin, points[0].Y)
	assert.Equal(t, MapMargin, points[4].Y)
	for i := 1; i < len(points); i++ {
		assert.Less(t, points[i].Y, points[i-1].Y, "node %d phải cao hơn node %d khi đi lên trên", i, i-1)
	}
}

func Test_compute_layout_right_goes_left_to_right(t *testing.T) {
	points := ComputeLayout(layoutFixture(5), TerrainCity, DirectionRight)
	require.Len(t, points, 5)
	assert.Equal(t, MapMargin, points[0].X)
	assert.Equal(t, MapViewWidth-MapMargin, points[4].X)
	for i := 1; i < len(points); i++ {
		assert.Greater(t, points[i].X, points[i-1].X, "node %d phải nằm bên phải node %d", i, i-1)
	}
}

func Test_compute_layout_stays_inside_view_box_for_every_terrain_and_size(t *testing.T) {
	// Biên độ lớn nhất 70 (volcano) × 6 terrain, từ 1 tới 60 node (path seed
	// thật có 5 stage × 5 topic; 60 là biên trên xấu nhất mà vẫn cần vẽ được).
	for _, terrain := range AllTerrains {
		for _, dir := range AllDirections {
			for _, n := range []int{1, 2, 3, 5, 12, 51, 60} {
				points := ComputeLayout(layoutFixture(n), terrain, dir)
				require.Len(t, points, n, "terrain=%s dir=%s n=%d phải ra đúng n node", terrain, dir, n)
				assertInsideViewBox(t, points)
			}
		}
	}
}

func Test_compute_layout_amplitude_scales_with_terrain(t *testing.T) {
	// 5 node: node 1 có t = 0.25 → sin(2π·0.25) = 1, tức lệch khỏi trục giữa
	// đúng bằng biên độ khai báo. So sánh trên node đó thì khác biệt biên độ
	// là thật, không bị đường sin bù mất.
	spread := func(terrain Terrain) float64 {
		p := ComputeLayout(layoutFixture(5), terrain, DirectionUp)
		return math.Abs(p[1].X - MapViewWidth/2)
	}
	assert.InDelta(t, TerrainVolcano.Amplitude(), spread(TerrainVolcano), 1e-9)
	assert.InDelta(t, TerrainCity.Amplitude(), spread(TerrainCity), 1e-9)
	assert.Greater(t, spread(TerrainVolcano), spread(TerrainMeadow))
	assert.Greater(t, spread(TerrainMeadow), spread(TerrainDesert))
	assert.Greater(t, spread(TerrainDesert), spread(TerrainCity))
}

func Test_compute_layout_user_override_wins_over_wave(t *testing.T) {
	x, y := 321.0, 654.0
	topics := layoutFixture(3)
	topics[1].MapX = &x
	topics[1].MapY = &y

	points := ComputeLayout(topics, TerrainVolcano, DirectionUp)
	assert.Equal(t, MapPoint{X: 321, Y: 654}, points[1], "giá trị user phải thắng layout tự động")
	// Node không set vẫn theo layout → chứng minh chỉ node đó bị override.
	assert.NotEqual(t, MapPoint{X: 321, Y: 654}, points[0])
}

func Test_compute_layout_user_override_on_one_axis_only(t *testing.T) {
	x := 100.0
	topics := layoutFixture(3)
	topics[2].MapX = &x

	points := ComputeLayout(topics, TerrainMeadow, DirectionUp)
	assert.Equal(t, 100.0, points[2].X, "trục X do user đặt phải giữ nguyên")
	auto := ComputeLayout(layoutFixture(3), TerrainMeadow, DirectionUp)
	assert.Equal(t, auto[2].Y, points[2].Y, "trục Y chưa set thì lấy từ layout")
}

func Test_compute_layout_single_node_sits_at_start_of_axis(t *testing.T) {
	up := ComputeLayout([]Topic{{ID: 1}}, TerrainMeadow, DirectionUp)
	require.Len(t, up, 1)
	assert.Equal(t, MapViewHeight-MapMargin, up[0].Y, "node đơn lẻ đứng cuối trục (bottom)")

	right := ComputeLayout([]Topic{{ID: 1}}, TerrainMeadow, DirectionRight)
	require.Len(t, right, 1)
	assert.Equal(t, MapMargin, right[0].X, "node đơn lẻ đứng đầu trục (left)")
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

	wantUp := ComputeLayout(topics, TerrainMeadow, DirectionUp)
	gotBad := ComputeLayout(topics, TerrainMeadow, Direction("diagonal"))
	assert.Equal(t, wantUp, gotBad, "direction rỗng/ngoài whitelist phải rơi về up")
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
		assert.Equal(t, DirectionUp, dir, "toàn bộ path Trung đi dọc, stage %d", i)
	}
	enWant := []Terrain{TerrainMeadow, TerrainOcean, TerrainCity, TerrainSnow, TerrainVolcano}
	enDirWant := []Direction{DirectionUp, DirectionUp, DirectionRight, DirectionRight, DirectionRight}
	for i, want := range enWant {
		got, dir := MapDefaults("en", i)
		assert.Equal(t, want, got, "en stage %d", i)
		assert.Equal(t, enDirWant[i], dir, "en đổi sang ngang từ stage 2 trở đi (đọc sách)")
	}
}

func Test_map_defaults_out_of_range_falls_back_to_column_default(t *testing.T) {
	for _, c := range []struct {
		lang  string
		index int
	}{
		{"zh", 5}, {"en", 5}, {"zh", -1}, {"vi", 0}, {"", 0},
	} {
		terrain, dir := MapDefaults(c.lang, c.index)
		assert.Equal(t, TerrainDefault, terrain, "lang=%s index=%d", c.lang, c.index)
		assert.Equal(t, DirectionDefault, dir, "lang=%s index=%d", c.lang, c.index)
	}
}
