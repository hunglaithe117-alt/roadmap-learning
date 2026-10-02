package roadmap

import (
	"fmt"
	"strings"
)

// Terrain là loại địa hình của 1 bản đồ (stage). Whitelist hẹp 6 giá trị —
// ROADMAP-MAP-IDEA §6 chốt "không cho user tự thêm ở v1" để layout còn
// deterministic và test được. CHECK ở DB (migration 00004) là backstop, 2
// nơi phải sửa cùng lúc.
type Terrain string

// TerrainDefault là giá trị DEFAULT của cột `roadmap_stages.terrain` và cũng là
// giá trị ComputeLayout rơi về khi gặp terrain rỗng/không hợp lệ.
const TerrainDefault Terrain = "meadow"

const (
	TerrainMeadow  Terrain = "meadow"  // đồng cỏ
	TerrainDesert  Terrain = "desert"  // sa mạc
	TerrainSnow    Terrain = "snow"    // tuyết
	TerrainVolcano Terrain = "volcano" // núi lửa
	TerrainOcean   Terrain = "ocean"   // biển
	TerrainCity    Terrain = "city"    // thành phố
)

// AllTerrains là tập hợp hợp lệ, dùng cho validate và cho vòng lặp test.
var AllTerrains = []Terrain{
	TerrainMeadow, TerrainDesert, TerrainSnow,
	TerrainVolcano, TerrainOcean, TerrainCity,
}

// Valid báo terrain có thuộc whitelist 6 giá trị không.
func (t Terrain) Valid() bool {
	for _, v := range AllTerrains {
		if v == t {
			return true
		}
	}
	return false
}

// String là nhãn tiếng Việt cho thông báo lỗi.
func (t Terrain) String() string { return string(t) }

// terrainAmplitude là biên độ dao động (đơn vị viewBox) của đường đi theo
// từng terrain — ROADMAP-MAP-IDEA §2 "hệ số dao động khác nhau theo terrain
// (desert phẳng hơn, volcano gấp thêm)". Số ở đây là hằng số thiết kế, không
// suy ra từ công thức: layout phải deterministic và đổi biên độ không được
// làm đổi hình dạng bản đồ đã lưu.
var terrainAmplitude = map[Terrain]float64{
	TerrainMeadow:  40,
	TerrainDesert:  25,
	TerrainSnow:    35,
	TerrainVolcano: 70,
	TerrainOcean:   30,
	TerrainCity:    20,
}

// Amplitude là biên độ dao động ngang của đường đi (đơn vị viewBox). Terrain
// không hợp lệ trả 0 — caller nên đã gọi Valid trước, ComputeLayout tự rơi về
// TerrainDefault.
func (t Terrain) Amplitude() float64 { return terrainAmplitude[t] }

// Direction là chiều đi của bản đồ: `up` = node 0 ở dưới, node cuối ở trên;
// `right` = node 0 bên trái, node cuối bên phải. CHECK ở DB khớp whitelist.
type Direction string

// DirectionDefault là DEFAULT của cột `roadmap_stages.direction`.
const DirectionDefault Direction = "up"

const (
	DirectionUp    Direction = "up"
	DirectionRight Direction = "right"
)

// AllDirections là tập hợp hợp lệ (đóng băng 2 giá trị).
var AllDirections = []Direction{DirectionUp, DirectionRight}

// Valid báo direction có thuộc tập 2 hằng không.
func (d Direction) Valid() bool {
	for _, v := range AllDirections {
		if v == d {
			return true
		}
	}
	return false
}

// MapPoint là toạ độ node trong viewBox của bản đồ. X là ngang, Y là dọc —
// cùng hệ toạ độ SVG để M6 render thẳng, không cần chuyển đổi.
type MapPoint struct {
	X float64
	Y float64
}

// ParseTerrain chuyển input người dùng (form/JSON) thành Terrain, trả lỗi tiếng
// Việt liệt kê đúng whitelist để client hiện được lựa chọn hợp lệ.
func ParseTerrain(raw string) (Terrain, error) {
	t := Terrain(strings.ToLower(strings.TrimSpace(raw)))
	if t == "" {
		return TerrainDefault, nil
	}
	if !t.Valid() {
		return "", fmt.Errorf("terrain chỉ nhận: %s", strings.Join(terrainNames(), ", "))
	}
	return t, nil
}

// ParseDirection giống ParseTerrain cho 2 chiều đi.
func ParseDirection(raw string) (Direction, error) {
	d := Direction(strings.ToLower(strings.TrimSpace(raw)))
	if d == "" {
		return DirectionDefault, nil
	}
	if !d.Valid() {
		return "", fmt.Errorf("direction chỉ nhận: %s", strings.Join(directionNames(), ", "))
	}
	return d, nil
}

func terrainNames() []string {
	out := make([]string, 0, len(AllTerrains))
	for _, t := range AllTerrains {
		out = append(out, string(t))
	}
	return out
}

func directionNames() []string {
	out := make([]string, 0, len(AllDirections))
	for _, d := range AllDirections {
		out = append(out, string(d))
	}
	return out
}
