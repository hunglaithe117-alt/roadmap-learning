package roadmap

import "strings"

// Bảng 6 terrain × 2 chiều đi cho 10 stage seed, theo
// phases/ROADMAP-MAP-IDEA.md §6:
//
//	Trung: meadow → meadow → desert → snow → volcano
//	Anh:   meadow → ocean  → city   → snow → volcano
//
// Lý do Anh đổi hướng ở giữa (en-g2 trở đi sang `right`): từ G2 trở đi học
// chủ yếu bằng đọc (novel, bài báo) nên hướng ngang hợp với trục "trang sách
// đọc từ trái sang phải"; phần nghe/nói đi theo chiều dọc "đi từ dưới lên".
// Bảng này là nguồn duy nhất — migration 00004 chỉ gán lại cho DB đã có sẵn
// stage, còn stage mới do seed loader gọi hàm này.

type mapDefault struct {
	terrain   Terrain
	direction Direction
}

var zhMapDefaults = []mapDefault{
	{TerrainMeadow, DirectionUp},
	{TerrainMeadow, DirectionUp},
	{TerrainDesert, DirectionUp},
	{TerrainSnow, DirectionUp},
	{TerrainVolcano, DirectionUp},
}

var enMapDefaults = []mapDefault{
	{TerrainMeadow, DirectionUp},
	{TerrainOcean, DirectionUp},
	{TerrainCity, DirectionRight},
	{TerrainSnow, DirectionRight},
	{TerrainVolcano, DirectionRight},
}

// MapDefaults trả terrain + direction gán sẵn cho stage thứ `index` (0-based)
// của path `lang`. Path không nằm trong bảng (user tự tạo, hoặc nhiều hơn 5
// stage) rơi về (TerrainDefault, DirectionDefault) — đúng với DEFAULT của cột
// trong migration 00004, nên stage tự tạo có hành vi đồng nhất.
func MapDefaults(lang string, index int) (Terrain, Direction) {
	var table []mapDefault
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh":
		table = zhMapDefaults
	case "en":
		table = enMapDefaults
	default:
		return TerrainDefault, DirectionDefault
	}
	if index < 0 || index >= len(table) {
		return TerrainDefault, DirectionDefault
	}
	return table[index].terrain, table[index].direction
}
