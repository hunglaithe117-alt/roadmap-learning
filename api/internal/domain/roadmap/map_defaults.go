package roadmap

import "strings"

type mapDefault struct {
	terrain   Terrain
	direction Direction
}

var zhMapDefaults = []mapDefault{
	{terrain: TerrainMeadow, direction: DirectionRight},
	{terrain: TerrainMeadow, direction: DirectionRight},
	{terrain: TerrainDesert, direction: DirectionRight},
	{terrain: TerrainSnow, direction: DirectionRight},
	{terrain: TerrainVolcano, direction: DirectionRight},
}

var enMapDefaults = []mapDefault{
	{terrain: TerrainMeadow, direction: DirectionRight},
	{terrain: TerrainOcean, direction: DirectionRight},
	{terrain: TerrainCity, direction: DirectionRight},
	{terrain: TerrainSnow, direction: DirectionRight},
	{terrain: TerrainVolcano, direction: DirectionRight},
}

// MapDefaults returns predefined terrain and direction for a stage by language and 0-based index.
// Falls back to (TerrainDefault, DirectionDefault) for custom paths or overflow indexes.
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
